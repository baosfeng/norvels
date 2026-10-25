package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// seedJob 建一个 running 任务并返回 id。
func seedJob(t *testing.T, st *Store) int64 {
	t.Helper()
	job, err := st.CreateImportJob(context.Background(), "/books")
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	return job.ID
}

// TestUpsertImportJobFileIdempotent 同一 (job_id, source) 原地更新，不堆重复行。
func TestUpsertImportJobFileIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	jobID := seedJob(t, st)

	first := ImportJobFile{Source: "/books/a.txt", DisplayPath: "a.txt", Status: ImportFileStatusFailed,
		Reason: "解析失败", BookID: 0}
	if err := st.UpsertImportJobFile(ctx, jobID, first); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	second := ImportJobFile{Source: "/books/a.txt", DisplayPath: "a.txt", Status: ImportFileStatusImported,
		Reason: "", BookID: 7}
	if err := st.UpsertImportJobFile(ctx, jobID, second); err != nil {
		t.Fatalf("Upsert 第二次: %v", err)
	}
	list, err := st.ListImportJobFiles(ctx, jobID, "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Files) != 1 || list.Files[0].Status != ImportFileStatusImported || list.Files[0].BookID != 7 {
		t.Fatalf("幂等更新后 = %+v，期望 1 条且状态/书籍 id 已更新", list.Files)
	}
	if n, err := st.CountImportJobFiles(ctx, jobID, ImportFileStatusImported); err != nil || n != 1 {
		t.Errorf("Count(imported) = %d, err=%v", n, err)
	}
}

// TestImportJobFileValidation 覆盖非法入参：空源、非法状态、任务不存在。
func TestImportJobFileValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	jobID := seedJob(t, st)

	cases := []struct {
		name string
		job  int64
		file ImportJobFile
		want error
	}{
		{"空源", jobID, ImportJobFile{Source: " ", Status: ImportFileStatusFailed}, ErrInvalidArgument},
		{"非法状态", jobID, ImportJobFile{Source: "/books/a.txt", Status: "bogus"}, ErrInvalidArgument},
		{"任务不存在", 9999, ImportJobFile{Source: "/books/a.txt", Status: ImportFileStatusFailed}, ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := st.UpsertImportJobFile(ctx, tc.job, tc.file); !errors.Is(err, tc.want) {
				t.Errorf("err = %v，期望 %v", err, tc.want)
			}
		})
	}
	if _, err := st.ListImportJobFiles(ctx, 9999, "", 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("任务不存在时 List err = %v，期望 ErrNotFound", err)
	}
	if _, err := st.ListImportJobFiles(ctx, jobID, "bogus", 10); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("非法 status 时 List err = %v，期望 ErrInvalidArgument", err)
	}
}

// TestListImportJobFilesFilterAndTruncated 过滤与截断口径。
func TestListImportJobFilesFilterAndTruncated(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	jobID := seedJob(t, st)
	statuses := []string{ImportFileStatusFailed, ImportFileStatusImported, ImportFileStatusFailed}
	for i, status := range statuses {
		src := "/books/" + strings.Repeat("x", i+1) + ".txt"
		if err := st.UpsertImportJobFile(ctx, jobID, ImportJobFile{
			Source: src, DisplayPath: "x.txt", Status: status,
		}); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}
	requireJobFileList(t, ctx, st, jobID, ImportFileStatusFailed, 10, 2, false)
	requireJobFileList(t, ctx, st, jobID, "", 2, 2, true)
	if sources, err := st.FailedImportJobFileSources(ctx, jobID); err != nil || len(sources) != 2 {
		t.Fatalf("FailedImportJobFileSources = %v, err=%v", sources, err)
	}
}

// requireJobFileList 断言文件级终态列表的条数与截断标记。
func requireJobFileList(t *testing.T, ctx context.Context, st *Store, jobID int64,
	status string, limit, wantLen int, wantTrunc bool) {
	t.Helper()
	got, err := st.ListImportJobFiles(ctx, jobID, status, limit)
	if err != nil || len(got.Files) != wantLen || got.Truncated != wantTrunc {
		t.Fatalf("ListImportJobFiles(%q, limit=%d) = %+v, err=%v，期望 %d 条 truncated=%v",
			status, limit, got, err, wantLen, wantTrunc)
	}
}

// TestGetImportJobFailedCount 详情的 failedCount 以文件级终态为准。
func TestGetImportJobFailedCount(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	jobID := seedJob(t, st)
	for _, src := range []string{"/books/a.txt", "/books/b.txt"} {
		if err := st.UpsertImportJobFile(ctx, jobID, ImportJobFile{
			Source: src, DisplayPath: src, Status: ImportFileStatusFailed,
		}); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}
	detail, err := st.GetImportJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetImportJob: %v", err)
	}
	if detail.FailedCount != 2 {
		t.Errorf("FailedCount = %d，期望 2", detail.FailedCount)
	}
}
