package recall

import (
	"context"

	"norvels/internal/recommend"
	"norvels/internal/store"
)

// 相似书召回的四路：同系列 → 同作者 → 同分类 → 共读邻居。

// sameSeries 取同系列的书（系列名精确匹配）。
func sameSeries(ctx context.Context, p Port, res *Result, seed store.BookSummary) error {
	if seed.Series == "" {
		return nil
	}
	page, err := p.ListBooks(ctx, store.BookQuery{Series: seed.Series, Page: 1, Size: recallPerSeries})
	if err != nil {
		return err
	}
	reason := recommend.SeriesReasonFor(seed.Series)
	for _, bs := range page.Items {
		res.AddSimilar(bs, reason)
	}
	return nil
}

// sameAuthor 取同作者的书。
func sameAuthor(ctx context.Context, p Port, res *Result, seed store.BookSummary) error {
	if seed.AuthorID <= 0 {
		return nil
	}
	page, err := p.ListBooks(ctx, store.BookQuery{AuthorID: seed.AuthorID, Page: 1, Size: recallPerAuthor})
	if err != nil {
		return err
	}
	reason := recommend.FixedReason(recommend.CodeSimilarAuthor)
	for _, bs := range page.Items {
		res.AddSimilar(bs, reason)
	}
	return nil
}

// sameCategory 取同 L1 / L2 分类的书（两级各查一次，理由统一为「同一分类」）。
func sameCategory(ctx context.Context, p Port, res *Result, seed store.BookSummary) error {
	if seed.CategoryL1 != "" {
		if err := similarCategory(ctx, p, res, seed.CategoryL1, true); err != nil {
			return err
		}
	}
	if seed.CategoryL2 != "" {
		return similarCategory(ctx, p, res, seed.CategoryL2, false)
	}
	return nil
}

// similarCategory 按 L1 或 L2 查一次同类书。
func similarCategory(ctx context.Context, p Port, res *Result,
	name string, l1 bool) error {
	q := store.BookQuery{Page: 1, Size: recallPerCategory}
	if l1 {
		q.CategoryL1 = name
	} else {
		q.CategoryL2 = name
	}
	page, err := p.ListBooks(ctx, q)
	if err != nil {
		return err
	}
	reason := recommend.FixedReason(recommend.CodeSimilarCategory)
	for _, bs := range page.Items {
		res.AddSimilar(bs, reason)
	}
	return nil
}

// cooccurOfBook 取「读过这本书的人还在读」的邻居（共读理由走 cooccur 分量）。
func cooccurOfBook(ctx context.Context, p Port, r RecPort, res *Result, bookID int64) error {
	pairs, err := r.CooccurForBook(ctx, bookID, recallCooccurLimit)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(pairs))
	for _, pair := range pairs {
		if pair.OtherID == bookID {
			continue
		}
		res.KeepCooccur(pair)
		ids = append(ids, pair.OtherID)
	}
	return addBooksByIDs(ctx, p, res, ids)
}
