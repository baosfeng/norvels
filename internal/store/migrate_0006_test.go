package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

// 本文件锁死 0006「移除字级分词与正文 FTS」的升级路径。
//
// v0.1.0 已发布，用户手上的库是旧结构（有 content_seg/title_seg 与 chapters_fts）。
// 升级必须做到：条数不变、结构干净、正文字节不变、失败即整体回滚。

// legacySeed 是灌进旧结构库的一批章节正文（含空章、超长章、非 ASCII 与生僻字）。
func legacySeed() []struct{ title, content string } {
	long := strings.Repeat("长夜书简的正文段落，青川剑砚在灯下展开。", 400)
	return []struct{ title, content string }{
		{"第一章 风起", "长夜书简，灯下无人。他推开窗，风从青川吹来。"},
		{"第二章 云涌", ""},
		{"第三章 归途", "青川剑砚合上了。远处有马蹄声，近了，又远了。"},
		{"第四章 长卷", long},
		{"番外 残页", "残页上只剩三个字：未完成。"},
	}
}

// seedV5Database 建一个停在版本 5（= v0.1.0 结构）的库，并灌入带分词镜像列的章节。
func seedV5Database(t *testing.T) (*Store, context.Context, int64) {
	t.Helper()
	ctx := context.Background()
	st, _ := openBare(t, "v5.db")
	if _, err := st.MigrateTo(ctx, 5); err != nil {
		t.Fatalf("迁到版本 5: %v", err)
	}
	bookID := insertLegacyBook(t, ctx, st)
	insertLegacyChapters(t, ctx, st, bookID)
	return st, ctx, bookID
}

// insertLegacyBook 写入一本书与一个作者，返回 book id。
func insertLegacyBook(t *testing.T, ctx context.Context, st *Store) int64 {
	t.Helper()
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO authors(name) VALUES ('示例作者')"); err != nil {
		t.Fatalf("插入作者: %v", err)
	}
	res, err := st.DB().ExecContext(ctx,
		"INSERT INTO books(title, author_id, source_path, source_name, file_size, file_sha256, "+
			"encoding, word_count, chapter_count, volume_count, intro, status) "+
			"VALUES ('长夜书简', 1, '/path/to/txt/a.txt', 'a.txt', 1024, ?, 'utf-8', 500, 5, 0, '简介', 'parsed')",
		strings.Repeat("a", 64))
	if err != nil {
		t.Fatalf("插入书籍: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("取 book id: %v", err)
	}
	return id
}

// insertLegacyChapters 按旧结构写入章节，分词镜像列用与生产一致的「逐字加空格」形态填充。
func insertLegacyChapters(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	for i, c := range legacySeed() {
		if _, err := st.DB().ExecContext(ctx,
			"INSERT INTO chapters(book_id, volume_id, idx, title, title_seg, content, content_seg, word_count) "+
				"VALUES (?, 0, ?, ?, ?, ?, ?, ?)",
			bookID, i, c.title, legacySegment(c.title), c.content, legacySegment(c.content),
			len([]rune(c.content))); err != nil {
			t.Fatalf("插入章节 %d: %v", i, err)
		}
	}
}

