package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"norvels/internal/store"
)

// TestDryRunCounts dry-run 的分类计数与真实导入口径一致，且不写库。
func TestDryRunCounts(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "已导入.txt")
	writeTxtFile(t, a, sampleText("已导入", "作者甲"))
	if _, err := Run(ctx, st, Options{Dir: dir, Recursive: true}, nil); err != nil {
		t.Fatalf("首次导入: %v", err)
	}
	writeTxtFile(t, filepath.Join(dir, "新书.txt"), sampleText("新书", "作者乙"))
	if err := os.WriteFile(filepath.Join(dir, "坏.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写 pdf: %v", err)
	}

	rep, err := DryRun(ctx, st, Options{Dir: dir, Recursive: true})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	requireDryRunCounts(t, rep)
	if n := countBooks(t, st); n != 1 {
		t.Errorf("dry-run 后书籍数 = %d，期望仍为 1（不写库）", n)
	}
}

// requireDryRunCounts 断言预检的文件 / 书 / 样例计数。
func requireDryRunCounts(t *testing.T, rep *Report) {
	t.Helper()
	if rep.Files.Total != 3 || rep.Files.WillImport != 1 || rep.Files.Duplicate != 1 || rep.Files.Unsupported != 1 {
		t.Fatalf("files = %+v，期望 total=3 willImport=1 duplicate=1 unsupported=1", rep.Files)
	}
	if rep.Books.New != 1 || rep.Books.Duplicate != 1 {
		t.Fatalf("books = %+v，期望 new=1 duplicate=1", rep.Books)
	}
	if len(rep.Sample) != 2 || len(rep.Errors) != 1 {
		t.Fatalf("sample/errors 长度 = %d/%d，期望 2/1", len(rep.Sample), len(rep.Errors))
	}
}

// TestDryRunTruncated 目录超过 200 个候选文件时只解析前 200 并置 truncated。
func TestDryRunTruncated(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	for i := 0; i < dryRunMaxFiles+1; i++ {
		path := filepath.Join(dir, "f"+itoaTest(i)+".txt")
		writeTxtFile(t, path, "书名：批量"+itoaTest(i))
	}
	rep, err := DryRun(ctx, st, Options{Dir: dir, Recursive: true})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if !rep.Truncated {
		t.Error("超过 200 个文件应置 truncated")
	}
	if rep.Files.Total != dryRunMaxFiles {
		t.Errorf("files.total = %d，期望 %d", rep.Files.Total, dryRunMaxFiles)
	}
}

// writeTxtFile 写一份文本文件（失败即 Fatal）。
func writeTxtFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写 %s: %v", path, err)
	}
}

// countBooks 数库里的书籍数。
func countBooks(t *testing.T, st *store.Store) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM books").Scan(&n); err != nil {
		t.Fatalf("count books: %v", err)
	}
	return n
}

// itoaTest 是测试用的极小整数格式化。
func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}
