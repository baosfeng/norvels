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
)

func TestStartImportValidation(t *testing.T) {
	env := newEnv(t)
	dir := t.TempDir()
	notTxt := filepath.Join(dir, "not-a-txt.md")
	if err := os.WriteFile(notTxt, []byte("x"), 0o644); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}

	cases := []struct {
		name string
		body string
	}{
		{"空请求体", ""},
		{"非法 JSON", "{"},
		{"多个 JSON 对象", `{"dir":"/tmp"} {"dir":"/tmp"}`},
		{"dir 缺失", `{"workers":2}`},
		{"dir 不存在", `{"dir":"/no/such/dir/norvels"}`},
		{"既不是目录也不是 txt", fmt.Sprintf(`{"dir":%q}`, notTxt)},
		{"workers 越界", fmt.Sprintf(`{"dir":%q,"workers":99}`, dir)},
		{"workers 负数", fmt.Sprintf(`{"dir":%q,"workers":-1}`, dir)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := env.do(http.MethodPost, "/api/import", strings.NewReader(tc.body),
				map[string]string{"Content-Type": "application/json"})
			requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
		})
	}
}

func TestStartImportConflict(t *testing.T) {
	env := newEnv(t)
	dir := t.TempDir()

	// 库中已有 running 任务（重启后仍能识别）
	if _, err := env.st.CreateImportJob(context.Background(), dir); err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}

	rr := env.do(http.MethodPost, "/api/import", strings.NewReader(fmt.Sprintf(`{"dir":%q}`, dir)),
		map[string]string{"Content-Type": "application/json"})
	requireError(t, rr, http.StatusConflict, codeConflict)
}

func TestStartImportAccepted(t *testing.T) {
	env := newEnv(t)
	dir := t.TempDir()

	rr := env.do(http.MethodPost, "/api/import",
		strings.NewReader(fmt.Sprintf(`{"dir":%q,"dryRun":true,"recursive":false}`, dir)),
		map[string]string{"Content-Type": "application/json"})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("状态码 = %d，期望 202，body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeBody[struct {
		JobID int64 `json:"jobId"`
	}](t, rr)
	if resp.JobID <= 0 {
		t.Fatalf("jobId = %d，期望 > 0（精确返回建好的任务 id）", resp.JobID)
	}
	if detail := env.waitJobDone(resp.JobID, 5*time.Second); detail == nil {
		t.Fatalf("任务 %d 未在超时内离开 running", resp.JobID)
	}

	// 任务列表
	listRR := env.get("/api/import/jobs")
	if listRR.Code != http.StatusOK {
		t.Fatalf("任务列表状态码 = %d，body=%s", listRR.Code, listRR.Body.String())
	}
	list := decodeBody[importJobsResponse](t, listRR)
	if list.Items == nil {
		t.Fatal("任务列表 items 不应为 null")
	}

	// 非法 limit
	requireError(t, env.get("/api/import/jobs?limit=0"), http.StatusBadRequest, codeInvalidParam)
}

// TestStartImportSingleTxtFile 确认 POST /api/import 的 dir 可以直接是一个 txt 文件。
func TestStartImportSingleTxtFile(t *testing.T) {
	env := newEnv(t)
	dir := t.TempDir()
	txt := filepath.Join(dir, "单本.txt")
	if err := os.WriteFile(txt, []byte("书名：单本\n作者：测试\n第一章 起\n正文。\n"), 0o644); err != nil {
		t.Fatalf("写入临时 txt 失败: %v", err)
	}

	rr := env.do(http.MethodPost, "/api/import",
		strings.NewReader(fmt.Sprintf(`{"dir":%q,"dryRun":true}`, txt)),
		map[string]string{"Content-Type": "application/json"})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("状态码 = %d，期望 202 接受单文件，body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeBody[struct {
		JobID int64 `json:"jobId"`
	}](t, rr)
	if resp.JobID <= 0 {
		t.Fatalf("jobId = %d，期望 > 0", resp.JobID)
	}
}

func TestGetImportJobValidation(t *testing.T) {
	env := newEnv(t)
	requireError(t, env.get("/api/import/jobs/abc"), http.StatusBadRequest, codeInvalidID)
	requireError(t, env.get("/api/import/jobs/0"), http.StatusBadRequest, codeInvalidID)
	requireError(t, env.get("/api/import/jobs/999999"), http.StatusNotFound, codeNotFound)
}
