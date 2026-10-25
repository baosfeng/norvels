package importer

import (
	"context"
	"path/filepath"
	"testing"

	"norvels/internal/store"
)

// TestRunMinChaptersSkips 章节数低于 MinChapters 的文件记 skipped 终态、不入库。
func TestRunMinChaptersSkips(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxtFile(t, filepath.Join(dir, "短书.txt"), sampleText("短书", "作者己"))
	job, err := st.CreateImportJob(ctx, dir)
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	res, err := Run(ctx, st, Options{Dir: dir, Recursive: true, JobID: job.ID, MinChapters: 100}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Imported) != 0 || res.Skipped != 1 {
		t.Fatalf("导入/跳过 = %d/%d，期望 0/1", len(res.Imported), res.Skipped)
	}
	files, err := st.ListImportJobFiles(ctx, job.ID, "", 10)
	if err != nil || len(files.Files) != 1 || files.Files[0].Status != store.ImportFileStatusSkipped {
		t.Fatalf("文件终态 = %+v, err=%v，期望 1 条 skipped", files.Files, err)
	}
}
