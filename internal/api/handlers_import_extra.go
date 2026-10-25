package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"norvels/internal/importer"
	"norvels/internal/store"
)

// 导入侧 P1-B 的边界常量（P1-B §2.2）。
const (
	// dryRunTimeout 是预检单次请求的硬上限：超时回 503 timeout，不落库。
	dryRunTimeout = 60 * time.Second
	// maxImportJobFiles 与 docs/api.md 一致：文件级终态最多返回 1000 条。
	maxImportJobFiles = 1000
	// maxRetrySources 是一次重试最多接受的源条数（超过视为异常调用）。
	maxRetrySources = 1000
)

// handleDryRun 同步预检：不建任务、不占锁、不写库（P1-B §2.2）。
//
// 与真实导入共用同一套解析与查重口径；超过 60 秒回 503 timeout，
// 让前端把「目录太大」当作可重试的临时失败，而不是 500。
func (s *Server) handleDryRun(w http.ResponseWriter, r *http.Request, req importRequest, source string) {
	ctx, cancel := context.WithTimeout(r.Context(), dryRunTimeout)
	defer cancel()
	opts := importOptions(req, source, 0, s.loadChapterRules(ctx), s.loadTagRules(ctx))
	report, err := importer.DryRun(ctx, s.st, opts)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusServiceUnavailable, codeTimeout, "预检超过 60 秒，请缩小目录范围后重试")
	case err != nil:
		s.fail(w, r, err, "预检失败")
	default:
		writeJSON(w, http.StatusOK, report)
	}
}

// handleListImportJobFiles 处理 GET /api/import/jobs/{id}/files?status=failed。
func (s *Server) handleListImportJobFiles(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !store.ValidImportFileStatus(status) {
		writeError(w, http.StatusBadRequest, codeInvalidParam,
			"status 只支持 imported|failed|skipped|unsupported|canceled")
		return
	}
	res, err := s.st.ListImportJobFiles(r.Context(), id, status, maxImportJobFiles)
	if err != nil {
		s.fail(w, r, err, "import job not found")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// retryImportRequest 是 POST /api/import/jobs/{id}/retry 的请求体。
type retryImportRequest struct {
	Sources []string `json:"sources"`
}

// handleRetryImportJob 处理 POST /api/import/jobs/{id}/retry：为失败文件建新任务。
//
// 原任务记录不变；新任务的 import_job_files 只含本次重试的文件（由 Sources 驱动）。
func (s *Server) handleRetryImportJob(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	job, err := s.st.GetImportJob(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, codeNotFound, "import job not found")
		return
	}
	if err != nil {
		s.fail(w, r, err, "import job not found")
		return
	}
	if job.Status == "running" {
		writeError(w, http.StatusConflict, codeConflict, "任务仍在运行，结束后才能重试失败文件")
		return
	}
	sources, ok := s.retrySources(w, r, id)
	if !ok {
		return
	}
	newJob, ok := s.acquireImportJob(w, r, job.SourceDir)
	if !ok {
		return
	}
	opts := importOptions(importRequest{Dir: job.SourceDir}, job.SourceDir, newJob.ID,
		s.loadChapterRules(r.Context()), s.loadTagRules(r.Context()))
	opts.Sources = sources
	ctx, cancel := context.WithCancel(s.baseContext())
	s.spawnImport(newJob.ID, cancel, func() { s.runImport(ctx, cancel, newJob.ID, job.SourceDir, opts) })
	writeJSON(w, http.StatusAccepted, struct {
		JobID int64 `json:"jobId"`
	}{JobID: newJob.ID})
}

// retrySources 解析重试源：省略 body 表示「重试该任务全部 failed」。
func (s *Server) retrySources(w http.ResponseWriter, r *http.Request, jobID int64) ([]string, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, importBodyLimit))
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求体过大或不可读")
		return nil, false
	}
	if strings.TrimSpace(string(raw)) == "" {
		return s.failedSourcesOfJob(w, r, jobID)
	}
	return s.explicitRetrySources(w, r, jobID, raw)
}

// explicitRetrySources 校验显式给出的源：必须属于该任务且状态为 failed。
func (s *Server) explicitRetrySources(w http.ResponseWriter, r *http.Request, jobID int64, raw []byte) ([]string, bool) {
	var body retryImportRequest
	if err := json.Unmarshal(raw, &body); err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求体必须是包含 sources 数组的 JSON 对象")
		return nil, false
	}
	if len(body.Sources) == 0 {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "没有可重试的文件（sources 为空）")
		return nil, false
	}
	if len(body.Sources) > maxRetrySources {
		writeError(w, http.StatusBadRequest, codeInvalidParam,
			"一次最多重试 "+itoa(maxRetrySources)+" 个文件")
		return nil, false
	}
	statuses, err := s.st.ImportJobFileStatuses(r.Context(), jobID, body.Sources)
	if err != nil {
		s.fail(w, r, err, "import job not found")
		return nil, false
	}
	for _, src := range body.Sources {
		if strings.TrimSpace(src) == "" || statuses[src] != store.ImportFileStatusFailed {
			writeError(w, http.StatusBadRequest, codeInvalidParam,
				"源不在该任务的失败清单里: "+src)
			return nil, false
		}
	}
	return body.Sources, true
}

// failedSourcesOfJob 取该任务全部 failed 源；没有可重试项时 400。
func (s *Server) failedSourcesOfJob(w http.ResponseWriter, r *http.Request, jobID int64) ([]string, bool) {
	sources, err := s.st.FailedImportJobFileSources(r.Context(), jobID)
	if err != nil {
		s.fail(w, r, err, "import job not found")
		return nil, false
	}
	if len(sources) == 0 {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "该任务没有可重试的失败文件")
		return nil, false
	}
	return sources, true
}

// itoa 是给错误消息用的极小整数格式化（避免为一个字符串引入额外依赖）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
