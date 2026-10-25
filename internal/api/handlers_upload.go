package api

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"norvels/internal/importer"
	"norvels/internal/parser"
)

// 上传接口（docs/specs/2026-10-25-import-formats.md §2）的默认上限。
const (
	defaultMaxUploadFileBytes  = 64 << 20  // 单文件 64 MiB
	defaultMaxUploadTotalBytes = 256 << 20 // 单批 256 MiB
	defaultMaxUploadFiles      = 20
)

// uploadResponse 对应 POST /api/import/upload 的 202 响应（逐文件给出 accepted/rejected）。
type uploadResponse struct {
	JobID    int64            `json:"jobId"`
	Accepted []uploadAccepted `json:"accepted"`
	Rejected []uploadRejected `json:"rejected"`
}

// uploadAccepted 是一个被接受的文件，format 为 txt/epub。
type uploadAccepted struct {
	Name   string `json:"name"`
	Format string `json:"format"`
}

// uploadRejected 是一个被拒绝的文件：code 供前端分支，message 面向人类。
type uploadRejected struct {
	Name    string `json:"name"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// uploadLimits 是本次请求生效的上限。
type uploadLimits struct {
	fileBytes  int64
	totalBytes int64
	files      int
}

// uploadLimits 从配置读取上限，零值回落默认值（测试注入小值触发 413/400）。
func (c Config) uploadLimits() uploadLimits {
	l := uploadLimits{fileBytes: defaultMaxUploadFileBytes, totalBytes: defaultMaxUploadTotalBytes, files: defaultMaxUploadFiles}
	if c.MaxUploadFileBytes > 0 {
		l.fileBytes = c.MaxUploadFileBytes
	}
	if c.MaxUploadTotalBytes > 0 {
		l.totalBytes = c.MaxUploadTotalBytes
	}
	if c.MaxUploadFiles > 0 {
		l.files = c.MaxUploadFiles
	}
	return l
}

// uploadRoot 是上传临时目录的根：未配置时用 os.TempDir()。
func (c Config) uploadRoot() string {
	if dir := strings.TrimSpace(c.UploadDir); dir != "" {
		return dir
	}
	return os.TempDir()
}

// uploadSession 是一次上传的中间状态；导入结束整个临时目录删除。
type uploadSession struct {
	dir      string
	force    bool
	files    []string
	accepted []uploadAccepted
	rejected []uploadRejected
	total    int64
}

// handleUploadImport 处理 POST /api/import/upload：流式落盘 → 建任务 → 后台导入。
//
// 先判 multipart 与「已有任务」再收文件，避免用户白传 256 MiB 才被打回 409；
// 任何一步失败都整目录清理，不留半截临时文件。
func (s *Server) handleUploadImport(w http.ResponseWriter, r *http.Request) {
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求必须是 multipart/form-data")
		return
	}
	if !s.uploadIdle(w, r) {
		return
	}
	sess, ok := s.receiveUpload(w, r, mr)
	if !ok {
		return
	}
	job, ok := s.acquireImportJob(w, r, "upload://"+filepath.Base(sess.dir))
	if !ok {
		s.discardUpload(sess.dir)
		return
	}
	opts := importer.Options{Dir: sess.dir, Force: sess.force, ParserOpts: parser.DefaultOptions(), JobID: job.ID}
	ctx, cancel := context.WithCancel(s.baseContext())
	go s.runUploadImport(ctx, cancel, job.ID, sess, opts)
	writeJSON(w, http.StatusAccepted, uploadResponse{JobID: job.ID, Accepted: sess.accepted, Rejected: sess.rejected})
}

// uploadIdle 在收文件之前先判一次「已有导入任务在跑」，有则写 409 并返回 false。
// 文案与 acquireImportJob 的 409 一致（真正建任务时还会在临界区内再判一次）。
func (s *Server) uploadIdle(w http.ResponseWriter, r *http.Request) bool {
	s.importMu.Lock()
	running, err := s.runningImportJob(r.Context())
	s.importMu.Unlock()
	switch {
	case err != nil:
		s.fail(w, r, err, "")
		return false
	case running != nil:
		writeError(w, http.StatusConflict, codeConflict,
			fmt.Sprintf("已有导入任务正在运行（job %d，来源 %s）", running.ID, running.SourceDir))
		return false
	}
	return true
}

// receiveUpload 建临时目录、流式读完 multipart，并做整批校验；失败时已写好响应。
func (s *Server) receiveUpload(w http.ResponseWriter, r *http.Request, mr *multipart.Reader) (*uploadSession, bool) {
	dir, err := createUploadDir(s.cfg.uploadRoot())
	if err != nil {
		s.logf("api: 创建上传临时目录失败: %v", err)
		writeError(w, http.StatusInternalServerError, codeInternal, "上传临时目录不可用")
		return nil, false
	}
	sess := &uploadSession{dir: dir, accepted: []uploadAccepted{}, rejected: []uploadRejected{}}
	if !s.readParts(sess, w, mr, s.cfg.uploadLimits()) || !sess.finish(w) {
		s.discardUpload(dir)
		return nil, false
	}
	return sess, true
}

// finish 校验整批结果：空批次、全部不支持、加密 EPUB 都在建任务前挡回去。
func (sess *uploadSession) finish(w http.ResponseWriter) bool {
	switch {
	case len(sess.files) == 0 && len(sess.rejected) == 0:
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请选择要上传的文件（字段名 files）")
		return false
	case len(sess.files) == 0:
		writeError(w, http.StatusBadRequest, codeUnsupportedFormat, "上传的文件都不受支持，目前支持 TXT 与 EPUB")
		return false
	}
	if name := sess.firstEncryptedEPUB(); name != "" {
		writeError(w, http.StatusUnprocessableEntity, codeEncryptedEPUB, name+" 已加密（DRM），暂不支持导入")
		return false
	}
	return true
}

// firstEncryptedEPUB 返回第一个声明加密的 EPUB 文件名（没有则空串）。
func (sess *uploadSession) firstEncryptedEPUB() string {
	for _, path := range sess.files {
		if importer.FormatOf(path) == importer.FormatEPUB && importer.EncryptedEPUB(path) {
			return filepath.Base(path)
		}
	}
	return ""
}

// runUploadImport 在 goroutine 中执行上传导入，保证任务不留在 running、临时目录最终删除。
func (s *Server) runUploadImport(ctx context.Context, cancel context.CancelFunc, jobID int64,
	sess *uploadSession, opts importer.Options) {
	defer cancel()
	defer s.discardUpload(sess.dir)
	defer func() {
		if rec := recover(); rec != nil {
			s.logf("api: 上传导入 goroutine panic: %v stacks=%s", rec, debug.Stack())
			_ = s.finalizeImportJob(jobID, "failed", "导入异常退出", nil)
		}
	}()
	start := time.Now()
	res, err := importer.RunUpload(ctx, s.st, opts)
	if err != nil {
		s.logf("api: 上传导入失败 dir=%s: %v", sess.dir, err)
		status, message := uploadFailure(err)
		_ = s.finalizeImportJob(jobID, status, message, res)
		return
	}
	if ferr := s.finalizeImportJob(jobID, "", "", res); ferr != nil {
		s.logf("api: 收尾上传任务失败 job=%d: %v", jobID, ferr)
	}
	s.logf("api: 上传导入完成 dir=%s 用时=%s", sess.dir, time.Since(start).Round(time.Second))
}

// uploadFailure 把导入错误翻译成任务终态与提示。
func uploadFailure(err error) (string, string) {
	switch {
	case errors.Is(err, importer.ErrNoFiles):
		return "failed", "上传中没有可导入的文件"
	case errors.Is(err, context.Canceled):
		return "canceled", "服务退出，导入已取消"
	}
	return "failed", err.Error()
}

// discardUpload 删除上传临时目录；删除失败只记日志（比让半截文件留在盘上强）。
func (s *Server) discardUpload(dir string) {
	if dir == "" {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		s.logf("api: 删除上传临时目录失败 dir=%s: %v", dir, err)
	}
}
