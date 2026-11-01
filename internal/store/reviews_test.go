package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 本文件锁死 AI 审核记录（0021，设计 §6 / §7）的存储语义：状态机、替换 / 保留 / 回滚
// 对 books.superseded_by 的影响，以及「被替换掉的旧版本对书架 / 详情 / 阅读不可见」。

// reviewPair 造一对「同一本书的两个版本」并入库：新版第 2 章正文不同。
//
// 返回 (旧书 id, 新书 id)。两本书都不做隐藏：审核记录由 CreateImportReview 落，
// 它负责把新书标记成 superseded。
func reviewPair(t *testing.T, ctx context.Context, st *Store) (int64, int64) {
	t.Helper()
	oldID := mustSaveBook(t, ctx, st, sampleBook("旧版", "示例作者", "/books/old.txt", strings.Repeat("a", 64)))
	newBook := sampleBook("新版", "示例作者", "/books/new.txt", strings.Repeat("b", 64))
	newBook.Chapters[1].Content = "他站在城楼上，望着远方的临川渡与风雪。"
	newID := mustSaveBook(t, ctx, st, newBook)
	return oldID, newID
}

// mustSaveBook 写一本书并返回 book id。
func mustSaveBook(t *testing.T, ctx context.Context, st *Store, b *model.ParsedBook) int64 {
	t.Helper()
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook(%s): %v", b.Title, err)
	}
	return res.BookID
}

// mustCreateReview 落一条审核记录（规则对比文案固定两条，便于断言）。
func mustCreateReview(t *testing.T, ctx context.Context, st *Store, oldID, newID int64) int64 {
	t.Helper()
	id, err := st.CreateImportReview(ctx, NewReview{
		JobID: 7, NewBookID: newID, OldBookID: oldID, HitRatio: 0.96,
		RuleReasons: []string{"噪声率 3.2%→0.4%", "乱码率 0.210%→0.000%"},
	})
	if err != nil {
		t.Fatalf("CreateImportReview: %v", err)
	}
	return id
}

// supersededOf 读一本书的 superseded_by。
func supersededOf(t *testing.T, ctx context.Context, st *Store, bookID int64) int64 {
	t.Helper()
	var v int64
	if err := st.DB().QueryRowContext(ctx, "SELECT superseded_by FROM books WHERE id = ?", bookID).Scan(&v); err != nil {
		t.Fatalf("read superseded_by %d: %v", bookID, err)
	}
	return v
}

// visibleBookIDs 返回书架可见（非 superseded）的书 id。
func visibleBookIDs(t *testing.T, ctx context.Context, st *Store) []int64 {
	t.Helper()
	page, err := st.ListBooks(ctx, BookQuery{})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	out := make([]int64, 0, len(page.Items))
	for _, it := range page.Items {
		out = append(out, it.ID)
	}
	return out
}

// TestCreateImportReviewHidesNewBook 落记录时新书立刻隐藏，旧书仍是当前版本。
func TestCreateImportReviewHidesNewBook(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)

	id := mustCreateReview(t, ctx, st, oldID, newID)
	rev, err := st.GetImportReview(ctx, id)
	if err != nil {
		t.Fatalf("GetImportReview: %v", err)
	}
	assertReviewShape(t, rev)
	assertBookVisibility(t, ctx, st, oldID, newID)
}

// assertReviewShape 断言新建记录的形状：pending、命中率、任务、理由、无决策信息。
func assertReviewShape(t *testing.T, rev ImportReview) {
	t.Helper()
	if rev.Status != ReviewStatusPending || rev.HitRatio != 0.96 || rev.JobID != 7 {
		t.Errorf("记录 = %+v，期望 pending / 0.96 / job 7", rev)
	}
	if len(rev.RuleReasons) != 2 || rev.AIReasons == nil {
		t.Errorf("理由 = 规则 %v / AI %v，期望规则两条且 AI 为空切片", rev.RuleReasons, rev.AIReasons)
	}
	if rev.DecidedAt != nil || rev.DecidedBy != "" {
		t.Errorf("未决策的记录不该带决策信息：%+v", rev)
	}
}

