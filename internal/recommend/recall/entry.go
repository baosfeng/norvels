package recall

import (
	"context"
	"fmt"

	"norvels/internal/store"
)

// Candidates 按 tab 跑对应召回并补齐候选标签（缺失的库信号自然为空）。
func Candidates(ctx context.Context, p Port, r RecPort, userID int64,
	interest map[string]float64, tab string) (*Result, error) {
	res := New()
	var err error
	switch tab {
	case TabResume:
		err = resumeOnly(ctx, p, r, res)
	case TabRandom:
		err = randomOnly(ctx, p, r, res)
	default:
		err = forYou(ctx, p, r, res, userID, interest)
	}
	if err != nil {
		return nil, err
	}
	if err := loadCandidateTags(ctx, p, res); err != nil {
		return nil, err
	}
	return res, nil
}

// Similar 取与 bookID 相似的书（排除自身）。
//
// 召回顺序固定为 同系列 → 同作者 → 同分类 → 共读邻居：越靠前的关系越「像」，
// 理由按第一条命中的关系给出（先到的路径占住文案，后面的不覆写）。
func Similar(ctx context.Context, p Port, r RecPort, bookID int64) (*Result, error) {
	books, err := p.BooksByIDs(ctx, []int64{bookID})
	if err != nil {
		return nil, err
	}
	if len(books) == 0 {
		return nil, fmt.Errorf("%w: 书 %d", store.ErrNotFound, bookID)
	}
	seed := books[0]
	res := New()
	res.selfID = bookID
	if err := runSteps([]func() error{
		func() error { return sameSeries(ctx, p, res, seed) },
		func() error { return sameAuthor(ctx, p, res, seed) },
		func() error { return sameCategory(ctx, p, res, seed) },
		func() error { return cooccurOfBook(ctx, p, r, res, bookID) },
	}); err != nil {
		return nil, err
	}
	if err := loadCandidateTags(ctx, p, res); err != nil {
		return nil, err
	}
	return res, nil
}

// runSteps 顺序执行召回步骤，任一步失败即整体失败。
func runSteps(steps []func() error) error {
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}
