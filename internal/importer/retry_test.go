package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeTestZip 造一个只含单个条目的 zip。
func writeTestZip(t *testing.T, path, inner, body string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(inner)
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("zip.Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写 zip: %v", err)
	}
}

// TestSourcesToCandidates 覆盖三种源：普通文件、zip 条目、已不存在的源。
func TestSourcesToCandidates(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, "a.txt")
	writeTxtFile(t, txt, sampleText("重试之书", "作者甲"))
	zipPath := filepath.Join(dir, "pack.zip")
	writeTestZip(t, zipPath, "inner.txt", sampleText("包内书", "作者乙"))
	scratch, err := makeScratch("")
	if err != nil {
		t.Fatalf("makeScratch: %v", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	cands, issues := sourcesToCandidates([]string{txt, zipPath + ":inner.txt", filepath.Join(dir, "missing.txt")}, scratch)
	if len(cands) != 2 {
		t.Fatalf("候选数 = %d，期望 2: %+v", len(cands), cands)
	}
	if cands[1].source != zipPath+":inner.txt" {
		t.Errorf("zip 条目 source = %q，期望可重放源", cands[1].source)
	}
	if len(issues) != 1 || issues[0].kind != issueKindFailed {
		t.Fatalf("缺失源的 issue = %+v，期望 1 条 failed", issues)
	}
}

// TestRunRetryOnlyProcessesSources 重试只处理给定源，其余文件不导入。
func TestRunRetryOnlyProcessesSources(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	writeTxtFile(t, a, sampleText("重试甲", "作者甲"))
	writeTxtFile(t, b, sampleText("重试乙", "作者乙"))
	job, err := st.CreateImportJob(ctx, dir)
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}

	res, err := Run(ctx, st, Options{Dir: dir, JobID: job.ID, Sources: []string{a}}, nil)
	if err != nil {
		t.Fatalf("Run 重试: %v", err)
	}
	if len(res.Imported) != 1 {
		t.Fatalf("导入数 = %d，期望 1（只处理 a.txt）", len(res.Imported))
	}
	files, err := st.ListImportJobFiles(ctx, job.ID, "", 10)
	if err != nil || len(files.Files) != 1 || files.Files[0].Source != a {
		t.Fatalf("文件级终态 = %+v, err=%v，期望只有 a.txt", files.Files, err)
	}
	if n := countBooks(t, st); n != 1 {
		t.Errorf("书籍数 = %d，期望 1（b.txt 未被导入）", n)
	}
}
