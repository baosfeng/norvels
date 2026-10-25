package store

import (
	"context"
	"testing"
)

// TestImportJobFileCountAndDefaultLimit 覆盖计数（空状态 = 全部）与默认 limit 分支。
func TestImportJobFileCountAndDefaultLimit(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	jobID := seedJob(t, st)
	if err := st.UpsertImportJobFile(ctx, jobID, ImportJobFile{
		Source: "/books/one.txt", DisplayPath: "one.txt", Status: ImportFileStatusFailed,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if n, err := st.CountImportJobFiles(ctx, jobID, ""); err != nil || n != 1 {
		t.Fatalf("CountImportJobFiles(空状态) = %d, err=%v，期望 1", n, err)
	}
	list, err := st.ListImportJobFiles(ctx, jobID, "", 0)
	if err != nil || len(list.Files) != 1 {
		t.Fatalf("limit=0（默认）列表 = %+v, err=%v", list, err)
	}
	if _, err := st.FailedImportJobFileSources(ctx, 0); err == nil {
		t.Error("jobID=0 的失败源查询应当报错")
	}
	if _, err := st.ListImportJobFiles(ctx, 0, "", 10); err == nil {
		t.Error("jobID=0 的文件列表查询应当报错")
	}
	if _, err := st.CountImportJobFiles(ctx, 0, ""); err == nil {
		t.Error("jobID=0 的计数查询应当报错")
	}
}
