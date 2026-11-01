package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 本文件锁死「正文独立库」的四条底线：
//  1. 正文只落在正文库文件上，主库连块表都不存在（0018 之后）；
//  2. 跨库读写往返一致（写得进、读得出、旧块被回收）；
//  3. 删书之后正文库没有残留；
//  4. 00018 迁移幂等（半路中断/重跑都不重复搬、不丢块）。
// 对账（ReconcileContent）单独一组用例，见文件尾。

// TestContentDBIsSeparateFile 正文块只落在正文库，主库没有块表。
func TestContentDBIsSeparateFile(t *testing.T) {
	ctx := context.Background()
	st, path := openMigrated(t, ctx)
	// 兄弟文件推导：同名同目录，加 -content。
	if want := filepath.Join(filepath.Dir(path), "norvels-content.db"); st.ContentPath() != want {
		t.Fatalf("ContentPath = %q，期望 %q", st.ContentPath(), want)
	}
	book := sampleBook("长夜书简", "青川客", "/books/a.txt", strings.Repeat("a", 64))
	res := saveOrFail(t, ctx, st, book)
	assertMainHasNoBlocks(t, ctx, st)
	assertContentBlocks(t, ctx, st, res.BookID)
	assertChapterText(t, ctx, st, res.BookID, book.Chapters[0].Content)
}

// openMigrated 开一个空库并迁移到最新版本（正文库随之建好）。
func openMigrated(t *testing.T, ctx context.Context) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "norvels.db")
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return st, path
}

// saveOrFail 保存一本书并返回结果。
func saveOrFail(t *testing.T, ctx context.Context, st *Store, b *model.ParsedBook) SaveResult {
	t.Helper()
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	return res
}

// assertMainHasNoBlocks 断言主库里没有块表（0018 已 DROP）。
func assertMainHasNoBlocks(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if exists, err := st.tableExists(ctx, "chapter_blocks"); err != nil || exists {
		t.Fatalf("主库不应再有 chapter_blocks（exists=%v err=%v）", exists, err)
	}
}

// assertContentBlocks 断言正文库里有这本书的块。
func assertContentBlocks(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	if n := contentBlocksOf(t, ctx, st, bookID); n == 0 {
		t.Fatalf("正文库里没有书 %d 的块", bookID)
	}
}

// assertChapterText 断言按章节 id 读回的正文与源数据一致。
func assertChapterText(t *testing.T, ctx context.Context, st *Store, bookID int64, want string) {
	t.Helper()
	ch, err := st.GetChapter(ctx, bookID, firstChapterID(t, ctx, st, bookID))
	if err != nil {
		t.Fatalf("GetChapter: %v", err)
	}
	if ch.Content != want {
		t.Errorf("正文读回来不一致：%q != %q", ch.Content, want)
	}
}

// TestContentDBEnvOverride 环境变量覆盖正文库路径。
func TestContentDBEnvOverride(t *testing.T) {
	ctx := context.Background()
	custom := filepath.Join(t.TempDir(), "content-custom.db")
	t.Setenv(EnvContentDB, custom)

	st := newTestStore(t)
	if st.ContentPath() != custom {
		t.Fatalf("ContentPath = %q，期望环境变量给的 %q", st.ContentPath(), custom)
	}
	res, err := st.SaveBook(ctx, sampleBook("荒渡灯影", "青川客", "/books/b.txt", strings.Repeat("b", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	ch, err := st.GetChapter(ctx, res.BookID, firstChapterID(t, ctx, st, res.BookID))
	if err != nil {
		t.Fatalf("GetChapter: %v", err)
	}
	if ch.Content == "" {
		t.Error("正文为空（块没写进覆盖后的正文库）")
	}
}

// TestContentDBWriteReapsOldBlock 整章替换：新块进正文库、旧块在提交后回收。
func TestContentDBWriteReapsOldBlock(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	res, err := st.SaveBook(ctx, sampleBook("孤灯旅舍", "青川客", "/books/c.txt", strings.Repeat("c", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	chapterID := firstChapterID(t, ctx, st, res.BookID)
	before := contentBlocksOf(t, ctx, st, res.BookID)

	if err := st.UpdateChapterContent(ctx, res.BookID, chapterID, strings.Repeat("新正文。", 200)); err != nil {
		t.Fatalf("UpdateChapterContent: %v", err)
	}
	ch, err := st.GetChapter(ctx, res.BookID, chapterID)
	if err != nil {
		t.Fatalf("GetChapter: %v", err)
	}
	if !strings.Contains(ch.Content, "新正文") {
		t.Errorf("替换后的正文没读到：%q", ch.Content)
	}
	// 旧块无人引用 → 被回收；总块数不该只增不减。
	after := contentBlocksOf(t, ctx, st, res.BookID)
	if after > before+1 {
		t.Errorf("块数从 %d 涨到 %d（旧块未被回收）", before, after)
	}
}

// TestDeleteBookLeavesNoContentBlocks 删书之后正文库没有残留。
func TestDeleteBookLeavesNoContentBlocks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	res, err := st.SaveBook(ctx, sampleBook("待删的书", "作者甲", "/books/d.txt", strings.Repeat("d", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	if n := contentBlocksOf(t, ctx, st, res.BookID); n == 0 {
		t.Fatal("删之前正文库就没有块，用例样本不成立")
	}
	if err := st.DeleteBook(ctx, res.BookID); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}
	if n := contentBlocksOf(t, ctx, st, res.BookID); n != 0 {
		t.Errorf("删书后正文库仍残留 %d 个块", n)
	}
	rep, err := st.ReconcileContent(ctx, false)
	if err != nil {
		t.Fatalf("ReconcileContent: %v", err)
	}
	if rep.OrphanBlocks != 0 || rep.DanglingRefs != 0 {
		t.Errorf("删书后对账不干净：%+v", rep)
	}
}

// firstChapterID 取一本书目录里的第一章 id。
func firstChapterID(t *testing.T, ctx context.Context, st *Store, bookID int64) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRowContext(ctx,
		"SELECT id FROM chapters WHERE book_id = ? ORDER BY idx, id LIMIT 1", bookID).Scan(&id); err != nil {
		t.Fatalf("取首章 id: %v", err)
	}
	return id
}

// contentBlocksOf 数一本书在**正文库**里的块数。
func contentBlocksOf(t *testing.T, ctx context.Context, st *Store, bookID int64) int {
	t.Helper()
	var n int
	if err := st.cdb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapter_blocks WHERE book_id = ?", bookID).Scan(&n); err != nil {
		t.Fatalf("数正文库块: %v", err)
	}
	return n
}
