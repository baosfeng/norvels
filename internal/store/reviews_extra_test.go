package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// 本文件补齐审核链路的边界分支：待办查询、不存在的记录、书被删后的替换 / 回滚、
// 空章对齐（block_id = 0）。

// TestPendingImportReviews 待办只取 pending，最旧的在前。
func TestPendingImportReviews(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)
	first := mustCreateReview(t, ctx, st, oldID, newID)
	second := mustCreateReview(t, ctx, st, oldID, newID)
	// 把第二条置 ready：它不该出现在待办里。
	if err := st.FinishImportReviewAI(ctx, second, ReviewAIResult{
		Status: ReviewStatusNeedsManual, Reasons: []string{"没有 key"},
	}); err != nil {
		t.Fatalf("FinishImportReviewAI: %v", err)
	}
	pending, err := st.PendingImportReviews(ctx, 10)
	if err != nil {
		t.Fatalf("PendingImportReviews: %v", err)
	}
	assertReviewIDs(t, "pending", pending, first)
	if _, err := st.PendingImportReviews(ctx, 0); err != nil {
		t.Errorf("limit=0 应回落默认值: %v", err)
	}
}

// TestGetImportReviewNotFound 不存在的记录返回 ErrNotFound。
func TestGetImportReviewNotFound(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.GetImportReview(context.Background(), 4242); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v，期望 ErrNotFound", err)
	}
}

// TestDecideAfterBookDeleted 书被删后：替换 / 回滚都显式失败，不出现半截状态。
func TestDecideAfterBookDeleted(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)
	id := mustCreateReview(t, ctx, st, oldID, newID)
	if err := st.DeleteBook(ctx, oldID); err != nil {
		t.Fatalf("DeleteBook(旧书): %v", err)
	}
	if _, err := st.DecideImportReview(ctx, id, ReviewActionReplace, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("旧书已删时替换 err = %v，期望 ErrNotFound", err)
	}
	// 回滚路径同样要求两本书都在：先确认替换，再删掉新书后回滚。
	oldID2, newID2 := reviewPair(t, ctx, st)
	id2 := mustCreateReview(t, ctx, st, oldID2, newID2)
	if _, err := st.DecideImportReview(ctx, id2, ReviewActionReplace, ""); err != nil {
		t.Fatalf("替换: %v", err)
	}
	if err := st.DeleteBook(ctx, newID2); err != nil {
		t.Fatalf("DeleteBook(新书): %v", err)
	}
	if _, err := st.RollbackImportReview(ctx, id2, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("新书已删时回滚 err = %v，期望 ErrNotFound", err)
	}
}

// TestSampleReviewDiffEmptyChapter 空章（block_id = 0）不参与差异，也不报错。
func TestSampleReviewDiffEmptyChapter(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldBook := sampleBook("空章版", "示例作者", "/books/empty1.txt", strings.Repeat("f", 64))
	oldBook.Chapters[2].Content = ""
	newBook := sampleBook("空章版", "示例作者", "/books/empty2.txt", strings.Repeat("0", 64))
	newBook.Chapters[2].Content = ""
	oldID := mustSaveBook(t, ctx, st, oldBook)
	newID := mustSaveBook(t, ctx, st, newBook)
	diff, err := st.SampleReviewDiff(ctx, oldID, newID)
	if err != nil {
		t.Fatalf("SampleReviewDiff: %v", err)
	}
	if diff.Differing != 0 {
		t.Errorf("空章比出 %d 处差异，期望 0", diff.Differing)
	}
	if diff.Compared != 3 {
		t.Errorf("比对章数 = %d，期望 3", diff.Compared)
	}
}

// TestScanReviewBookQualityTime 质量分时间列解析失败时留零值（脏数据不废整页）。
func TestScanReviewBookQualityTime(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, _ := reviewPair(t, ctx, st)
	if _, err := st.DB().ExecContext(ctx, "UPDATE books SET quality_at = '坏时间' WHERE id = ?", oldID); err != nil {
		t.Fatalf("改写 quality_at: %v", err)
	}
	books, err := st.BooksForReview(ctx, []int64{oldID})
	if err != nil {
		t.Fatalf("BooksForReview: %v", err)
	}
	if books[oldID].Quality.Computed() {
		t.Errorf("坏时间应视为未计算：%+v", books[oldID].Quality)
	}
}
