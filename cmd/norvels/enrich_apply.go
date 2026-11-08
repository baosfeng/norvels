package main

import (
	"context"
	"fmt"
	"os"

	"norvels/internal/enrich"
	"norvels/internal/store"
)

// decideChanges 把一本书的决策摊成变更清单（**不写库**）。
//
// 写库由调用方在「日志已落库」之后统一做：先记 intent 再改数据，
// 进程中途被杀也不会留下「改了库却没有回滚依据」的改动。
func decideChanges(c store.EnrichCandidate, dec enrich.Decision) []store.EnrichChange {
	changes := make([]store.EnrichChange, 0, 3)
	changes = appendField(changes, c, store.EnrichFieldAuthor, dec.Author, c.Author)
	changes = appendField(changes, c, store.EnrichFieldIntro, dec.Intro, c.Intro)
	category := enrich.JoinCategory(c.CategoryL1, c.CategoryL2)
	changes = appendField(changes, c, store.EnrichFieldCategory, dec.Category, category)
	return changes
}

// appendField 把一个字段决策追加进变更清单（nil 或未变化则跳过）。
func appendField(out []store.EnrichChange, c store.EnrichCandidate, field string,
	v *enrich.FieldValue, old string) []store.EnrichChange {
	if v == nil || !v.Changed {
		return out
	}
	return append(out, store.EnrichChange{
		BookID: c.BookID, Field: field, OldValue: old, NewValue: v.Value,
		SourceID: firstSource(v.Sources), Confidence: v.Confidence,
	})
}

// applyEnrichChanges 按书分组把变更写进库（一次 UpdateBookMeta 一本）。
//
// 写库统一走 store.UpdateBookMeta（不是裸 SQL）：作者变更要同步 books_authors
// 与作者聚合列，绕过它会让 /api/authors 的作品数漂掉。
// 单本失败只警告不中断：整批停在一半比少数几本没写成更难收拾。
func applyEnrichChanges(ctx context.Context, st *store.Store, changes []store.EnrichChange) int {
	byBook := make(map[int64][]store.EnrichChange, len(changes))
	order := make([]int64, 0, len(changes))
	for _, ch := range changes {
		if _, ok := byBook[ch.BookID]; !ok {
			order = append(order, ch.BookID)
		}
		byBook[ch.BookID] = append(byBook[ch.BookID], ch)
	}
	written := 0
	for _, bookID := range order {
		if err := writeBookChanges(ctx, st, bookID, byBook[bookID]); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 写库失败 book=%d: %v\n", bookID, err)
			continue
		}
		written += len(byBook[bookID])
	}
	return written
}

// writeBookChanges 把一本书的改动写成一次 UpdateBookMeta。
func writeBookChanges(ctx context.Context, st *store.Store, bookID int64, changes []store.EnrichChange) error {
	patch := store.BookMetaPatch{}
	for _, ch := range changes {
		value := ch.NewValue
		switch ch.Field {
		case store.EnrichFieldAuthor:
			patch.Author = &value
		case store.EnrichFieldIntro:
			patch.Intro = &value
		case store.EnrichFieldCategory:
			l1, l2 := enrich.SplitCategory(value)
			patch.CategoryL1 = &l1
			patch.CategoryL2 = &l2
		}
	}
	return st.UpdateBookMeta(ctx, bookID, patch)
}

// firstSource 取首个参与投票的源 id（报告里只展示一个，完整清单留在 Value.Sources）。
func firstSource(sources []string) string {
	if len(sources) == 0 {
		return "unknown"
	}
	return sources[0]
}

// runEnrichRollback 按批次回滚：把 old_value 写回库。
//
// 只回滚「本批次记录过的字段」；dry-run 时只打印将回滚多少条。
func runEnrichRollback(ctx context.Context, st *store.Store, batchID string, apply bool) error {
	changes, err := st.EnrichBatchChanges(ctx, batchID)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return fmt.Errorf("批次 %s 没有可回滚的记录", batchID)
	}
	fmt.Printf("批次 %s：待回滚 %d 条变更%s\n", batchID, len(changes), dryHint(apply))
	if !apply {
		for _, ch := range changes {
			fmt.Printf("  书 %d · %s：%q → %q\n", ch.BookID, ch.Field, ch.NewValue, ch.OldValue)
		}
		return nil
	}
	n, err := st.RollbackEnrichBatch(ctx, batchID)
	if err != nil {
		return err
	}
	fmt.Printf("已回滚 %d 条变更（批次 %s）\n", n, batchID)
	return nil
}

// dryHint 返回 dry-run 提示串（回滚与切分共用）。
func dryHint(apply bool) string {
	if apply {
		return ""
	}
	return "（dry-run，未写库；加 --apply 才真执行）"
}