// assertBookVisibility 断言新书对读者不可见（书架 / 详情 / 目录），旧书照常可读。
func assertBookVisibility(t *testing.T, ctx context.Context, st *Store, oldID, newID int64) {
	t.Helper()
	if got := supersededOf(t, ctx, st, newID); got != oldID {
		t.Errorf("新书 superseded_by = %d，期望 %d", got, oldID)
	}
	if got := visibleBookIDs(t, ctx, st); len(got) != 1 || got[0] != oldID {
		t.Errorf("书架 = %v，期望只剩旧书 %d", got, oldID)
	}
	if _, err := st.GetBook(ctx, newID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetBook(隐藏书) err = %v，期望 ErrNotFound", err)
	}
	chapters, err := st.ListChapters(ctx, ChapterQuery{BookID: newID})
	if err != nil {
		t.Fatalf("ListChapters(隐藏书): %v", err)
	}
	if chapters.Total != 0 {
		t.Errorf("隐藏书的目录 = %d 章，期望 0", chapters.Total)
	}
	if _, err := st.GetBook(ctx, oldID); err != nil {
		t.Errorf("GetBook(旧书) err = %v，期望可读", err)
	}
}

// TestCreateImportReviewRejectsBadInput 入参非法与旧书不存在都要显式失败。
func TestCreateImportReviewRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)

	cases := []struct {
		name string
		in   NewReview
		want error
	}{
		{"同书", NewReview{NewBookID: oldID, OldBookID: oldID}, ErrInvalidArgument},
		{"新书 id 非法", NewReview{NewBookID: 0, OldBookID: oldID}, ErrInvalidArgument},
		{"旧书不存在", NewReview{NewBookID: newID, OldBookID: 9999}, ErrNotFound},
	}
	for _, tc := range cases {
		if _, err := st.CreateImportReview(ctx, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v，期望 %v", tc.name, err, tc.want)
		}
	}
	// 失败路径不能留下「书被隐藏了但没有记录」的半截状态。
	if got := supersededOf(t, ctx, st, newID); got != 0 {
		t.Errorf("失败的创建改动了新书的 superseded_by = %d，期望 0", got)
	}
}

// TestListImportReviewsFilters 列表按状态筛、最新的在前、条数有上限。
func TestListImportReviewsFilters(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)
	first := mustCreateReview(t, ctx, st, oldID, newID)
	second := mustCreateReview(t, ctx, st, oldID, newID)
	if err := st.FinishImportReviewAI(ctx, second, ReviewAIResult{
		Status: ReviewStatusReady, Winner: ReviewWinnerNew, Confidence: 0.8,
		Reasons: []string{"错字更少"},
	}); err != nil {
		t.Fatalf("FinishImportReviewAI: %v", err)
	}

	assertReviewIDs(t, "pending", mustListReviews(t, ctx, st, ReviewStatusPending, 0), first)
	assertReviewIDs(t, "ready", mustListReviews(t, ctx, st, ReviewStatusReady, 0), second)
	// 默认按 created_at DESC, id DESC：后建的在前。
	assertReviewIDs(t, "全部", mustListReviews(t, ctx, st, ReviewStatusAll, 0), second, first)
	assertReviewIDs(t, "limit=1", mustListReviews(t, ctx, st, "", 1), second)
	assertReviewIDs(t, "未知状态", mustListReviews(t, ctx, st, "no-such-status", 0))
	// AI 结论写回在 ready 那条上。
	ready := mustListReviews(t, ctx, st, ReviewStatusReady, 0)
	if len(ready) == 1 && ready[0].AIWinner != ReviewWinnerNew {
		t.Errorf("ready 列表 = %+v，期望带 AI 结论", ready)
	}
}

// mustListReviews 列表查询（出错直接失败）。
func mustListReviews(t *testing.T, ctx context.Context, st *Store, status string, limit int) []ImportReview {
	t.Helper()
	items, err := st.ListImportReviews(ctx, status, limit)
	if err != nil {
		t.Fatalf("ListImportReviews(%q): %v", status, err)
	}
	return items
}

// assertReviewIDs 断言列表里恰好是这些 id（顺序敏感）。
func assertReviewIDs(t *testing.T, name string, got []ImportReview, want ...int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s 列表 = %d 条，期望 %d 条：%+v", name, len(got), len(want), got)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("%s 列表[%d] = %d，期望 %d", name, i, got[i].ID, id)
		}
	}
}

