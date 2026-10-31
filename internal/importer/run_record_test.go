package importer

import (
	"context"
	"testing"

	"norvels/internal/store"
)

// TestRecordJobFile 文件级终态的四条路径：无任务（dry-run）、空源、正常写入、写库失败。
//
// 这组分支原本只在整批导入里「碰巧走到」，全量并行跑时偶发漏掉写失败分支，
// 让 importer 覆盖率在 90.3/90.4 之间抖动；这里显式覆盖，口径不再依赖时序。
func TestRecordJobFile(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()

	// 1) job 为 nil（dry-run）：空操作，不 panic。
	(&runState{st: st, res: &Result{}}).
		recordJobFile(ctx, "/books/示例.txt", "示例.txt", store.ImportFileStatusImported, "", 1)

	// 2) job 非空但 source 为空：空操作。
	job, err := st.CreateImportJob(ctx, dir)
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	state := &runState{st: st, job: job, res: &Result{}}
	state.recordJobFile(ctx, "", "示例.txt", store.ImportFileStatusImported, "", 1)

	// 3) 正常写入：任务明细里出现该条终态。
	state.recordJobFile(ctx, "/books/示例.txt", "示例.txt", store.ImportFileStatusImported, "", 7)
	files, err := st.ListImportJobFiles(ctx, job.ID, "", 10)
	if err != nil {
		t.Fatalf("ListImportJobFiles: %v", err)
	}
	if len(files.Files) != 1 || files.Files[0].Status != store.ImportFileStatusImported {
		t.Fatalf("文件终态未落库: %+v", files.Files)
	}

	// 4) 写库失败（任务不存在）：只记 lastErr，不 panic、不影响整批导入。
	bad := &runState{st: st, job: &store.ImportJob{ID: 999999}, res: &Result{}}
	bad.recordJobFile(ctx, "/books/示例.txt", "示例.txt", store.ImportFileStatusImported, "", 1)
	if bad.lastErr == "" {
		t.Error("写库失败应记 lastErr")
	}
}
