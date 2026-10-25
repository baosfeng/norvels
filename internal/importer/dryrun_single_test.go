package importer

import (
	"context"
	"path/filepath"
	"testing"
)

// TestDryRunSingleFile dir 直接指向单个 txt 时也能预检（与真实导入的 dir 语义一致）。
func TestDryRunSingleFile(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	path := filepath.Join(t.TempDir(), "单本.txt")
	writeTxtFile(t, path, sampleText("单本预检", "作者庚"))
	rep, err := DryRun(ctx, st, Options{Dir: path})
	if err != nil {
		t.Fatalf("DryRun 单文件: %v", err)
	}
	if rep.Files.Total != 1 || rep.Files.WillImport != 1 || rep.Books.New != 1 {
		t.Fatalf("单文件预检 = %+v / %+v，期望 1 本将导入", rep.Files, rep.Books)
	}
}
