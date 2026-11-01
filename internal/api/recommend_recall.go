package api

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"norvels/internal/recommend"
	"norvels/internal/recstore"
	"norvels/internal/store"
)

// 多路召回（设计 §3）：兴趣分类 / 兴趣作者 / 共读 / 热门 / 新鲜 / 随机。
//
// 为什么不放在 internal/recommend：召回要读两个库（主库候选 + 推荐库信号），
// 而 recommend 是纯函数包（只依赖标准库）。这里就是两库的组装点——
// 也是整个功能里唯一同时认识 store 与 recstore 的地方。

const (
	recallPerCategory   = 40 // 每个兴趣分类取 40 本（设计 §3）
	recallTopCategories = 3  // 只取用户 top3 分类：兴趣分布长尾，多取只是重复同一批书
	recallTopAuthors    = 2
	recallPerAuthor     = 20
	recallCooccurLimit  = 60
	recallHotDays       = 7
	recallHotLimit      = 40
	recallFreshLimit    = 20
	recallRandomLimit   = 20
	// recentShownDays 是「近期已曝光」的窗口：7 天内推过的不重复推（只扣分不硬过滤）。
	recentShownDays = 7
)

// recallResult 是各路召回的并集：书目（回主库补齐的展示字段）+ 打分信号。
//
// 用 map 去重 + order 保序：同一本书常被多路同时召回（兴趣 + 热度），
// 只留第一次出现的顺序能让「随机探索」与打分的输入稳定可复现。
type recallResult struct {
	books   map[int64]store.BookSummary
	order   []int64
	cooccur map[int64]int
	hot     map[int64]recstore.HotBook
}

// newRecallResult 预分配容量（一屏召回通常几十到几百本）。
func newRecallResult() *recallResult {
	return &recallResult{
		books:   make(map[int64]store.BookSummary, 64),
		order:   make([]int64, 0, 64),
		cooccur: make(map[int64]int, 32),
		hot:     make(map[int64]recstore.HotBook, 32),
	}
}

// add 收下一本书（同一本书只留第一次出现的顺序）。
func (rr *recallResult) add(bs store.BookSummary) {
	if bs.ID <= 0 {
		return
	}
	if _, ok := rr.books[bs.ID]; ok {
		return
	}
	rr.books[bs.ID] = bs
	rr.order = append(rr.order, bs.ID)
}

// recallSignals 是打分需要的用户侧信号（与召回结果一起进 candidates）。
type recallSignals struct {
	read   map[int64]bool
	recent map[int64]bool
	now    time.Time
}

// candidates 把召回结果转成打分输入（顺序与召回顺序一致）。
func (rr *recallResult) candidates(sig recallSignals) []recommend.Candidate {
	out := make([]recommend.Candidate, 0, len(rr.order))
	for _, id := range rr.order {
		bs := rr.books[id]
		out = append(out, recommend.Candidate{
			BookID: bs.ID, AuthorID: bs.AuthorID,
			CategoryL1: bs.CategoryL1, CategoryL2: bs.CategoryL2,
			ImportedAt: bs.ImportedAt, Rating: bs.Rating,
			Read: sig.read[id], RecentShown: sig.recent[id],
			CooccurUsers: rr.cooccur[id],
			HotReads:     rr.hot[id].Reads, HotClicks: rr.hot[id].Clicks,
			Fresh: recommend.Freshness(bs.ImportedAt, sig.now),
		})
	}
	return out
}

