package main

import (
	"context"
	"fmt"
	"os"

	"norvels/internal/enrich"
	"norvels/internal/store"
)

// applyDecision 把决策落到库：dry-run 只产出变更清单，--apply 才真正写。
//
// 写库统一走 store.UpdateBookMeta（不是裸 SQL）：作者变更要同步 books_authors 与
// 作者聚合列，绕过它会让 /api/authors 的作品数漂掉。
func applyDecision(ctx context.Context, st *store.Store, c store.EnrichCandidate,
	dec enrich.Decision, apply bool) []store.EnrichChange {
	changes := make([]store.EnrichChange, 0, 3)
	if dec.Author != nil && dec.Author.Changed {
		changes = append(changes, store.EnrichChange{
			BookID: c.BookID, Field: store.EnrichFieldAuthor,
			OldValue: c.Author, NewValue: dec.Author.Value,
			SourceID: firstSource(dec.Author.Sources), Confidence: dec.Author.Confidence,
		})
	}
	if dec.Intro != nil && dec.Intro.Changed {
		changes = append(changes, store.EnrichChange{
			BookID: c.BookID, Field: store.EnrichFieldIntro,
			OldValue: c.Intro, NewValue: dec.Intro.Value,
			SourceID: firstSource(dec.Intro.Sources), Confidence: dec.Intro.Confidence,
		})
	}
	if dec.Category != nil && dec.Category.Changed {
		changes = append(changes, store.EnrichChange{
			BookID: c.BookID, Field: store.EnrichFieldCategory,
			OldValue: enrich.JoinCategory(c.CategoryL1, c.CategoryL2),
			NewValue: dec.Category.Value,
			SourceID: firstSource(dec.Category.Sources), Confidence: dec.Category.Confidence,
		})
	}
	if !apply || len(changes) == 0 {
		return changes
	}
	if err := writeChanges(ctx, st, changes); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 写库失败 book=%d: %v\n", c.BookID, err)
		return nil
	}
	return changes
}

// writeChanges 把一本书的改动写进库（一次 UpdateBookMeta，字段全量给出）。
func writeChanges(ctx context.Context, st *store.Store, changes []store.EnrichChange) error {
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
	return st.UpdateBookMeta(ctx, changes[0].BookID, patch)
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
