package store

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
)

// 0013 的口径：老库同书重名卷按 (idx, id) 保留首个、其余改成「原标题 (n)」，
// 改名条数进迁移日志，最后建表达式唯一索引；重跑幂等。

// TestMigrate0013DedupesVolumeTitles 用含重名卷的 v12 库跑 0013。
func TestMigrate0013DedupesVolumeTitles(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "v12-dupes.db")
	if _, err := st.MigrateTo(ctx, 12); err != nil {
		t.Fatalf("迁到 0012: %v", err)
	}
	bookA := seedDupVolumes(t, ctx, st, "/books/dup-a.txt", "a13",
		[]string{"卷一", " 卷一", "卷一 (2)", "卷二", "卷一"})
	bookB := seedDupVolumes(t, ctx, st, "/books/dup-b.txt", "b13",
		[]string{"第二部", " 第二部", "第二部"})

	logs := captureMigrationLog(t)
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("0012 → 最新: %v", err)
	}
	requireVolumeUniqueIndex(t, ctx, st)
	// (idx,id) 顺序保留首个：idx0 的「卷一」不动；idx1 的「 卷一」要跳过本书
	// 真实存在的「卷一 (2)」，改成「卷一 (3)」；idx4 接着用「卷一 (4)」。
	requireVolumeTitles(t, ctx, st, bookA, []string{"卷一", "卷一 (3)", "卷一 (2)", "卷二", "卷一 (4)"})
	requireVolumeTitles(t, ctx, st, bookB, []string{"第二部", "第二部 (2)", "第二部 (3)"})
	requireLogLine(t, logs(), "迁移 0013：同书卷名去重改名 4 条")
	t.Logf("迁移日志：%s", strings.TrimSpace(logs()))
}

// TestMigrate0013Idempotent 删掉版本记录重跑：不改标题、不重建索引、日志报 0 条。
func TestMigrate0013Idempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := seedDupVolumesAfterMigrate(t, ctx, st)
	before := volumeTitlesOf(t, ctx, st, book)

	logs := captureMigrationLog(t)
	mustExec(t, ctx, st, "DELETE FROM schema_migrations WHERE version >= 13")
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("重跑 0013（应为幂等）: %v", err)
	}
	requireVolumeTitles(t, ctx, st, book, before)
	requireVolumeUniqueIndex(t, ctx, st)
	requireLogLine(t, logs(), "改名 0 条")
}

// TestMigrate0013FromCleanLibraryHasNoRenames 全新库的 0013 只建索引，日志报 0 条。
func TestMigrate0013FromCleanLibraryHasNoRenames(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "fresh-0013.db")
	logs := captureMigrationLog(t)
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("空库迁移: %v", err)
	}
	requireVolumeUniqueIndex(t, ctx, st)
	requireLogLine(t, logs(), "改名 0 条")
}

// seedDupVolumes 按给定卷名顺序写一本 v12 结构的书（idx = 0..n-1），返回 bookID。
func seedDupVolumes(t *testing.T, ctx context.Context, st *Store, path, sha string, titles []string) int64 {
	t.Helper()
	mustExec(t, ctx, st,
		"INSERT INTO books(title, source_path, file_sha256) VALUES ('重名卷书', '"+path+"', '"+sha+"')")
	var bookID int64
	if err := st.DB().QueryRowContext(ctx,
		"SELECT id FROM books WHERE source_path = ?", path).Scan(&bookID); err != nil {
		t.Fatalf("读 book id: %v", err)
	}
	for idx, title := range titles {
		if _, err := st.DB().ExecContext(ctx,
			"INSERT INTO volumes(book_id, idx, title) VALUES (?, ?, ?)", bookID, idx, title); err != nil {
			t.Fatalf("插卷 %q: %v", title, err)
		}
	}
	return bookID
}

// seedDupVolumesAfterMigrate 在已迁到最新的库上造两卷（唯一索引已生效，名字必须不重复）。
func seedDupVolumesAfterMigrate(t *testing.T, ctx context.Context, st *Store) int64 {
	t.Helper()
	bookID := seedDupVolumes(t, ctx, st, "/books/after13.txt", "c13", []string{"上部", "下部"})
	return bookID
}

// volumeTitlesOf 按 (idx, id) 读一本书的卷名。
func volumeTitlesOf(t *testing.T, ctx context.Context, st *Store, bookID int64) []string {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx,
		"SELECT title FROM volumes WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		t.Fatalf("读卷名: %v", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	var out []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			t.Fatalf("扫描卷名: %v", err)
		}
		out = append(out, title)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读卷名: %v", err)
	}
	return out
}

// requireVolumeTitles 断言整本书的卷名序列（含顺序）。
func requireVolumeTitles(t *testing.T, ctx context.Context, st *Store, bookID int64, want []string) {
	t.Helper()
	got := volumeTitlesOf(t, ctx, st, bookID)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("卷名 = %q，期望 %q", got, want)
	}
}

// requireVolumeUniqueIndex 断言 0013 的唯一索引存在且是唯一索引。
func requireVolumeUniqueIndex(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_index_list('volumes') WHERE name = ? AND \"unique\" = 1",
		volumeTitleUniqueIndex).Scan(&n); err != nil || n != 1 {
		t.Fatalf("唯一索引 %s 缺失或非唯一: n=%d err=%v", volumeTitleUniqueIndex, n, err)
	}
}

// requireLogLine 断言捕获到的日志里有指定子串（迁移日志口径）。
func requireLogLine(t *testing.T, logs, want string) {
	t.Helper()
	if !strings.Contains(logs, want) {
		t.Fatalf("迁移日志缺 %q，实际：%s", want, logs)
	}
}

// captureMigrationLog 捕获标准日志，返回「读取当前日志」的函数（断言时再读）。
func captureMigrationLog(t *testing.T) func() string {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf.String
}
