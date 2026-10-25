package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"norvels/internal/store"
)

// seedJobWithFiles 造一个已完成任务并写入三条文件级终态。
func (e *testEnv) seedJobWithFiles() int64 {
	e.t.Helper()
	ctx := context.Background()
	job, err := e.st.CreateImportJob(ctx, "/books")
	if err != nil {
		e.t.Fatalf("CreateImportJob: %v", err)
	}
	now := time.Now().UTC()
	job.Status, job.FinishedAt, job.Total = "completed_with_errors", &now, 3
	job.Failed = 2
	if err := e.st.UpdateImportJob(ctx, job); err != nil {
		e.t.Fatalf("UpdateImportJob: %v", err)
	}
	files := []store.ImportJobFile{
		{Source: "/books/bad1.txt", DisplayPath: "bad1.txt", Status: store.ImportFileStatusFailed, Reason: "解析失败"},
		{Source: "/books/bad2.txt", DisplayPath: "bad2.txt", Status: store.ImportFileStatusFailed, Reason: "解析失败"},
		{Source: "/books/good.txt", DisplayPath: "good.txt", Status: store.ImportFileStatusImported, BookID: 5},
	}
	for _, f := range files {
		if err := e.st.UpsertImportJobFile(ctx, job.ID, f); err != nil {
			e.t.Fatalf("UpsertImportJobFile: %v", err)
		}
	}
	return job.ID
}

// TestDryRunReport 预检返回报告且不新增 import_jobs 行。
func TestDryRunReport(t *testing.T) {
	env := newEnv(t)
	dir := t.TempDir()
	writeTXT(t, filepath.Join(dir, "可导入.txt"), "书名：预检书\n作者：预检作者\n第一章 起\n合成正文。\n")
	if err := os.WriteFile(filepath.Join(dir, "不支持.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写 pdf: %v", err)
	}
	before := env.jobCount(t)

	rr := env.do(http.MethodPost, "/api/import",
		strings.NewReader(fmt.Sprintf(`{"dir":%q,"dryRun":true}`, dir)), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("dry-run 状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	rep := decodeBody[DryRunReportProbe](t, rr)
	if rep.Files.Total < 2 || rep.Files.WillImport != 1 || rep.Files.Unsupported != 1 {
		t.Fatalf("报告计数 = %+v，期望 willImport=1 unsupported=1", rep.Files)
	}
	if rep.Books.New != 1 || len(rep.Sample) != 1 {
		t.Fatalf("books/sample = %+v / %+v", rep.Books, rep.Sample)
	}
	if after := env.jobCount(t); after != before {
		t.Errorf("dry-run 新增了 import_jobs 行：before=%d after=%d", before, after)
	}
}

// DryRunReportProbe 是预检响应的测试侧结构。
type DryRunReportProbe struct {
	Truncated bool `json:"truncated"`
	Files     struct {
		Total       int `json:"total"`
		WillImport  int `json:"willImport"`
		Duplicate   int `json:"duplicate"`
		Unsupported int `json:"unsupported"`
		Failed      int `json:"failed"`
	} `json:"files"`
	Books struct {
		New       int `json:"new"`
		Duplicate int `json:"duplicate"`
		Split     int `json:"split"`
	} `json:"books"`
	Sample []struct {
		Source string `json:"source"`
		Title  string `json:"title"`
	} `json:"sample"`
	Errors []struct {
		Reason string `json:"reason"`
	} `json:"errors"`
}

// jobCount 返回库里的任务行数。
func (e *testEnv) jobCount(t *testing.T) int {
	e.t.Helper()
	jobs, err := e.st.ListImportJobs(context.Background(), 200)
	if err != nil {
		e.t.Fatalf("ListImportJobs: %v", err)
	}
	return len(jobs)
}

// writeTXT 写一份合成 txt。
func writeTXT(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写 txt: %v", err)
	}
}

// TestListImportJobFilesEndpoint 过滤、404 与非法 status。
func TestListImportJobFilesEndpoint(t *testing.T) {
	env := newEnv(t)
	jobID := env.seedJobWithFiles()

	rr := env.get(fmt.Sprintf("/api/import/jobs/%d/files?status=failed", jobID))
	if rr.Code != http.StatusOK {
		t.Fatalf("files 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	body := decodeBody[store.ImportJobFiles](t, rr)
	if len(body.Files) != 2 || body.Files[0].Status != store.ImportFileStatusFailed {
		t.Fatalf("failed 列表 = %+v", body.Files)
	}
	requireError(t, env.get(fmt.Sprintf("/api/import/jobs/%d/files?status=bogus", jobID)),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.get("/api/import/jobs/9999/files"), http.StatusNotFound, codeNotFound)
}

// TestRetryImportJob 省略 sources 时重试全部 failed，新任务只含这些文件。
func TestRetryImportJob(t *testing.T) {
	env := newEnv(t)
	jobID := env.seedJobWithFiles()

	rr := env.do(http.MethodPost, fmt.Sprintf("/api/import/jobs/%d/retry", jobID), nil, nil)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("retry 状态码 = %d，期望 202，body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeBody[struct {
		JobID int64 `json:"jobId"`
	}](t, rr)
	if resp.JobID <= 0 || resp.JobID == jobID {
		t.Fatalf("新任务 id = %d，期望 > 0 且不等于原任务 %d", resp.JobID, jobID)
	}
	detail := env.waitJobDone(resp.JobID, 5*time.Second)
	if detail == nil {
		t.Fatalf("重试任务 %d 未结束", resp.JobID)
	}
	files, err := env.st.ListImportJobFiles(context.Background(), resp.JobID, "", 10)
	if err != nil || len(files.Files) != 2 {
		t.Fatalf("新任务文件数 = %+v, err=%v，期望 2", files.Files, err)
	}
	original, err := env.st.GetImportJob(context.Background(), jobID)
	if err != nil || original.Status != "completed_with_errors" || original.FailedCount != 2 {
		t.Errorf("原任务被改动: %+v, err=%v", original, err)
	}
}

// TestRetryImportJobRejects 404 / running 409 / 无失败项 400 / 源不属于任务 400。
func TestRetryImportJobRejects(t *testing.T) {
	env := newEnv(t)
	requireError(t, env.do(http.MethodPost, "/api/import/jobs/9999/retry", nil, nil),
		http.StatusNotFound, codeNotFound)

	running, err := env.st.CreateImportJob(context.Background(), "/books")
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	requireError(t, env.do(http.MethodPost, fmt.Sprintf("/api/import/jobs/%d/retry", running.ID), nil, nil),
		http.StatusConflict, codeConflict)

	done := env.seedJobWithFiles()
	// 显式源只有一条 imported：不可重试。
	payload := `{"sources":["/books/good.txt"]}`
	requireError(t, env.do(http.MethodPost, fmt.Sprintf("/api/import/jobs/%d/retry", done),
		strings.NewReader(payload), nil), http.StatusBadRequest, codeInvalidParam)
	// 源不在任务里。
	payload = `{"sources":["/books/not-in-job.txt"]}`
	requireError(t, env.do(http.MethodPost, fmt.Sprintf("/api/import/jobs/%d/retry", done),
		strings.NewReader(payload), nil), http.StatusBadRequest, codeInvalidParam)
}
