package api

import (
	"context"

	"norvels/internal/store"
)

// 标签 / 系列 / 续读三路召回（task-8 追加）。
//
// 与 recommend_recall.go 拆成两个文件：主文件已经接近 400 行上限，
// 而这三路是后来加的「增量召回」，放在一起便于单独读。

// recallByTags 按兴趣标签召回：标签是人工/词表打的书目属性，比分类更细、更贴口味。
func (s *Server) recallByTags(ctx context.Context, rr *recallResult,
	interest map[string]float64) error {
	for _, name := range topKeys(interest, interestDimTag, recallTopTags) {
		page, err := s.st.ListBooks(ctx, store.BookQuery{Tag: name, Page: 1, Size: recallPerTag})
		if err != nil {
			return err
		}
		for _, bs := range page.Items {
			rr.add(bs)
		}
	}
	return nil
}

// recallBySeries 按兴趣系列召回：同系列的书风格连续，是「接着读」最自然的候选。
func (s *Server) recallBySeries(ctx context.Context, rr *recallResult,
	interest map[string]float64) error {
	for _, name := range topKeys(interest, interestDimSeries, recallTopSeries) {
		page, err := s.st.ListBooks(ctx, store.BookQuery{Series: name, Page: 1, Size: recallPerSeries})
		if err != nil {
			return err
		}
		for _, bs := range page.Items {
			rr.add(bs)
		}
	}
	return nil
}

// recallByResume 取最近在读、进度落在 (0,1) 的书作为续读候选。
//
// 进度 0 表示刚打开没读（与没读过没区别），1 表示读完（已读不该再推）：
// 两端都排除，只留「读到一半」这一段。
func (s *Server) recallByResume(ctx context.Context, rr *recallResult) error {
	items, err := s.st.ListReadingProgress(ctx, recallResumeLimit)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(items))
	for _, p := range items {
		if p.Percent <= 0 || p.Percent >= 1 {
			continue
		}
		rr.resume[p.BookID] = p.Percent
		ids = append(ids, p.BookID)
	}
	return s.addBooksByIDs(ctx, rr, ids)
}

// loadCandidateTags 一次性补齐候选书的标签：逐本查会变成几十次主键往返。
func (s *Server) loadCandidateTags(ctx context.Context, rr *recallResult) error {
	if len(rr.order) == 0 {
		return nil
	}
	tags, err := s.st.BookTagsByIDs(ctx, rr.order)
	if err != nil {
		return err
	}
	rr.tags = tags
	return nil
}
