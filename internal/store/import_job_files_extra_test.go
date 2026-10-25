package store

import (
	"context"
	"testing"
)

// TestImportJobFileStatuses 批量查源状态（重试入口的校验依据）。
func TestImportJobFileStatuses(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	jobID := seedJob(t, st)
	for _, f := range []ImportJobFile{
		{Source: "/books/a.txt", DisplayPath: "a.txt", Status: ImportFileStatusFailed},
		{Source: "/books/b.txt", DisplayPath: "b.txt", Status: ImportFileStatusImported, BookID: 3},
	} {
		if err := st.UpsertImportJobFile(ctx, jobID, f); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}
	statuses, err := st.ImportJobFileStatuses(ctx, jobID, []string{"/books/a.txt", "/books/b.txt", "/books/x.txt"})
	if err != nil {
		t.Fatalf("ImportJobFileStatuses: %v", err)
	}
	if statuses["/books/a.txt"] != ImportFileStatusFailed || statuses["/books/b.txt"] != ImportFileStatusImported {
		t.Errorf("statuses = %v", statuses)
	}
	if _, ok := statuses["/books/x.txt"]; ok {
		t.Errorf("不存在的源不该出现在结果里: %v", statuses)
	}
	if _, err := st.ImportJobFileStatuses(ctx, 0, nil); err == nil {
		t.Error("jobID=0 应当报错")
	}
}