// TestFinishImportReviewAI 只允许 pending 被 AI 结果改写，且校验结论形状。
func TestFinishImportReviewAI(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)
	id := mustCreateReview(t, ctx, st, oldID, newID)

	bad := []ReviewAIResult{
		{Status: "unknown"},
		{Status: ReviewStatusReady, Winner: "A"},
	}
	for _, res := range bad {
		if err := st.FinishImportReviewAI(ctx, id, res); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("写回 %+v err = %v，期望 ErrInvalidArgument", res, err)
		}
	}
	if err := st.FinishImportReviewAI(ctx, id, ReviewAIResult{
		Status: ReviewStatusReady, Winner: ReviewWinnerOld, Confidence: 1.5,
		Reasons: []string{"旧版更通顺"},
	}); err != nil {
		t.Fatalf("写回 ready: %v", err)
	}
	rev, _ := st.GetImportReview(ctx, id)
	if rev.Status != ReviewStatusReady || rev.AIWinner != ReviewWinnerOld || rev.AIConfidence != 1 {
		t.Errorf("写回结果 = %+v，期望 ready / old / 置信度夹到 1", rev)
	}
	// 非 pending 不能被再次改写。
	if err := st.FinishImportReviewAI(ctx, id, ReviewAIResult{Status: ReviewStatusNeedsManual}); !errors.Is(err, ErrReviewState) {
		t.Errorf("重复写回 err = %v，期望 ErrReviewState", err)
	}
	// 不存在的记录。
	if err := st.FinishImportReviewAI(ctx, 9999, ReviewAIResult{Status: ReviewStatusNeedsManual}); !errors.Is(err, ErrReviewState) {
		t.Errorf("写回不存在的记录 err = %v，期望 ErrReviewState", err)
	}
}

// TestDecideReplaceAndRollback 替换 → 书架只剩新书；回滚 → 恢复旧书。
func TestDecideReplaceAndRollback(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)
	id := mustCreateReview(t, ctx, st, oldID, newID)

	// 旧书的章节 id 在替换前取出来：替换后这一章应当读不到。
	chapters, err := st.ListChapters(ctx, ChapterQuery{BookID: oldID})
	if err != nil || len(chapters.Items) == 0 {
		t.Fatalf("ListChapters(旧书) = %+v, err=%v", chapters, err)
	}
	oldChapterID := chapters.Items[0].ID

	rev, err := st.DecideImportReview(ctx, id, ReviewActionReplace, "user:1")
	if err != nil {
		t.Fatalf("DecideImportReview(replace): %v", err)
	}
	assertReplaced(t, ctx, st, rev, oldID, newID, oldChapterID)

	if _, err := st.RollbackImportReview(ctx, id, "user:1"); err != nil {
		t.Fatalf("RollbackImportReview: %v", err)
	}
	assertRolledBack(t, ctx, st, oldID, newID, oldChapterID)
}

// assertReplaced 断言替换后的状态：新书成为当前版本，旧书连正文都读不到。
func assertReplaced(t *testing.T, ctx context.Context, st *Store, rev ImportReview,
	oldID, newID, oldChapterID int64) {
	t.Helper()
	if rev.Status != ReviewStatusConfirmedReplace || rev.DecidedBy != "user:1" || rev.DecidedAt == nil {
		t.Errorf("替换后记录 = %+v，期望 confirmed_replace / user:1 / 有决策时间", rev)
	}
	if got := supersededOf(t, ctx, st, oldID); got != newID {
		t.Errorf("旧书 superseded_by = %d，期望 %d", got, newID)
	}
	if got := supersededOf(t, ctx, st, newID); got != 0 {
		t.Errorf("新书 superseded_by = %d，期望 0（成为当前版本）", got)
	}
	if got := visibleBookIDs(t, ctx, st); len(got) != 1 || got[0] != newID {
		t.Errorf("替换后书架 = %v，期望只剩新书 %d", got, newID)
	}
	if _, err := st.GetChapter(ctx, oldID, oldChapterID); !errors.Is(err, ErrNotFound) {
		t.Errorf("读被替换掉的旧版本正文 err = %v，期望 ErrNotFound", err)
	}
}

// assertRolledBack 断言回滚后的状态：旧书恢复可见、正文可读，新书重新隐藏。
func assertRolledBack(t *testing.T, ctx context.Context, st *Store, oldID, newID, oldChapterID int64) {
	t.Helper()
	if got := visibleBookIDs(t, ctx, st); len(got) != 1 || got[0] != oldID {
		t.Errorf("回滚后书架 = %v，期望恢复旧书 %d", got, oldID)
	}
	if got := supersededOf(t, ctx, st, newID); got != oldID {
		t.Errorf("回滚后新书 superseded_by = %d，期望 %d", got, oldID)
	}
	if _, err := st.GetChapter(ctx, oldID, oldChapterID); err != nil {
		t.Errorf("回滚后旧版本正文应可读: %v", err)
	}
}

