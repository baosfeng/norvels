package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 0022 预计算的三条不变量，每条都有测试守着：
//  1. **增量 == 全量**：导入/删改路径的增量维护结果，必须与「清空后重算」逐项相等；
//  2. **缓存 == 实时**：读接口走预计算表与走原实时 SQL 必须给出同一组数字；
//  3. **缺失不 500**：缓存不可用时接口降级并标 stale，不返回错误。

// TestPrecomputeIncrementalMatchesFull 是本次改造最重要的守恒测试。
//
// 做法：正常走导入/覆盖/删除/编辑/拆分/合并/删书等写路径，记下增量维护出的三张表；
// 再清空它们按全量重算一遍，两次结果必须逐项相等。任何一处漏维护（比如删书忘了减
// 章数、编辑忘了重算直方图）都会在这里露头。
func TestPrecomputeIncrementalMatchesFull(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)

	before := readPrecomputeSnapshot(t, ctx, st)
	if err := st.RefreshMetrics(ctx); err != nil {
		t.Fatalf("RefreshMetrics: %v", err)
	}
	after := readPrecomputeSnapshot(t, ctx, st)
	assertSnapshotsEqual(t, "增量 vs 全量", before, after)
}

// seedDashboardBooks 建出足够覆盖各条写路径的样本，并把每条路径都跑一遍。
func seedDashboardBooks(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	// sha 用不同字符起头：repeat64 只取首字符，同为 'p' 会让第二本被当成同文件幂等跳过。
	id1 := savePrecomputeBook(t, st, samplePrecomputeBook("预计算一号", "作者甲", "/books/pre1.txt", "a"))
	id2 := savePrecomputeBook(t, st, samplePrecomputeBook("预计算二号", "作者乙", "/books/pre2.txt", "b"))

	// 覆盖写入（--force）：旧章节与直方图要先减掉再重加。
	replacePrecomputeBook(t, st, samplePrecomputeBook("预计算一号（改）", "作者丙", "/books/pre1.txt", "a"))

	// 追更：只追加尾部章节，章数与直方图都要 +。
	appendPrecomputeChapter(t, st, id2)

	// 单章正文替换（字数变、章数不变）。
	chs := precomputeChapters(t, st, id2)
	// 替换用的正文也要带换行：后面还要在这章上做拆分，一段到底的正文切不开。
	if err := st.UpdateChapterContent(ctx, id2, chs[0].ID, repeatParagraph("替换后的正文内容。", 40, 8)); err != nil {
		t.Fatalf("UpdateChapterContent: %v", err)
	}
	// 单章删除。
	if err := st.DeleteChapter(ctx, id2, chs[len(chs)-1].ID); err != nil {
		t.Fatalf("DeleteChapter: %v", err)
	}
	// 插入新章 + 批量删除。
	insertPrecomputeChapter(t, st, id2)
	left := precomputeChapters(t, st, id2)
	if err := st.DeleteChapters(ctx, id2, []int64{left[len(left)-1].ID}); err != nil {
		t.Fatalf("DeleteChapters: %v", err)
	}
	// 拆章（1 → 2）与合章（2 → 1），两条路径对直方图的影响相反。
	splitPrecomputeChapter(t, st, id2)
	mergePrecomputeChapters(t, st, id2)
	// 删书：章数、直方图、作者聚合都要回落。
	if err := st.DeleteBook(ctx, id1); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}
}

