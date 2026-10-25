package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"norvels/internal/store"
)

// TestRetryExplicitSources 显式 sources 重试成功：只重试指定文件。
func TestRetryExplicitSources(t *testing.T) {
	env := newEnv(t)
	jobID := env.seedJobWithFiles()
	payload := `{"sources":["/books/bad1.txt"]}`
	rr := env.do(http.MethodPost, fmt.Sprintf("/api/import/jobs/%d/retry", jobID), strings.NewReader(payload), nil)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("显式源重试状态码 = %d，期望 202，body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeBody[struct {
		JobID int64 `json:"jobId"`
	}](t, rr)
	if resp.JobID <= 0 {
		t.Fatalf("新任务 id = %d", resp.JobID)
	}
	env.waitJobDone(resp.JobID, 5*time.Second)
	files, err := env.st.ListImportJobFiles(context.Background(), resp.JobID, "", 10)
	if err != nil || len(files.Files) != 1 || files.Files[0].Source != "/books/bad1.txt" {
		t.Fatalf("新任务文件 = %+v, err=%v，期望只含 bad1.txt", files.Files, err)
	}
}

// TestRetryNoFailed 任务没有 failed 文件时重试返回 400。
func TestRetryNoFailed(t *testing.T) {
	env := newEnv(t)
	job, err := env.st.CreateImportJob(context.Background(), "/books")
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	now := time.Now().UTC()
	job.Status, job.FinishedAt = "completed", &now
	if err := env.st.UpdateImportJob(context.Background(), job); err != nil {
		t.Fatalf("UpdateImportJob: %v", err)
	}
	requireError(t, env.do(http.MethodPost, fmt.Sprintf("/api/import/jobs/%d/retry", job.ID), nil, nil),
		http.StatusBadRequest, codeInvalidParam)
}

// TestRetryEmptyAndTooManySources 空数组与超量 sources 都按参数错误拒绝。
func TestRetryEmptyAndTooManySources(t *testing.T) {
	env := newEnv(t)
	jobID := env.seedJobWithFiles()
	requireError(t, env.do(http.MethodPost, fmt.Sprintf("/api/import/jobs/%d/retry", jobID),
		strings.NewReader(`{"sources":[]}`), nil), http.StatusBadRequest, codeInvalidParam)

	many := make([]string, 0, maxRetrySources+1)
	for i := 0; i <= maxRetrySources; i++ {
		many = append(many, fmt.Sprintf("/books/f%d.txt", i))
	}
	payload, err := jsonMarshalSources(many)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	requireError(t, env.do(http.MethodPost, fmt.Sprintf("/api/import/jobs/%d/retry", jobID),
		strings.NewReader(payload), nil), http.StatusBadRequest, codeInvalidParam)
}

// jsonMarshalSources 把源列表包成请求体。
func jsonMarshalSources(sources []string) (string, error) {
	body, err := json.Marshal(retryImportRequest{Sources: sources})
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// TestImportJobFilesAllAndInvalidStatus 覆盖「不带 status 返回全部」与非法状态。
func TestImportJobFilesAllAndInvalidStatus(t *testing.T) {
	env := newEnv(t)
	jobID := env.seedJobWithFiles()
	rr := env.get(fmt.Sprintf("/api/import/jobs/%d/files", jobID))
	if rr.Code != http.StatusOK {
		t.Fatalf("files 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	body := decodeBody[store.ImportJobFiles](t, rr)
	if len(body.Files) != 3 {
		t.Fatalf("全部文件数 = %d，期望 3", len(body.Files))
	}
	requireError(t, env.get(fmt.Sprintf("/api/import/jobs/%d/files?status=bogus", jobID)),
		http.StatusBadRequest, codeInvalidParam)
}
