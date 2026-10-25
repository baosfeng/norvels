package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"norvels/internal/importer"
	"norvels/internal/parser"
	"norvels/internal/store"
)

const (
	importBodyLimit  = 1 << 20 // 1 MiB
	maxImportWorkers = 32
	// maxImportErrors 与 docs/api.md 一致：详情最多返回 200 条失败明细。
	maxImportErrors = 200
	// staleImportJobAfter 之后仍为 running 的任务视为进程崩溃留下的僵尸任务。
	staleImportJobAfter = 12 * time.Hour
)

// importRequest 是 POST /api/import 的请求体。
type importRequest struct {
	Dir       string `json:"dir"`
	Recursive *bool  `json:"recursive"`
	Force     bool   `json:"force"`
	Workers   int    `json:"workers"`
	DryRun    bool   `json:"dryRun"`
}

func (s *Server) handleStartImport(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeImportRequest(w, r)
	if !ok {
		return
	}
	source, ok := validateImportSource(w, req)
	if !ok {
		return
	}
	job, ok := s.acquireImportJob(w, r, source)
	if !ok {
		return
	}
	opts := importOptions(req, source, job.ID)

	// 导入任务随服务退出而取消（Config.Context 由 serve 的 signal ctx 注入）。
	ctx, cancel := context.WithCancel(s.baseContext())
	go s.runImport(ctx, cancel, job.ID, source, opts)

	writeJSON(w, http.StatusAccepted, struct {
		JobID int64 `json:"jobId"`
	}{JobID: job.ID})
}

// decodeImportRequest 解析并校验请求体：只允许一个 JSON 对象，且不超过 importBodyLimit。
// 出错时已写好 400 响应，返回 ok=false。
func decodeImportRequest(w http.ResponseWriter, r *http.Request) (importRequest, bool) {
	var req importRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, importBodyLimit))
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, codeInvalidParam, "请求体不能为空")
			return req, false
		}
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求体必须是合法 JSON 对象")
		return req, false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求体只能包含一个 JSON 对象")
		return req, false
	}
	return req, true
}

// validateImportSource 校验源路径与 workers，并把源路径归一化为绝对路径。
// 源路径可以是目录，也可以是单个 .txt 文件。
// 出错时已写好 400 响应，返回 ok=false。
func validateImportSource(w http.ResponseWriter, req importRequest) (string, bool) {
	source := strings.TrimSpace(req.Dir)
	if source == "" {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "dir 必填（目录，或 .txt / .epub 文件路径）")
		return "", false
	}
	info, err := os.Stat(source)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "路径不存在或不可读: "+source)
		return "", false
	}
	if !info.IsDir() && !importer.SupportedExt(info.Name()) {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "路径既不是目录也不是支持的图书文件（.txt / .epub）: "+source)
		return "", false
	}
	if req.Workers < 0 || req.Workers > maxImportWorkers {
		writeError(w, http.StatusBadRequest, codeInvalidParam,
			fmt.Sprintf("workers 必须在 0~%d 之间", maxImportWorkers))
		return "", false
	}
	if abs, absErr := filepath.Abs(source); absErr == nil {
		source = abs
	}
	return source, true
}

// acquireImportJob 在临界区内查运行中任务并建新任务，返回新建的任务。
// 出错时已写好响应（409/500），返回 ok=false。
//
// 409 以库中任务状态为准（进程重启后仍然一致）；
// 查库与建任务放在同一临界区，避免并发 POST 各建一个任务。
func (s *Server) acquireImportJob(w http.ResponseWriter, r *http.Request, dir string) (*store.ImportJob, bool) {
	s.importMu.Lock()
	running, runErr := s.runningImportJob(r.Context())
	var job *store.ImportJob
	var createErr error
	if runErr == nil && running == nil {
		job, createErr = s.st.CreateImportJob(r.Context(), dir)
	}
	s.importMu.Unlock()

	switch {
	case runErr != nil:
		s.fail(w, r, runErr, "")
		return nil, false
	case running != nil:
		writeError(w, http.StatusConflict, codeConflict,
			fmt.Sprintf("已有导入任务正在运行（job %d，来源 %s）", running.ID, running.SourceDir))
		return nil, false
	case createErr != nil:
		s.fail(w, r, createErr, "")
		return nil, false
	}
	return job, true
}

// importOptions 把请求体翻译成 importer.Options（recursive 缺省为 true）。
// dir 既可以是目录也可以是单个 .txt 文件；recursive 对文件无意义，会被扫描器忽略。
func importOptions(req importRequest, dir string, jobID int64) importer.Options {
	recursive := true
	if req.Recursive != nil {
		recursive = *req.Recursive
	}
	return importer.Options{
		Dir:        dir,
		Recursive:  recursive,
		Force:      req.Force,
		DryRun:     req.DryRun,
		Workers:    req.Workers,
		ParserOpts: parser.DefaultOptions(),
		JobID:      jobID,
	}
}