// readPrecomputeSnapshot 读出三张预计算表的当前值（用于两次比对）。
func readPrecomputeSnapshot(t *testing.T, ctx context.Context, st *Store) precomputeSnapshot {
	t.Helper()
	var snap precomputeSnapshot
	rows, err := st.DB().QueryContext(ctx,
		"SELECT bucket, count FROM "+tableChapterLengthHist+" ORDER BY bucket")
	if err != nil {
		t.Fatalf("读直方图: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var bucket, count int
		if err := rows.Scan(&bucket, &count); err != nil {
			t.Fatalf("扫描直方图: %v", err)
		}
		snap.hist = append(snap.hist, count)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("直方图 rows: %v", err)
	}
	snap.daily = readDailyRows(t, ctx, st)
	snap.authors = readAuthorAggRows(t, ctx, st)
	return snap
}

// precomputeSnapshot 是三张预计算表的一次快照。
type precomputeSnapshot struct {
	hist    []int
	daily   map[string][2]int // 本地日 → (书数, 章数)
	authors map[int64][2]int  // 作者 id → (作品数, 字数)
}

// readDailyRows 读 import_daily。
func readDailyRows(t *testing.T, ctx context.Context, st *Store) map[string][2]int {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx, "SELECT day, book_count, chapter_count FROM "+tableImportDaily)
	if err != nil {
		t.Fatalf("读 import_daily: %v", err)
	}
	defer rows.Close()
	out := make(map[string][2]int, 16)
	for rows.Next() {
		var (
			day      string
			books    int
			chapters int
		)
		if err := rows.Scan(&day, &books, &chapters); err != nil {
			t.Fatalf("扫描 import_daily: %v", err)
		}
		out[day] = [2]int{books, chapters}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("import_daily rows: %v", err)
	}
	return out
}

// readAuthorAggRows 读 author_agg。
func readAuthorAggRows(t *testing.T, ctx context.Context, st *Store) map[int64][2]int {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx, "SELECT author_id, book_count, word_count FROM "+tableAuthorAgg)
	if err != nil {
		t.Fatalf("读 author_agg: %v", err)
	}
	defer rows.Close()
	out := make(map[int64][2]int, 16)
	for rows.Next() {
		var (
			id    int64
			books int
			words int
		)
		if err := rows.Scan(&id, &books, &words); err != nil {
			t.Fatalf("扫描 author_agg: %v", err)
		}
		out[id] = [2]int{books, words}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("author_agg rows: %v", err)
	}
	return out
}

// assertSnapshotsEqual 逐项比对两次快照。
func assertSnapshotsEqual(t *testing.T, label string, want, got precomputeSnapshot) {
	t.Helper()
	if !equalInts(want.hist, got.hist) {
		t.Errorf("%s：长度直方图 = %v，期望 %v", label, got.hist, want.hist)
	}
	if len(want.daily) != len(got.daily) {
		t.Errorf("%s：import_daily 天数 = %d，期望 %d（got=%v want=%v）",
			label, len(got.daily), len(want.daily), got.daily, want.daily)
	}
	for day, w := range want.daily {
		if g, ok := got.daily[day]; !ok || g != w {
			t.Errorf("%s：import_daily[%s] = %v，期望 %v", label, day, g, w)
		}
	}
	if len(want.authors) != len(got.authors) {
		t.Errorf("%s：author_agg 行数 = %d，期望 %d", label, len(got.authors), len(want.authors))
	}
	for id, w := range want.authors {
		if g, ok := got.authors[id]; !ok || g != w {
			t.Errorf("%s：author_agg[%d] = %v，期望 %v", label, id, g, w)
		}
	}
}

// equalInts 比较两个切片是否逐项相等（nil 与空切片视为相等）。
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------- 断言辅助 ----------

// mustSaveBook 新建一本书并要求成功。
func savePrecomputeBook(t *testing.T, st *Store, b *model.ParsedBook) int64 {
	t.Helper()
	res, err := st.SaveBook(context.Background(), b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook(%s): %v", b.Title, err)
	}
	return res.BookID
}

// mustSaveBookForce 覆盖写入并要求成功。
func replacePrecomputeBook(t *testing.T, st *Store, b *model.ParsedBook) int64 {
	t.Helper()
	res, err := st.SaveBook(context.Background(), b, SaveOptions{Force: true})
	if err != nil {
		t.Fatalf("SaveBook(force %s): %v", b.Title, err)
	}
	return res.BookID
}

// mustListChapters 取一本书的章节列表（按目录序）。
func precomputeChapters(t *testing.T, st *Store, bookID int64) []ChapterBrief {
	t.Helper()
	page, err := st.ListChapters(context.Background(), ChapterQuery{BookID: bookID, Limit: 200})
	if err != nil {
		t.Fatalf("ListChapters(%d): %v", bookID, err)
	}
	if len(page.Items) == 0 {
		t.Fatalf("书 %d 没有章节", bookID)
	}
	return page.Items
}