// recallCandidates 依次跑完六路召回（顺序无关，缺失的库信号自然为空）。
func (s *Server) recallCandidates(ctx context.Context, userID int64,
	interest map[string]float64) (*recallResult, error) {
	rr := newRecallResult()
	steps := []func() error{
		func() error { return s.recallByInterest(ctx, rr, interest) },
		func() error { return s.recallByCooccur(ctx, rr, userID) },
		func() error { return s.recallByHot(ctx, rr) },
		func() error { return s.recallByFresh(ctx, rr) },
		func() error { return s.recallByRandom(ctx, rr) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return rr, nil
}

// recallByInterest 按兴趣分类与兴趣作者两路召回。
func (s *Server) recallByInterest(ctx context.Context, rr *recallResult,
	interest map[string]float64) error {
	for _, name := range topKeys(interest, "category", recallTopCategories) {
		if err := s.recallCategory(ctx, rr, name); err != nil {
			return err
		}
	}
	for _, raw := range topKeys(interest, "author", recallTopAuthors) {
		authorID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue // 作者 key 是 id 的十进制文本，坏值只可能来自脏数据，跳过即可
		}
		if err := s.recallAuthor(ctx, rr, authorID); err != nil {
			return err
		}
	}
	return nil
}

// recallCategory 取某个分类下的书；先按大分类查，没命中再按小分类查。
//
// 兴趣表里只存「分类名」不存层级（导入时 L1/L2 都写进同一个 dim），
// 所以一个名字可能两级都不是；先 L1 后 L2 的顺序让大分类命中时少一次查询。
func (s *Server) recallCategory(ctx context.Context, rr *recallResult, name string) error {
	page, err := s.st.ListBooks(ctx, store.BookQuery{CategoryL1: name, Page: 1, Size: recallPerCategory})
	if err != nil {
		return err
	}
	if len(page.Items) == 0 {
		page, err = s.st.ListBooks(ctx, store.BookQuery{CategoryL2: name, Page: 1, Size: recallPerCategory})
		if err != nil {
			return err
		}
	}
	for _, bs := range page.Items {
		rr.add(bs)
	}
	return nil
}

// recallAuthor 取某个兴趣作者的书。
func (s *Server) recallAuthor(ctx context.Context, rr *recallResult, authorID int64) error {
	page, err := s.st.ListBooks(ctx, store.BookQuery{AuthorID: authorID, Page: 1, Size: recallPerAuthor})
	if err != nil {
		return err
	}
	for _, bs := range page.Items {
		rr.add(bs)
	}
	return nil
}

// recallByCooccur 按「读过 A 的人也在读 B」召回，并记下每本书的共读强度。
func (s *Server) recallByCooccur(ctx context.Context, rr *recallResult, userID int64) error {
	pairs, err := s.cfg.Rec.Cooccur(ctx, userID, recallCooccurLimit)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(pairs))
	for _, p := range pairs {
		// 同一邻居书可能由多本已读书贡献：取最大共读人数（取和会把「读得多」算成「读得广」）。
		if p.Users > rr.cooccur[p.OtherID] {
			rr.cooccur[p.OtherID] = p.Users
		}
		ids = append(ids, p.OtherID)
	}
	return s.addBooksByIDs(ctx, rr, ids)
}

// recallByHot 按近 7 天热度召回（冷启动时这是唯一有信号的一路）。
func (s *Server) recallByHot(ctx context.Context, rr *recallResult) error {
	hot, err := s.cfg.Rec.HotBooks(ctx, recallHotDays, recallHotLimit)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(hot))
	for _, h := range hot {
		rr.hot[h.BookID] = h
		ids = append(ids, h.BookID)
	}
	return s.addBooksByIDs(ctx, rr, ids)
}

// recallByFresh 取最近入库的书。
func (s *Server) recallByFresh(ctx context.Context, rr *recallResult) error {
	page, err := s.st.ListBooks(ctx, store.BookQuery{Sort: "imported", Page: 1, Size: recallFreshLimit})
	if err != nil {
		return err
	}
	for _, bs := range page.Items {
		rr.add(bs)
	}
	return nil
}

// recallByRandom 随机探索：保证每次请求都有「没见过的书」。
func (s *Server) recallByRandom(ctx context.Context, rr *recallResult) error {
	books, err := s.st.RandomBookSummaries(ctx, recallRandomLimit)
	if err != nil {
		return err
	}
	for _, bs := range books {
		rr.add(bs)
	}
	return nil
}

// addBooksByIDs 把 id 批量补成书目（推荐的候选必须带回标题与封面）。
func (s *Server) addBooksByIDs(ctx context.Context, rr *recallResult, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	books, err := s.st.BooksByIDs(ctx, ids)
	if err != nil {
		return err
	}
	for _, bs := range books {
		rr.add(bs)
	}
	return nil
}

// topKeys 取某个兴趣维度下权重最高的前 n 个 key（去掉 "dim:" 前缀）。
//
// 同权重按 key 字典序：排序必须完全确定，否则同一次请求的召回集合会随
// map 迭代顺序漂移，测试与线上排查都复现不了。
func topKeys(interest map[string]float64, dim string, n int) []string {
	type entry struct {
		key    string
		weight float64
	}
	prefix := dim + ":"
	entries := make([]entry, 0, len(interest))
	for k, w := range interest {
		if strings.HasPrefix(k, prefix) {
			entries = append(entries, entry{key: strings.TrimPrefix(k, prefix), weight: w})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].weight != entries[j].weight {
			return entries[i].weight > entries[j].weight
		}
		return entries[i].key < entries[j].key
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.key
	}
	return out
}