// runningImportJob 返回库中正在运行的任务；僵尸任务（超过 staleImportJobAfter）会被标记失败并跳过。
func (s *Server) runningImportJob(ctx context.Context) (*store.ImportJob, error) {
	jobs, err := s.st.ListImportJobs(ctx, 20)
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		job := jobs[i]
		if job.Status != "running" {
			continue
		}
		if !job.StartedAt.IsZero() && time.Since(job.StartedAt) > staleImportJobAfter {
			s.logf("api: 清理僵尸导入任务 job=%d 目录=%s（开始于 %s）",
				job.ID, job.SourceDir, job.StartedAt.Format(time.RFC3339))
			_ = s.finalizeImportJob(job.ID, "failed", "任务超时未收尾，已标记失败", nil)
			continue
		}
		return &job, nil
	}
	return nil, nil
}

// runImport 在 goroutine 中执行导入，并保证任务记录最终不会停留在 running。
func (s *Server) runImport(ctx context.Context, cancel context.CancelFunc, jobID int64, dir string, opts importer.Options) {
	defer cancel()
	defer func() {
		if rec := recover(); rec != nil {
			s.logf("api: 导入 goroutine panic: %v\n%s", rec, debug.Stack())
			_ = s.finalizeImportJob(jobID, "failed", "导入异常退出", nil)
		}
	}()

	start := time.Now()
	res, err := importer.Run(ctx, s.st, opts, nil)
	switch {
	case errors.Is(err, importer.ErrNoFiles):
		s.logf("api: 导入目录中没有 txt dir=%s", dir)
		_ = s.finalizeImportJob(jobID, "failed", "目录中没有 .txt 文件", res)
	case errors.Is(err, context.Canceled):
		s.logf("api: 导入被取消 dir=%s", dir)
		_ = s.finalizeImportJob(jobID, "canceled", "服务退出，导入已取消", res)
	case err != nil:
		s.logf("api: 导入失败 dir=%s: %v", dir, err)
		_ = s.finalizeImportJob(jobID, "failed", err.Error(), res)
	default:
		// importer 已写入终态；DryRun 等未落终态的情况在这里补写。
		if ferr := s.finalizeImportJob(jobID, "", "", res); ferr != nil {
			s.logf("api: 收尾导入任务失败 job=%d: %v", jobID, ferr)
		}
		imported, skipped, failed := 0, 0, 0
		if res != nil {
			imported, skipped, failed = len(res.Imported), res.Skipped, res.Failed
		}
		s.logf("api: 导入完成 dir=%s 导入=%d 跳过=%d 失败=%d 用时=%s",
			dir, imported, skipped, failed, time.Since(start).Round(time.Second))
	}
}

// finalizeImportJob 在任务仍为 running 时写入终态。
// status 为空表示「importer 自己没写终态（例如 DryRun）」，此时补写 completed。
func (s *Server) finalizeImportJob(jobID int64, status, message string, res *importer.Result) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.baseContext()), 5*time.Second)
	defer cancel()
	detail, err := s.st.GetImportJob(ctx, jobID)
	if err != nil {
		return err
	}
	if detail == nil || detail.Status != "running" {
		return nil
	}
	job := detail.ImportJob
	now := time.Now().UTC()
	job.FinishedAt = &now
	if status == "" {
		job.Status = "completed"
	} else {
		job.Status = status
	}
	if message != "" {
		job.Message = message
	}
	if res != nil {
		job.Imported = len(res.Imported)
		job.Skipped = res.Skipped
		job.Failed = res.Failed
		if job.Total == 0 {
			job.Total = job.Imported + job.Skipped + job.Failed
		}
	}
	return s.st.UpdateImportJob(ctx, &job)
}

// importJobsResponse 对应 GET /api/import/jobs。
type importJobsResponse struct {
	Items []store.ImportJob `json:"items"`
}

func (s *Server) handleListImportJobs(w http.ResponseWriter, r *http.Request) {
	limit, err := intQuery(r, "limit", defaultJobLimit, 1, maxJobLimit)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	jobs, err := s.st.ListImportJobs(r.Context(), limit)
	if err != nil {
		s.fail(w, r, err, "导入任务不可用")
		return
	}
	if jobs == nil {
		jobs = []store.ImportJob{}
	}
	writeJSON(w, http.StatusOK, importJobsResponse{Items: jobs})
}

func (s *Server) handleGetImportJob(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	job, err := s.st.GetImportJob(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, "import job not found")
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, codeNotFound, "import job not found")
		return
	}
	if job.Errors == nil {
		job.Errors = []store.ImportError{}
	}
	if len(job.Errors) > maxImportErrors {
		job.Errors = job.Errors[:maxImportErrors]
	}
	writeJSON(w, http.StatusOK, job)
}
