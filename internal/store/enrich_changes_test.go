package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// seedEnrichBook 造一本「占位作者 + 无简介 + 未分类」的书，供三类候选清单共用。
func seedEnrichBook(t *testing.T, st *Store) int64 {
	t.Helper()
	book := sampleBook("示例待补书", "佚名", "/books/demo-enrich.txt", repeat64("e"))
	book.Intro = ""
	res, err := st.SaveBook(context.Background(), book, SaveOptions{})
	requireNoErr(t, err, "seedEnrichBook")
	return res.BookID
}

// TestEnrichChangesRoundTrip 覆盖变更日志的写入 → 读回。
func TestEnrichChangesRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	bookID := seedEnrichBook(t, st)
	err := st.RecordEnrichChanges(ctx, "b1", []EnrichChange{
		{BookID: bookID, Field: EnrichFieldAuthor, OldValue: "佚名", NewValue: "作者甲",
			SourceID: "src-a", Confidence: 0.9},
		{BookID: bookID, Field: EnrichFieldIntro, NewValue: "新的简介", SourceID: "src-a", Confidence: 1},
	})
	requireNoErr(t, err, "RecordEnrichChanges")
	changes, err := st.EnrichBatchChanges(ctx, "b1")
	requireNoErr(t, err, "EnrichBatchChanges")
	requireLen(t, "变更条数", changes, 2)
	assertEqual(t, "第一条字段", changes[0].Field, EnrichFieldAuthor)
	assertEqual(t, "第一条新值", changes[0].NewValue, "作者甲")
}

// TestRecordEnrichChangesEmpty 覆盖空列表：合法调用，不建事务、不报错。
func TestRecordEnrichChangesEmpty(t *testing.T) {
	st := newTestStore(t)
	requireNoErr(t, st.RecordEnrichChanges(context.Background(), "b-empty", nil), "空批次")
}

// TestRecordEnrichChangesNoBatch 覆盖 batchID 为空：必须报错（否则回滚无从下手）。
func TestRecordEnrichChangesNoBatch(t *testing.T) {
	st := newTestStore(t)
	err := st.RecordEnrichChanges(context.Background(), "", []EnrichChange{
		{BookID: 1, Field: EnrichFieldAuthor, NewValue: "x"},
	})
	if err == nil {
		t.Fatal("batchID 为空应报错")
	}
}

// TestRollbackEnrichBatch 覆盖回滚写回：作者与简介都还原成旧值。
func TestRollbackEnrichBatch(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	bookID := seedEnrichBook(t, st)
	author, intro := "作者甲", "新的简介"
	requireNoErr(t, st.UpdateBookMeta(ctx, bookID, BookMetaPatch{Author: &author, Intro: &intro}), "写新值")
	requireNoErr(t, st.RecordEnrichChanges(ctx, "b2", []EnrichChange{
		{BookID: bookID, Field: EnrichFieldAuthor, OldValue: "佚名", NewValue: author},
		{BookID: bookID, Field: EnrichFieldIntro, OldValue: "", NewValue: intro},
	}), "记录变更")
	n, err := st.RollbackEnrichBatch(ctx, "b2")
	requireNoErr(t, err, "RollbackEnrichBatch")
	assertEqual(t, "回滚条数", n, 2)
	got, err := st.GetBook(ctx, bookID)
	requireNoErr(t, err, "GetBook")
	if len(got.Authors) == 0 || got.Authors[0] != "佚名" {
		t.Errorf("回滚后作者应为佚名，实际 %v", got.Authors)
	}
	assertEqual(t, "回滚后简介", got.Intro, "")
}

// TestRollbackEnrichBatchUnknown 覆盖批次不存在：报错而不是静默成功。
func TestRollbackEnrichBatchUnknown(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.RollbackEnrichBatch(context.Background(), "nope"); err == nil {
		t.Fatal("不存在的批次应报错")
	}
}

// TestEnrichCandidatesKinds 覆盖三类待补清单的判定口径。
func TestEnrichCandidatesKinds(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedEnrichBook(t, st)
	for _, kind := range EnrichKinds() {
		list, err := st.EnrichCandidates(ctx, kind, 10)
		requireNoErr(t, err, "kind="+kind)
		if len(list) == 0 {
			t.Errorf("kind=%s 应至少返回种子书", kind)
		}
	}
	if _, err := st.EnrichCandidates(ctx, "not_a_kind", 10); err == nil {
		t.Fatal("未知 kind 应报错")
	}
}

// TestEnrichCandidatesLimits 覆盖 limit 参数：小于等于 0 表示不限制。
func TestEnrichCandidatesLimits(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedEnrichBook(t, st)
	list, err := st.EnrichCandidates(ctx, EnrichKindMissingIntro, 1)
	requireNoErr(t, err, "limit=1")
	requireLen(t, "limit=1 条数", list, 1)
	if _, err := st.EnrichCandidates(ctx, EnrichKindMissingIntro, 0); err != nil {
		t.Fatalf("limit=0 应视为不限制，实际报错 %v", err)
	}
}

// TestListHugeChapters 覆盖超长章节阈值与参数校验。
func TestListHugeChapters(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedEnrichBook(t, st)
	long := strings.Repeat("字", 40000)
	book := sampleBook("超长章节样本", "作者甲", "/books/demo-huge.txt", repeat64("h"))
	book.Chapters = []model.ParsedChapter{
		{Idx: 0, VolumeIdx: -1, Title: "第一章", Content: long, WordCount: 40000},
		{Idx: 1, VolumeIdx: -1, Title: "第二章", Content: "很短的正文。", WordCount: 5},
	}
	_, err := st.SaveBook(ctx, book, SaveOptions{})
	requireNoErr(t, err, "保存超长章节书")
	list, err := st.ListHugeChapters(ctx, 30000, 10)
	requireNoErr(t, err, "ListHugeChapters")
	requireLen(t, "超长章条数", list, 1)
	if list[0].BookID == 0 {
		t.Errorf("超长章的 bookId 不该为 0")
	}
	if _, err := st.ListHugeChapters(ctx, 0, 10); err == nil {
		t.Fatal("minWords <= 0 应报错")
	}
}