// TestDecideKeepOnlyChangesStatus 保留只改状态：书侧完全不动。
func TestDecideKeepOnlyChangesStatus(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)
	id := mustCreateReview(t, ctx, st, oldID, newID)

	rev, err := st.DecideImportReview(ctx, id, ReviewActionKeep, "")
	if err != nil {
		t.Fatalf("DecideImportReview(keep): %v", err)
	}
	if rev.Status != ReviewStatusConfirmedKeep || rev.DecidedBy != "" {
		t.Errorf("保留后记录 = %+v，期望 confirmed_keep / 无账号", rev)
	}
	if got := supersededOf(t, ctx, st, oldID); got != 0 {
		t.Errorf("旧书 superseded_by = %d，期望 0", got)
	}
	if got := supersededOf(t, ctx, st, newID); got != oldID {
		t.Errorf("新书 superseded_by = %d，期望仍是 %d", got, oldID)
	}
}

// TestReviewStateMachineRejects 非法迁移一律 ErrReviewState。
func TestReviewStateMachineRejects(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)
	id := mustCreateReview(t, ctx, st, oldID, newID)

	if _, err := st.DecideImportReview(ctx, id, "bogus", ""); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("未知决策 err = %v，期望 ErrInvalidArgument", err)
	}
	if _, err := st.RollbackImportReview(ctx, id, ""); !errors.Is(err, ErrReviewState) {
		t.Errorf("pending 回滚 err = %v，期望 ErrReviewState", err)
	}
	if _, err := st.DecideImportReview(ctx, 9999, ReviewActionKeep, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的记录 err = %v，期望 ErrNotFound", err)
	}
	if _, err := st.DecideImportReview(ctx, id, ReviewActionKeep, ""); err != nil {
		t.Fatalf("保留: %v", err)
	}
	if _, err := st.DecideImportReview(ctx, id, ReviewActionKeep, ""); !errors.Is(err, ErrReviewState) {
		t.Errorf("重复决策 err = %v，期望 ErrReviewState", err)
	}
	if _, err := st.RollbackImportReview(ctx, id, ""); !errors.Is(err, ErrReviewState) {
		t.Errorf("confirmed_keep 回滚 err = %v，期望 ErrReviewState", err)
	}
}

// TestBooksForReview 批量取书摘要：被隐藏的书也要能取到（审核页要对照）。
func TestBooksForReview(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)
	if _, err := st.CreateImportReview(ctx, NewReview{NewBookID: newID, OldBookID: oldID, HitRatio: 0.99}); err != nil {
		t.Fatalf("CreateImportReview: %v", err)
	}
	books, err := st.BooksForReview(ctx, []int64{oldID, newID, 0, oldID, 9999})
	if err != nil {
		t.Fatalf("BooksForReview: %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("取回 %d 本书，期望 2（重复 / 非法 / 不存在的 id 被忽略）", len(books))
	}
	if books[oldID].Title != "旧版" || books[newID].Title != "新版" {
		t.Errorf("书名 = %q / %q", books[oldID].Title, books[newID].Title)
	}
	if books[newID].SupersededBy != oldID {
		t.Errorf("新书 superseded_by = %d，期望 %d", books[newID].SupersededBy, oldID)
	}
	if books[oldID].ChapterCount != 3 {
		t.Errorf("章数 = %d，期望 3", books[oldID].ChapterCount)
	}
	if empty, err := st.BooksForReview(ctx, nil); err != nil || len(empty) != 0 {
		t.Errorf("空入参 = %+v, err=%v", empty, err)
	}
}

// TestDeleteReleasesHiddenBook 删除「取代者」后，被隐藏的版本恢复可见。
//
// 否则 superseded_by 会指向一个不存在的 id，那本书既不在书架上、也没有界面能把它捞回来。
func TestDeleteReleasesHiddenBook(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)
	mustCreateReview(t, ctx, st, oldID, newID) // 新书被隐藏
	if err := st.DeleteBook(ctx, oldID); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}
	if got := visibleBookIDs(t, ctx, st); len(got) != 1 || got[0] != newID {
		t.Errorf("书架 = %v，期望被隐藏的新版本恢复可见 %d", got, newID)
	}
	if got := supersededOf(t, ctx, st, newID); got != 0 {
		t.Errorf("新版本 superseded_by = %d，期望 0", got)
	}
}