// repeatParagraph 造出「paragraphs 段、每段 times 次重复」的正文（段间用换行分隔）。
func repeatParagraph(text string, times, paragraphs int) string {
	line := strings.Repeat(text, times)
	lines := make([]string, paragraphs)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// voidBookID 吞掉一个 book id：追更路径按 source_path / file_sha256 找书，
// 调用方传 id 只是为了在用例里表达「给这本书追更」。
func voidBookID(int64) {}

// samplePrecomputeBook 构造一本用于预计算测试的书（3 章，章节长短分档）。
func samplePrecomputeBook(title, author, path, sha string) *model.ParsedBook {
	b := sampleBook(title, author, path, repeat64(sha))
	// WordCount 归零：sampleBook 的样本值（30）与这里的正文对不上，留着会让
	// 「全书字数」与「各章字数之和」从一开始就不一致，后面所有聚合比对都失去意义。
	b.WordCount = 0
	// 带换行：SplitChapter 的切点按行边界算，一段到底的正文切不开。
	b.Chapters = []model.ParsedChapter{
		{Idx: 0, VolumeIdx: -1, Title: "第1章 短", Content: repeatParagraph("短。", 10, 10)},
		{Idx: 1, VolumeIdx: -1, Title: "第2章 中", Content: repeatParagraph("中等的正文内容。", 40, 10)},
		{Idx: 2, VolumeIdx: -1, Title: "第3章 长", Content: repeatParagraph("很长的正文内容段落。", 90, 10)},
	}
	return b
}

// appendOneChapter 走追更路径追加一章。
func appendPrecomputeChapter(t *testing.T, st *Store, bookID int64) {
	t.Helper()
	ctx := context.Background()
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: bookID, Limit: 200})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	// 追更要求新序列以旧序列为前缀（按标题全等比对）：已有各章原样带上（含正文，
	// 否则字数会变），只在末尾追加一章。
	b := samplePrecomputeBook("预计算二号", "作者乙", "/books/pre2.txt", "b")
	keep := append([]model.ParsedChapter(nil), b.Chapters...)
	if len(keep) < len(page.Items) {
		t.Fatalf("样本只有 %d 章，库里已有 %d 章", len(keep), len(page.Items))
	}
	for i := range page.Items {
		keep[i].Title = page.Items[i].Title
	}
	b.Chapters = append(keep, model.ParsedChapter{
		Idx: len(page.Items), VolumeIdx: -1, Title: "第4章 追更",
		Content: repeatParagraph("追更的正文。", 30, 8),
	})
	if _, err := st.SaveBook(ctx, b, SaveOptions{Update: true}); err != nil {
		t.Fatalf("追更 SaveBook: %v", err)
	}
	voidBookID(bookID)
}

// insertOneChapter 走单章插入路径追加一章。
func insertPrecomputeChapter(t *testing.T, st *Store, bookID int64) {
	t.Helper()
	if _, err := st.InsertChapter(context.Background(), bookID, model.ParsedChapter{
		Title: "第99章 插入", Content: strings.Repeat("插入的章节正文。", 300),
	}, 0); err != nil {
		t.Fatalf("InsertChapter: %v", err)
	}
}

// splitFirstChapter 把第一章拆成两段。
func splitPrecomputeChapter(t *testing.T, st *Store, bookID int64) {
	t.Helper()
	chs := precomputeChapters(t, st, bookID)
	// cutAfter 是**段落下标**（0 起），不是字节偏移：在第 3 段之后切开。
	if _, err := st.SplitChapter(context.Background(), bookID, chs[0].ID, []int{3}); err != nil {
		t.Fatalf("SplitChapter: %v", err)
	}
}

// mergeFirstTwoChapters 把前两章合并成一章。
func mergePrecomputeChapters(t *testing.T, st *Store, bookID int64) {
	t.Helper()
	chs := precomputeChapters(t, st, bookID)
	if len(chs) < 2 {
		t.Fatalf("只剩 %d 章，无法合并", len(chs))
	}
	if err := st.MergeChapters(context.Background(), bookID, []int64{chs[0].ID, chs[1].ID}); err != nil {
		t.Fatalf("MergeChapters: %v", err)
	}
}

var _ = errors.Is
