package recall

import (
	"context"
	"strconv"

	"norvels/internal/store"
)

// forYou 是默认 tab 的全量九路召回。
func forYou(ctx context.Context, p Port, r RecPort, res *Result,
	userID int64, interest map[string]float64) error {
	return runSteps([]func() error{
		func() error { return byCategories(ctx, p, res, interest) },
		func() error { return byAuthors(ctx, p, res, interest) },
		func() error { return byTags(ctx, p, res, interest) },
		func() error { return bySeries(ctx, p, res, interest) },
		func() error { return byCooccur(ctx, p, r, res, userID) },
		func() error { return byHot(ctx, r, p, res) },
		func() error { return byFresh(ctx, p, res) },
		func() error { return byRandom(ctx, p, res) },
		func() error { return byResume(ctx, p, res) },
	})
}

// resumeOnly 是续读 tab：只拼「读到一半」的书，热门/新鲜兜底保证非空。
func resumeOnly(ctx context.Context, p Port, r RecPort, res *Result) error {
	return runSteps([]func() error{
		func() error { return byResume(ctx, p, res) },
		func() error { return byHot(ctx, r, p, res) },
		func() error { return byFresh(ctx, p, res) },
	})
}

// randomOnly 是随机 tab：随机池 + 热门兜底（随机池可能为空）。
func randomOnly(ctx context.Context, p Port, r RecPort, res *Result) error {
	return runSteps([]func() error{
		func() error { return byRandom(ctx, p, res) },
		func() error { return byHot(ctx, r, p, res) },
	})
}

// byCategories 按兴趣分类召回（L1/L2 合并后逐个查）。
func byCategories(ctx context.Context, p Port, res *Result,
	interest map[string]float64) error {
	for _, name := range interestCategoryNames(interest) {
		if err := byCategory(ctx, p, res, name); err != nil {
			return err
		}
	}
	return nil
}

// byAuthors 按兴趣作者召回。
func byAuthors(ctx context.Context, p Port, res *Result, interest map[string]float64) error {
	for _, raw := range topKeys(interest, dimAuthor, recallTopAuthors) {
		authorID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue // 作者 key 是 id 的十进制文本，坏值只可能来自脏数据，跳过即可
		}
		if err := byAuthor(ctx, p, res, authorID); err != nil {
			return err
		}
	}
	return nil
}

// byTags 按兴趣标签召回：标签是人工/词表打的书目属性，比分类更细、更贴口味。
func byTags(ctx context.Context, p Port, res *Result, interest map[string]float64) error {
	for _, name := range topKeys(interest, dimTag, recallTopTags) {
		if err := addPage(ctx, p, res, store.BookQuery{Tag: name, Page: 1, Size: recallPerTag}); err != nil {
			return err
		}
	}
	return nil
}

// bySeries 按兴趣系列召回：同系列的书风格连续，是「接着读」最自然的候选。
func bySeries(ctx context.Context, p Port, res *Result, interest map[string]float64) error {
	for _, name := range topKeys(interest, dimSeries, recallTopSeries) {
		if err := addPage(ctx, p, res, store.BookQuery{Series: name, Page: 1, Size: recallPerSeries}); err != nil {
			return err
		}
	}
	return nil
}

// byCooccur 按「读过 A 的人也在读 B」召回，并记下每本书的共读强度。
func byCooccur(ctx context.Context, p Port, r RecPort, res *Result, userID int64) error {
	pairs, err := r.Cooccur(ctx, userID, recallCooccurLimit)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(pairs))
	for _, pair := range pairs {
		res.KeepCooccur(pair)
		ids = append(ids, pair.OtherID)
	}
	return addBooksByIDs(ctx, p, res, ids)
}

// byHot 按近 7 天热度召回（冷启动时这是唯一有信号的一路）。
func byHot(ctx context.Context, r RecPort, p Port, res *Result) error {
	hot, err := r.HotBooks(ctx, recallHotDays, recallHotLimit)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(hot))
	for _, h := range hot {
		res.hot[h.BookID] = h
		ids = append(ids, h.BookID)
	}
	return addBooksByIDs(ctx, p, res, ids)
}

// byFresh 取最近入库的书。
func byFresh(ctx context.Context, p Port, res *Result) error {
	return addPage(ctx, p, res, store.BookQuery{Sort: "imported", Page: 1, Size: recallFreshLimit})
}

// byRandom 随机探索：保证每次请求都有「没见过的书」。
func byRandom(ctx context.Context, p Port, res *Result) error {
	books, err := p.RandomBookSummaries(ctx, recallRandomLimit)
	if err != nil {
		return err
	}
	for _, bs := range books {
		res.Add(bs)
	}
	return nil
}

// byResume 取最近在读、进度落在 (0,1) 的书作为续读候选。
//
// 进度 0 表示刚打开没读（与没读过没区别），1 表示读完（已读不该再推）：
// 两端都排除，只留「读到一半」这一段。
func byResume(ctx context.Context, p Port, res *Result) error {
	items, err := p.ListReadingProgress(ctx, recallResumeLimit)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		if item.Percent <= 0 || item.Percent >= 1 {
			continue
		}
		res.Resume[item.BookID] = item.Percent
		ids = append(ids, item.BookID)
	}
	return addBooksByIDs(ctx, p, res, ids)
}

// byCategory 取某个分类下的书；先按大分类查，没命中再按小分类查。
//
// 合并后的名字不知道来自哪一级（L1 与 L2 的名字可能重名），所以两级都试：
// 先 L1 后 L2 的顺序让大分类命中时少一次查询。
func byCategory(ctx context.Context, p Port, res *Result, name string) error {
	page, err := p.ListBooks(ctx, store.BookQuery{CategoryL1: name, Page: 1, Size: recallPerCategory})
	if err != nil {
		return err
	}
	if len(page.Items) == 0 {
		page, err = p.ListBooks(ctx, store.BookQuery{CategoryL2: name, Page: 1, Size: recallPerCategory})
		if err != nil {
			return err
		}
	}
	for _, bs := range page.Items {
		res.Add(bs)
	}
	return nil
}

// byAuthor 取某个兴趣作者的书。
func byAuthor(ctx context.Context, p Port, res *Result, authorID int64) error {
	return addPage(ctx, p, res, store.BookQuery{AuthorID: authorID, Page: 1, Size: recallPerAuthor})
}

// addPage 查一页书目并全部收下。
func addPage(ctx context.Context, p Port, res *Result, q store.BookQuery) error {
	page, err := p.ListBooks(ctx, q)
	if err != nil {
		return err
	}
	for _, bs := range page.Items {
		res.Add(bs)
	}
	return nil
}

// addBooksByIDs 把 id 批量补成书目（推荐的候选必须带回标题与封面）。
func addBooksByIDs(ctx context.Context, p Port, res *Result, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	books, err := p.BooksByIDs(ctx, ids)
	if err != nil {
		return err
	}
	for _, bs := range books {
		res.Add(bs)
	}
	return nil
}

// loadCandidateTags 一次性补齐候选书的标签：逐本查会变成几十次主键往返。
func loadCandidateTags(ctx context.Context, p Port, res *Result) error {
	if len(res.order) == 0 {
		return nil
	}
	tags, err := p.BookTagsByIDs(ctx, res.order)
	if err != nil {
		return err
	}
	res.tags = tags
	return nil
}
