package api

import (
	"context"
	"net/http"
	"testing"

	"norvels/internal/importer"
	"norvels/internal/store"
)

// cancelPath 拼取消接口路径。
func cancelPath(jobID int64) string { return "/api/import/jobs/" + itoa64(jobID) + "/cancel" }

// TestCancelImportJobNotFound 不存在的任务 → 404。
func TestCancelImportJobNotFound(t *testing.T) {
	e := newEnv(t)
	requireError(t, e.do(http.MethodPost, cancelPath(987654), nil, nil), http.StatusNotFound, codeNotFound)
}

// TestCancelImportJobRunning 运行中且已登记 → 202 "canceling"，且取消函数真的被调用。
func TestCancelImportJobRunning(t *testing.T) {
	e := newEnv(t)
	job, err := e.st.CreateImportJob(context.Background(), "/path/to/txt")
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	called := make(chan struct{})
	unregister := importer.Track(job.ID, func() { close(called) })
	defer unregister()

	rr := e.do(http.MethodPost, cancelPath(job.ID), nil, nil)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("取消状态码 = %d，期望 202，body=%s", rr.Code, rr.Body.String())
	}
	if body := decodeBody[cancelAccepted](t, rr); body.Status != "canceling" {
		t.Errorf("响应 status = %q，期望 canceling", body.Status)
	}
	select {
	case <-called:
	default:
		t.Error("取消函数没有被调用")
	}

	// 注销后再取消：任务记录还是 running，但本进程已没有它的取消入口 → 409。
	unregister()
	requireError(t, e.do(http.MethodPost, cancelPath(job.ID), nil, nil), http.StatusConflict, codeConflict)
}

// TestCancelImportJobFinished 已结束的任务 → 409；GET 任务详情能看到 canceled 状态。
func TestCancelImportJobFinished(t *testing.T) {
	e := newEnv(t)
	job, err := e.st.CreateImportJob(context.Background(), "/path/to/txt")
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	job.Status = "canceled"
	if err := e.st.UpdateImportJob(context.Background(), job); err != nil {
		t.Fatalf("UpdateImportJob: %v", err)
	}

	requireError(t, e.do(http.MethodPost, cancelPath(job.ID), nil, nil), http.StatusConflict, codeConflict)

	detail := decodeBody[store.ImportJobDetail](t, e.get("/api/import/jobs/"+itoa64(job.ID)))
	if detail.Status != "canceled" {
		t.Errorf("任务状态 = %q，期望 canceled", detail.Status)
	}
}
