package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"norvels/internal/importer"
)

// 上传落盘与逐文件校验的常量。
const (
	uploadDirPerm      = 0o700
	uploadFilePerm     = 0o600
	uploadFieldFiles   = "files"
	uploadFieldForce   = "force"
	maxForceFieldBytes = 16 // force 只可能是 true/false
)

// 上传校验失败的哨兵值：落盘层返回，由 handler 翻译成 HTTP 状态与错误码。
var (
	errUploadFileTooLarge  = errors.New("upload: file too large")
	errUploadTotalTooLarge = errors.New("upload: total too large")
)

// readParts 逐个读取 multipart 部分（流式，只有单个文件在写盘缓冲区里）。
func (s *Server) readParts(sess *uploadSession, w http.ResponseWriter, mr *multipart.Reader, limits uploadLimits) bool {
	seen := 0
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return true
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidParam, "multipart 请求体损坏")
			return false
		}
		ok := false
		seen, ok = s.consumePart(sess, w, part, limits, seen)
		_ = part.Close()
		if !ok {
			return false
		}
	}
}

// consumePart 处理单个 multipart 部分，返回最新已见文件数与是否继续。
func (s *Server) consumePart(sess *uploadSession, w http.ResponseWriter, part *multipart.Part, limits uploadLimits, seen int) (int, bool) {
	switch part.FormName() {
	case uploadFieldForce:
		sess.force = readForceField(part)
		return seen, true
	case uploadFieldFiles:
		seen++
		if seen > limits.files {
			writeError(w, http.StatusBadRequest, codeTooManyFiles, fmt.Sprintf("一次最多上传 %d 个文件", limits.files))
			return seen, false
		}
		return seen, s.addFile(sess, w, part, limits)
	default:
		return seen, true // 未知字段忽略：NextPart 会自动丢弃未读内容
	}
}

// addFile 把单个 files 部分按扩展名过滤后受限写盘；返回是否继续处理后面的文件。
func (s *Server) addFile(sess *uploadSession, w http.ResponseWriter, part *multipart.Part, limits uploadLimits) bool {
	name := importer.CleanUploadName(part.FileName())
	if name == "" {
		return true // 没有可用文件名的部分（空文件域）直接忽略
	}
	format := importer.FormatOf(name)
	if format == "" {
		sess.rejected = append(sess.rejected, uploadRejected{
			Name: name, Code: codeUnsupportedFormat, Message: unsupportedFormatMessage(name),
		})
		return true
	}
	path, err := sess.writeFile(part, name, limits)
	if err != nil {
		s.writeUploadError(w, limits, name, err)
		return false
	}
	sess.files = append(sess.files, path)
	sess.accepted = append(sess.accepted, uploadAccepted{Name: filepath.Base(path), Format: format})
	return true
}

// writeUploadError 把落盘错误翻译成响应：超限 413，其余视为服务端故障。
func (s *Server) writeUploadError(w http.ResponseWriter, limits uploadLimits, name string, err error) {
	switch {
	case errors.Is(err, errUploadFileTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge,
			fmt.Sprintf("%s 超过单文件上限 %s", name, humanBytes(limits.fileBytes)))
	case errors.Is(err, errUploadTotalTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge,
			fmt.Sprintf("本批上传总量超过上限 %s", humanBytes(limits.totalBytes)))
	default:
		s.logf("api: 保存上传文件 %s 失败: %v", name, err)
		writeError(w, http.StatusInternalServerError, codeInternal, "保存上传文件失败")
	}
}

// writeFile 受限写盘：单文件与剩余总量取较小者作为上限，多写 1 字节用于探测超限。
func (sess *uploadSession) writeFile(part *multipart.Part, name string, limits uploadLimits) (string, error) {
	remaining := limits.totalBytes - sess.total
	if remaining <= 0 {
		return "", errUploadTotalTooLarge
	}
	limit, tooLarge := limits.fileBytes, errUploadFileTooLarge
	if remaining < limit {
		limit, tooLarge = remaining, errUploadTotalTooLarge
	}
	path, err := uniqueUploadPath(sess.dir, name)
	if err != nil {
		return "", err
	}
	written, err := writeLimited(path, part, limit)
	if err == nil && written <= limit {
		sess.total += written
		return path, nil
	}
	_ = os.Remove(path)
	if err != nil {
		return "", err
	}
	return "", tooLarge
}

// writeLimited 把 src 写入 path，最多 limit+1 字节；返回值 >limit 表示源超限。
func writeLimited(path string, src io.Reader, limit int64) (int64, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, uploadFilePerm)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return io.Copy(f, io.LimitReader(src, limit+1))
}

// uniqueUploadPath 为同名文件找一个不冲突的落盘路径（-1、-2 后缀）。
func uniqueUploadPath(dir, name string) (string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 0; i < 100; i++ {
		candidate := filepath.Join(dir, name)
		if i > 0 {
			candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		}
		if _, statErr := os.Stat(candidate); errors.Is(statErr, os.ErrNotExist) {
			return candidate, nil
		} else if statErr != nil {
			return "", statErr
		}
	}
	return "", fmt.Errorf("upload: 同名文件过多: %s", name)
}

// createUploadDir 在上传根下建本次任务的临时目录（根不存在则创建）。
func createUploadDir(root string) (string, error) {
	if err := os.MkdirAll(root, uploadDirPerm); err != nil {
		return "", err
	}
	return os.MkdirTemp(root, "upload-")
}

// readForceField 读取文本字段 force：只认 true/1，其余（含超长）按 false。
func readForceField(part *multipart.Part) bool {
	b, err := io.ReadAll(io.LimitReader(part, maxForceFieldBytes))
	if err != nil {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(string(b)))
	return v == "true" || v == "1"
}

// unsupportedFormatMessage 生成「暂不支持 .pdf，目前支持 TXT 与 EPUB」这类人话。
func unsupportedFormatMessage(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return "暂不支持无扩展名的文件，" + supportedFormatsHint()
	}
	return "暂不支持 " + ext + "，" + supportedFormatsHint()
}

// supportedFormatsHint 是「目前支持 TXT 与 EPUB」这类提示的唯一出处。
func supportedFormatsHint() string {
	return "目前支持 " + strings.Join(importer.SupportedFormats(), " 与 ")
}

// humanBytes 把字节数写成人话：整 MiB 用 MiB，其余用字节（测试会注入小上限）。
func humanBytes(n int64) string {
	if n >= 1<<20 && n%(1<<20) == 0 {
		return fmt.Sprintf("%d MiB", n>>20)
	}
	return fmt.Sprintf("%d 字节", n)
}
