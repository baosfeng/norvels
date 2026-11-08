package main

import (
	"strings"
	"testing"
	"time"

	"norvels/internal/enrich"
	"norvels/internal/model"
	"norvels/internal/store"
)

// seedLongChapterBook 造一本含一章 4 万字正文（400 行 × 100 字）的书。
func seedLongChapterBook(t *testing.T, st *store.Store) int64 {
	t.Helper()
	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteString(strings.Repeat("字", 100))
		b.WriteByte('\n')
	}
	book := &model.ParsedBook{
		Title: "超长章节示例", Author: "作者甲", SourcePath: "/books/demo-long.txt",
		SourceName: "demo-long.txt", Encoding: "utf-8", FileSize: 1024,
		SHA256: strings.Repeat("b", 64), WordCount: 40000,
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: -1, Title: "第一章", Content: b.String(), WordCount: 40000},
		},
	}
	res, err := st.SaveBook(t.Context(), book, store.SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return res.BookID
}

// TestRunSplitLongChaptersDryRun 覆盖切分命令的 dry-run：只报告，不改库。
func TestRunSplitLongChaptersDryRun(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	bookID := seedLongChapterBook(t, st)
	err := runSplitLongChapters(t.Context(), st, splitOpts{apply: false, minWords: 30000, targetWords: 8000})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := st.GetBook(t.Context(), bookID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ChapterCount != 1 {
		t.Fatalf("dry-run 不该改章节数，实际 %d", detail.ChapterCount)
	}
}

// TestRunSplitLongChaptersApply 覆盖切分命令的 --apply：章节数增加、正文不丢。
func TestRunSplitLongChaptersApply(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	bookID := seedLongChapterBook(t, st)
	if err := runSplitLongChapters(t.Context(), st, splitOpts{apply: true, minWords: 30000, targetWords: 8000}); err != nil {
		t.Fatal(err)
	}
	detail, err := st.GetBook(t.Context(), bookID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ChapterCount < 2 {
		t.Fatalf("apply 后应切成多章，实际 %d", detail.ChapterCount)
	}
}

// TestRunSplitLongChaptersNoBoundary 覆盖「整篇没有换行」：不切、不报错。
func TestRunSplitLongChaptersNoBoundary(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	book := &model.ParsedBook{
		Title: "无段落示例", Author: "作者甲", SourcePath: "/books/demo-nonl.txt",
		SourceName: "demo-nonl.txt", Encoding: "utf-8", FileSize: 1024,
		SHA256: strings.Repeat("c", 64), WordCount: 40000,
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: -1, Title: "第一章", Content: strings.Repeat("字", 40000), WordCount: 40000},
		},
	}
	res, err := st.SaveBook(t.Context(), book, store.SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runSplitLongChapters(t.Context(), st, splitOpts{apply: true, minWords: 30000, targetWords: 8000}); err != nil {
		t.Fatal(err)
	}
	detail, _ := st.GetBook(t.Context(), res.BookID)
	if detail.ChapterCount != 1 {
		t.Fatalf("没有段落边界时不该切，实际 %d 章", detail.ChapterCount)
	}
}

// TestRunEnrichPlanOnly 覆盖 enrich --plan：只出清单、不联网、不写库。
func TestRunEnrichPlanOnly(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	seedCandidateBooks(t, st)
	err := runEnrich(t.Context(), st, enrichOpts{
		planOnly: true, fields: "author,intro,category", workers: 2, qps: 1,
		maxSources: 1, timeout: time.Second, limit: 0, minTitleSim: 0.86,
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRunEnrichOfflineCategory 覆盖「没有在线书源」：作者/简介无从补，但分类能靠本地词表离线补。
func TestRunEnrichOfflineCategory(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	bookID := seedCandidateBooks(t, st)
	err := runEnrich(t.Context(), st, enrichOpts{
		apply: true, fields: "category", workers: 1, qps: 1, maxSources: 1,
		timeout: time.Second, minTitleSim: 0.86,
	})
	if err != nil {
		t.Fatalf("没有书源时离线分类不应报错，实际 %v", err)
	}
	detail, err := st.GetBook(t.Context(), bookID)
	if err != nil {
		t.Fatal(err)
	}
	// 种子书名带「修仙」，本地词表应把它归到仙侠，且不依赖任何书源。
	if detail.CategoryL1 != "仙侠" {
		t.Fatalf("离线词表应把「修仙示例录」归为仙侠，实际 %q", detail.CategoryL1)
	}
}

// TestRunEnrichRollbackDryRun 覆盖回滚的 dry-run：打印但不写库。
func TestRunEnrichRollbackDryRun(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	bookID := seedCandidateBooks(t, st)
	author := "作者甲"
	if err := st.UpdateBookMeta(t.Context(), bookID, store.BookMetaPatch{Author: &author}); err != nil {
		t.Fatal(err)
	}
	err := st.RecordEnrichChanges(t.Context(), "b-dry", []store.EnrichChange{
		{BookID: bookID, Field: store.EnrichFieldAuthor, OldValue: "佚名", NewValue: author},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runEnrichRollback(t.Context(), st, "b-dry", false); err != nil {
		t.Fatal(err)
	}
	detail, _ := st.GetBook(t.Context(), bookID)
	if len(detail.Authors) == 0 || detail.Authors[0] != "作者甲" {
		t.Fatalf("dry-run 不该改库，实际作者 %v", detail.Authors)
	}
}

// TestRunEnrichRollbackApply 覆盖回滚的 --apply：作者写回旧值。
func TestRunEnrichRollbackApply(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	bookID := seedCandidateBooks(t, st)
	author := "作者甲"
	if err := st.UpdateBookMeta(t.Context(), bookID, store.BookMetaPatch{Author: &author}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEnrichChanges(t.Context(), "b-apply", []store.EnrichChange{
		{BookID: bookID, Field: store.EnrichFieldAuthor, OldValue: "佚名", NewValue: author},
	}); err != nil {
		t.Fatal(err)
	}
	if err := runEnrichRollback(t.Context(), st, "b-apply", true); err != nil {
		t.Fatal(err)
	}
	detail, _ := st.GetBook(t.Context(), bookID)
	if len(detail.Authors) == 0 || detail.Authors[0] != "佚名" {
		t.Fatalf("回滚后作者应为佚名，实际 %v", detail.Authors)
	}
}

// TestRunEnrichRollbackUnknownBatch 覆盖回滚不存在的批次：报错而不是静默成功。
func TestRunEnrichRollbackUnknownBatch(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	if err := runEnrichRollback(t.Context(), st, "nope", false); err == nil {
		t.Fatal("不存在的批次应报错")
	}
}

// TestWriteChangesAppliesPatch 覆盖 applyDecision 在 --apply 时真正写库。
func TestWriteChangesAppliesPatch(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	bookID := seedCandidateBooks(t, st)
	author := enrich.FieldValue{Value: "作者甲", Confidence: 1, Sources: []string{"src-a"}, Changed: true}
	dec := enrich.Decision{Author: &author}
	changes := applyDecision(t.Context(), st, store.EnrichCandidate{BookID: bookID, Author: "佚名"}, dec, true)
	if len(changes) != 1 {
		t.Fatalf("应产出 1 条变更，实际 %+v", changes)
	}
	detail, _ := st.GetBook(t.Context(), bookID)
	if len(detail.Authors) == 0 || detail.Authors[0] != "作者甲" {
		t.Fatalf("apply 后作者应更新，实际 %v", detail.Authors)
	}
}

// TestLoadResumeSet 覆盖 --resume 的跳过集合。
func TestLoadResumeSet(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	bookID := seedCandidateBooks(t, st)
	if err := st.RecordEnrichChanges(t.Context(), "b-resume", []store.EnrichChange{
		{BookID: bookID, Field: store.EnrichFieldAuthor, OldValue: "佚名", NewValue: "作者甲"},
	}); err != nil {
		t.Fatal(err)
	}
	set := loadResumeSet(t.Context(), st, enrichOpts{resume: true, resumeBatch: "b-resume"})
	if !set[bookID] {
		t.Fatalf("resume 集合应包含已处理的书，实际 %v", set)
	}
	empty := loadResumeSet(t.Context(), st, enrichOpts{resume: false})
	if len(empty) != 0 {
		t.Fatalf("未开 resume 时集合应为空，实际 %v", empty)
	}
}
