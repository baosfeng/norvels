package importer

import (
	"context"
	"path/filepath"
	"testing"

	"norvels/internal/store"
)

// TestRunRetryZipEntry 重试 zip 条目：只解出该条目并导入。
func TestRunRetryZipEntry(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "pack.zip")
	writeTestZip(t, zipPath, "inner.txt", sampleText("包内重试书", "作者辛"))
	job, err := st.CreateImportJob(ctx, dir)
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	res, err := Run(ctx, st, Options{Dir: dir, JobID: job.ID, Sources: []string{zipPath + ":inner.txt"}}, nil)
	if err != nil {
		t.Fatalf("Run 重试 zip: %v", err)
	}
	if len(res.Imported) != 1 {
		t.Fatalf("导入数 = %d，期望 1", len(res.Imported))
	}
	files, err := st.ListImportJobFiles(ctx, job.ID, "", 10)
	if err != nil || len(files.Files) != 1 || files.Files[0].Status != store.ImportFileStatusImported {
		t.Fatalf("文件终态 = %+v, err=%v，期望 1 条 imported", files.Files, err)
	}
}
