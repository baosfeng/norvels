package store

import (
	"context"
	"fmt"
	"strings"
)

// RollbackEnrichBatch 按批次把旧值写回，返回回滚的变更条数。
//
// 三条口径：
//   - 只回滚本次记录过的字段，不做「整本书恢复快照」——那是另一种语义，
//     而且嵌入式的正文库无法原子地跟着回退；
//   - 逐条走 UpdateBookMeta（作者同步 books_authors 与聚合列），不写裸 SQL；
//   - 单条失败不中断整批：路径上已有 book 被删、字段被用户改过等情形都会让单条失败，
//     中断会让「回滚一半」这种最难收拾的状态出现；失败条数由返回值的差集体现。
func (s *Store) RollbackEnrichBatch(ctx context.Context, batchID string) (int, error) {
	changes, err := s.EnrichBatchChanges(ctx, batchID)
	if err != nil {
		return 0, err
	}
	if len(changes) == 0 {
		return 0, fmt.Errorf("%w: 批次 %s 没有变更记录", ErrInvalidArgument, batchID)
	}
	done := 0
	var firstErr error
	for _, ch := range changes {
		patch, ok := rollbackPatch(ch.Field, ch.OldValue)
		if !ok {
			continue
		}
		if err := s.UpdateBookMeta(ctx, ch.BookID, patch); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		done++
	}
	if firstErr != nil {
		return done, fmt.Errorf("store: 回滚部分失败（成功 %d / 共 %d）：%w", done, len(changes), firstErr)
	}
	return done, nil
}

// rollbackPatch 把一条变更记录翻成「写回旧值」的补丁；未知字段返回 ok=false。
func rollbackPatch(field, old string) (BookMetaPatch, bool) {
	v := old
	switch field {
	case EnrichFieldAuthor:
		return BookMetaPatch{Author: &v}, true
	case EnrichFieldIntro:
		return BookMetaPatch{Intro: &v}, true
	case EnrichFieldCategory:
		l1, l2 := splitCategoryValue(v)
		return BookMetaPatch{CategoryL1: &l1, CategoryL2: &l2}, true
	}
	return BookMetaPatch{}, false
}

// splitCategoryValue 把 "L1/L2" 拆回两级；没有 "/" 时整串当 L1。
func splitCategoryValue(v string) (l1, l2 string) {
	if i := strings.IndexByte(v, '/'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}
