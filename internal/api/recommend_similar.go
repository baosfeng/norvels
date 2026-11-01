package api

import (
	"context"
	"fmt"

	"norvels/internal/recommend"
	"norvels/internal/store"
)

// 相似书推荐：GET /api/recommend?similar=<bookId>。
//
// 召回顺序固定为 同系列 → 同作者 → 同分类 → 共读邻居：越靠前的关系越「像」，
// 理由按第一条命中的关系给出（先到的路径占住文案，后面的不覆写）。

// recallSimilar 取与 bookID 相似的书（排除自身）。
func (s *Server) recallSimilar(ctx context.Context, bookID int64) (*recallResult, error) {
	books, err := s.st.BooksByIDs(ctx, []int64{bookID})
	if err != nil {
		return nil, err
	}
	if len(books) == 0 {
		return nil, fmt.Errorf("%w: 书 %d", store.ErrNotFound, bookID)
	}
	seed := books[0]
	rr := newRecallResult()
	rr.selfID = bookID
	steps := []func() error{
		func() error { return s.recallSameSeries(ctx, rr, seed) },
		func() error { return s.recallSameAuthor(ctx, rr, seed) },
		func() error { return s.recallSameCategory(ctx, rr, seed) },
		func() error { return s.recallCooccurOfBook(ctx, rr, bookID) },
	}
	if err := runRecallSteps(steps); err != nil {
		return nil, err
	}
	if err := s.loadCandidateTags(ctx, rr); err != nil {
		return nil, err
	}
	return rr, nil
}

// addSimilar 收下相似候选并记下关系文案（先到先得）。
func (rr *recallResult) addSimilar(bs store.BookSummary, reason string) {
	if bs.ID == rr.selfID {
		return
	}
	if _, ok := rr.similar[bs.ID]; !ok {
		rr.similar[bs.ID] = reason
	}
	rr.add(bs)
}

// recallSameSeries 取同系列的书（系列名精确匹配）。
func (s *Server) recallSameSeries(ctx context.Context, rr *recallResult, seed store.BookSummary) error {
	if seed.Series == "" {
		return nil
	}
	page, err := s.st.ListBooks(ctx, store.BookQuery{Series: seed.Series, Page: 1, Size: recallPerSeries})
	if err != nil {
		return err
	}
	reason := recommend.SeriesReason(seed.Series)
	for _, bs := range page.Items {
		rr.addSimilar(bs, reason)
	}
	return nil
}

// recallSameAuthor 取同作者的书。
func (s *Server) recallSameAuthor(ctx context.Context, rr *recallResult, seed store.BookSummary) error {
	if seed.AuthorID <= 0 {
		return nil
	}
	page, err := s.st.ListBooks(ctx, store.BookQuery{AuthorID: seed.AuthorID, Page: 1, Size: recallPerAuthor})
	if err != nil {
		return err
	}
	for _, bs := range page.Items {
		rr.addSimilar(bs, recommend.ReasonSameAuthor)
	}
	return nil
}

// recallSameCategory 取同 L1 / L2 分类的书（两级各查一次，理由统一为「同一分类」）。
func (s *Server) recallSameCategory(ctx context.Context, rr *recallResult, seed store.BookSummary) error {
	if seed.CategoryL1 != "" {
		if err := s.recallSimilarCategory(ctx, rr, seed.CategoryL1, true); err != nil {
			return err
		}
	}
	if seed.CategoryL2 != "" {
		return s.recallSimilarCategory(ctx, rr, seed.CategoryL2, false)
	}
	return nil
}

// recallSimilarCategory 按 L1 或 L2 查一次同类书。
func (s *Server) recallSimilarCategory(ctx context.Context, rr *recallResult,
	name string, l1 bool) error {
	q := store.BookQuery{Page: 1, Size: recallPerCategory}
	if l1 {
		q.CategoryL1 = name
	} else {
		q.CategoryL2 = name
	}
	page, err := s.st.ListBooks(ctx, q)
	if err != nil {
		return err
	}
	for _, bs := range page.Items {
		rr.addSimilar(bs, recommend.ReasonSameCategory)
	}
	return nil
}

// recallCooccurOfBook 取「读过这本书的人还在读」的邻居（共读理由走 cooccur 分量）。
func (s *Server) recallCooccurOfBook(ctx context.Context, rr *recallResult, bookID int64) error {
	pairs, err := s.cfg.Rec.CooccurForBook(ctx, bookID, recallCooccurLimit)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(pairs))
	for _, p := range pairs {
		if p.OtherID == bookID {
			continue
		}
		if p.Users > rr.cooccur[p.OtherID] {
			rr.cooccur[p.OtherID] = p.Users
		}
		ids = append(ids, p.OtherID)
	}
	return s.addBooksByIDs(ctx, rr, ids)
}
