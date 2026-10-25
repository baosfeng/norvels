package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDryRunNonRecursive 覆盖非递归扫描（readTop）与空文本解析失败（failed 分类）。
func TestDryRunNonRecursive(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxtFile(t, filepath.Join(dir, "空.txt"), "")
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeTxtFile(t, filepath.Join(sub, "子目录.txt"), sampleText("子书", "作者丙"))

	rep, err := DryRun(ctx, st, Options{Dir: dir, Recursive: false})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if rep.Files.Total != 1 || rep.Files.Failed != 1 || len(rep.Errors) != 1 {
		t.Fatalf("非递归预检 = %+v / errors=%+v，期望只解析顶层空文件并记 failed", rep.Files, rep.Errors)
	}
}

// TestDryRunZipUnsupported 覆盖 zip 内不支持条目的 unsupported 计数（整包仍有可导入条目）。
func TestDryRunZipUnsupported(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeMixedZip(t, filepath.Join(dir, "pack.zip"))

	rep, err := DryRun(ctx, st, Options{Dir: dir, Recursive: true})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if rep.Files.Unsupported != 1 || rep.Files.WillImport != 1 || len(rep.Errors) != 1 {
		t.Fatalf("zip 预检 = %+v / %+v，期望 unsupported=1 willImport=1 与 1 条错误", rep.Files, rep.Errors)
	}
}

// writeMixedZip 造一个同时含可导入 txt 与不支持 pdf 的 zip。
func writeMixedZip(t *testing.T, path string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	good, err := zw.Create("inner.txt")
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := good.Write([]byte(sampleText("包内书", "作者丁"))); err != nil {
		t.Fatalf("zip.Write: %v", err)
	}
	bad, err := zw.Create("说明.pdf")
	if err != nil {
		t.Fatalf("zip.Create pdf: %v", err)
	}
	if _, err := bad.Write([]byte("x")); err != nil {
		t.Fatalf("zip.Write pdf: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写 zip: %v", err)
	}
}

// TestDryRunForceNotDuplicate force 覆盖时已存在的书不算重复。
func TestDryRunForceNotDuplicate(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxtFile(t, filepath.Join(dir, "书.txt"), sampleText("强制书", "作者戊"))
	if _, err := Run(ctx, st, Options{Dir: dir, Recursive: true}, nil); err != nil {
		t.Fatalf("首次导入: %v", err)
	}
	rep, err := DryRun(ctx, st, Options{Dir: dir, Recursive: true, Force: true})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if rep.Files.Duplicate != 0 || rep.Files.WillImport != 1 || rep.Books.New != 1 {
		t.Fatalf("force 预检 = %+v / %+v，期望不判重复", rep.Files, rep.Books)
	}
}

// TestSourcesToCandidatesZipMissing 覆盖 zip 条目缺失的 issue 与展示路径。
func TestSourcesToCandidatesZipMissing(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "pack.zip")
	writeTestZip(t, zipPath, "inner.txt", "书名：包内\n作者：作者\n第一章 起\n正文。\n")
	scratch, err := makeScratch("")
	if err != nil {
		t.Fatalf("makeScratch: %v", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	cands, issues := sourcesToCandidates([]string{zipPath + ":not-there.txt"}, scratch)
	if len(cands) != 0 || len(issues) != 1 {
		t.Fatalf("cands/issues = %d/%d，期望 0/1", len(cands), len(issues))
	}
	if issues[0].path != "pack.zip:not-there.txt" {
		t.Errorf("展示路径 = %q，期望「包名:包内路径」", issues[0].path)
	}
}
