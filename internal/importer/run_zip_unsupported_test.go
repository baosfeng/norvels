package importer

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"norvels/internal/store"
)

// TestRunZipUnsupportedRecordsFiles 目录导入里整包只有不支持条目时，文件级终态记为 unsupported。
func TestRunZipUnsupportedRecordsFiles(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTestZip(t, filepath.Join(dir, "pack.zip"), "说明.pdf", "x")
	job, err := st.CreateImportJob(ctx, dir)
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	if _, err := Run(ctx, st, Options{Dir: dir, Recursive: true, JobID: job.ID}, nil); !errors.Is(err, ErrNoFiles) {
		t.Fatalf("Run err = %v，期望 ErrNoFiles（包内无条目可导入）", err)
	}
	files, err := st.ListImportJobFiles(ctx, job.ID, "", 10)
	if err != nil || len(files.Files) != 1 || files.Files[0].Status != store.ImportFileStatusFailed {
		t.Fatalf("文件终态 = %+v, err=%v，期望 1 条 failed", files.Files, err)
	}
}