// legacySegment 复刻 0006 之前 store.Segment 的行为（CJK 逐字加空格），
// 目的是让测试数据「像」真实旧库，而不是随便填一串占位符。
func legacySegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(r)
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) || (r >= 0xF900 && r <= 0xFAFF) {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// chapterBodyDigest 返回全库章节正文的指纹（id + 原文，按 id 排序后哈希）。
//
// 用指纹而不是逐条比对，是为了让断言「整库正文一个字节都没变」足够强，
// 又不必在失败信息里倾倒整本书。
func chapterBodyDigest(t *testing.T, ctx context.Context, st *Store) string {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx, "SELECT id, content FROM chapters ORDER BY id")
	if err != nil {
		t.Fatalf("读正文: %v", err)
	}
	defer rows.Close()
	h := sha256.New()
	n := 0
	for rows.Next() {
		var (
			id      int64
			content string
		)
		if err := rows.Scan(&id, &content); err != nil {
			t.Fatalf("扫描正文: %v", err)
		}
		fmt.Fprintf(h, "%d\x00%s\x00", id, content)
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历正文: %v", err)
	}
	if n == 0 {
		t.Fatal("指纹覆盖了 0 章，断言会退化为恒真")
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestMigrate0006KeepsDataAndDropsFTS 是本次改动最关键的断言。
func TestMigrate0006KeepsDataAndDropsFTS(t *testing.T) {
	st, ctx, bookID := seedV5Database(t)

	before := snapshotCounts(t, ctx, st)
	beforeDigest := chapterBodyDigest(t, ctx, st)
	assertLegacyStructurePresent(t, ctx, st)

	// 只迁到 0006：本测试锁的是「移除分词与 FTS」这一步，
	// 0007（正文改块压缩）由 migrate_0007_test.go 单独验证。
	status, err := st.MigrateTo(ctx, 6)
	if err != nil {
		t.Fatalf("升级到最新版本: %v", err)
	}
	if status.FailedVersion != 0 {
		t.Errorf("迁移失败于版本 %d", status.FailedVersion)
	}
	if status.ToVersion != 6 {
		t.Errorf("迁移后版本 = %d，期望 6", status.ToVersion)
	}

	assertCountsUnchanged(t, ctx, st, before)
	assertSameDigest(t, ctx, st, beforeDigest)
	assertLegacyStructureGone(t, ctx, st)
	assertChapterReadable(t, ctx, st, bookID)
}

// counts 是一组「数据没变」的计数快照。
type counts struct {
	books    int
	chapters int
	authors  int
	words    int64
}

// snapshotCounts 取迁移前的条数快照。
func snapshotCounts(t *testing.T, ctx context.Context, st *Store) counts {
	t.Helper()
	var c counts
	if err := st.DB().QueryRowContext(ctx,
		"SELECT (SELECT COUNT(*) FROM books), (SELECT COUNT(*) FROM chapters), "+
			"(SELECT COUNT(*) FROM authors), (SELECT COALESCE(SUM(word_count),0) FROM chapters)").Scan(
		&c.books, &c.chapters, &c.authors, &c.words); err != nil {
		t.Fatalf("取条数快照: %v", err)
	}
	if c.chapters == 0 || c.books == 0 {
		t.Fatal("旧库快照为 0 条，断言会退化为恒真")
	}
	return c
}

// assertCountsUnchanged 校验迁移前后条数与字数完全一致。
func assertCountsUnchanged(t *testing.T, ctx context.Context, st *Store, before counts) {
	t.Helper()
	after := snapshotCounts(t, ctx, st)
	if after != before {
		t.Errorf("条数漂移：迁移前 %+v，迁移后 %+v", before, after)
	}
}

// assertSameDigest 校验全库正文逐字节一致（核心断言）。
func assertSameDigest(t *testing.T, ctx context.Context, st *Store, before string) {
	t.Helper()
	after := chapterBodyDigest(t, ctx, st)
	if after != before {
		t.Fatalf("正文字节发生变化：迁移前 %s，迁移后 %s", before, after)
	}
}

// assertLegacyStructurePresent 确认造出来的确实是旧结构（否则测试没有意义）。
func assertLegacyStructurePresent(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	for _, col := range []string{"title_seg", "content_seg"} {
		if !columnExists(t, ctx, st, "chapters", col) {
			t.Fatalf("造数据失败：旧结构应含 %q 列", col)
		}
	}
	exists, err := st.tableExists(ctx, "chapters_fts")
	if err != nil || !exists {
		t.Fatalf("造数据失败：旧结构应含 chapters_fts（err=%v）", err)
	}
	var triggers int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name LIKE 'chapters_a%'").Scan(&triggers); err != nil {
		t.Fatalf("查触发器: %v", err)
	}
	if triggers != 3 {
		t.Fatalf("造数据失败：应有 3 个同步触发器，实际 %d", triggers)
	}
}

// assertLegacyStructureGone 校验分词列、FTS 表与触发器全部消失。
func assertLegacyStructureGone(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	for _, col := range []string{"title_seg", "content_seg"} {
		if columnExists(t, ctx, st, "chapters", col) {
			t.Errorf("升级后 chapters 仍有 %q 列", col)
		}
	}
	var leftovers int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE name LIKE 'chapters_fts%'").Scan(&leftovers); err != nil {
		t.Fatalf("查 FTS 残骸: %v", err)
	}
	if leftovers != 0 {
		t.Errorf("升级后仍有 %d 个 chapters_fts 相关对象", leftovers)
	}
	var triggers int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='trigger'").Scan(&triggers); err != nil {
		t.Fatalf("查触发器: %v", err)
	}
	if triggers != 0 {
		t.Errorf("升级后仍残留 %d 个触发器", triggers)
	}
}

// assertChapterReadable 校验升级后单章正文仍可读出，且与种子内容一致。
//
// 这里**直接查 content 列**而不是走 GetChapter：本测试只迁到版本 6，
// 此时正文还是原文 TEXT（0007 才改成块压缩），GetChapter 读的是块寻址列，尚不存在。
func assertChapterReadable(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx,
		"SELECT title, content FROM chapters WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		t.Fatalf("读章节: %v", err)
	}
	defer rows.Close()

	seed := legacySeed()
	i := 0
	for rows.Next() {
		var title, content string
		if err := rows.Scan(&title, &content); err != nil {
			t.Fatalf("扫描章节: %v", err)
		}
		if i >= len(seed) {
			t.Fatalf("章节数超出种子 %d", len(seed))
		}
		if content != seed[i].content {
			t.Errorf("第 %d 章正文不一致：got=%q want=%q", i, truncateForLog(content), truncateForLog(seed[i].content))
		}
		if title != seed[i].title {
			t.Errorf("第 %d 章标题 = %q，期望 %q", i, title, seed[i].title)
		}
		i++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历章节: %v", err)
	}
	if i != len(seed) {
		t.Errorf("章节数 = %d，期望 %d", i, len(seed))
	}
}

// truncateForLog 截断过长的正文，避免长章把测试输出刷屏。
func truncateForLog(s string) string {
	r := []rune(s)
	if len(r) <= 60 {
		return s
	}
	return string(r[:60]) + "…"
}
