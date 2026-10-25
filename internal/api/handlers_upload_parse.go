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
	uploadDirPerm     = 0o700
	uploadFilePerm    = 0o600
	uploadFieldFiles  = "files"
	uploadFieldForce  = "force"
	uploadFieldUpdate = "update"
	maxBoolFieldBytes = 16 // force/update 只可能是 true/false
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
		sess.force = readBoolField(part)
		return seen, true
	case uploadFieldUpdate:
		sess.update = readBoolField(part)
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
	if format == importer.FormatZIP {
		return s.addZipEntries(sess, path)
	}
	sess.files = append(sess.files, path)
	sess.accepted = append(sess.accepted, uploadAccepted{Name: filepath.Base(path), Format: format})
	return true
}

// addZipEntries 展开 zip 的中央目录，把包内每个条目单独报成 accepted / rejected。
//
// 条目名对外是「包名.zip:包内路径」；一个 zip 只要有一个条目被接受，整包就进入导入队列
// （真正的解包在 importer 里做，避免上传期把字节摊到磁盘上两次）。
func (s *Server) addZipEntries(sess *uploadSession, path string) bool {
	base := filepath.Base(path)
	entries, err := importer.ListZIP(path)
	if err != nil {
		_ = os.Remove(path)
		sess.rejected = append(sess.rejected, uploadRejected{
			Name: base, Code: zipErrorCode(err), Message: zipErrorMessage(err),
		})
		return true
	}
	accepted := 0
	for _, e := range entries {
		name := base + ":" + e.Name
		if e.Format == "" {
			sess.rejected = append(sess.rejected, uploadRejected{
				Name: name, Code: codeUnsupportedFormat, Message: e.Reason,
			})
			continue
		}
		sess.accepted = append(sess.accepted, uploadAccepted{Name: name, Format: e.Format})
		accepted++
	}
	if accepted > 0 {
		sess.files = append(sess.files, path)
	}
	return true
}

// zipErrorCode 把解包错误映射成前端可分支的错误码。
func zipErrorCode(err error) string {
	switch {
	case errors.Is(err, importer.ErrZIPTooLarge):
		return codeZipTooLarge
	case errors.Is(err, importer.ErrZIPEmpty):
		return codeEmptyZip
	default:
		return codeInvalidZip
	}
}

// zipErrorMessage 生成 zip 级失败的人话原因。
func zipErrorMessage(err error) string {
	switch {
	case errors.Is(err, importer.ErrZIPTooLarge):
		return "zip 解压超过上限（1000 个条目 / 512 MiB 合计 / 64 MiB 单条目）"
	case errors.Is(err, importer.ErrZIPEmpty):
		return "zip 里没有可导入的文件（只收 .txt 与 .epub）"
	case errors.Is(err, importer.ErrZIPInvalid):
		return "不是合法的 zip 包，或包内含有越界路径 / 符号链接"
	}
	return "zip 解包失败"
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

// readBoolField 读取布尔文本字段（force/update）：只认 true/1，其余（含超长）按 false。
func readBoolField(part *multipart.Part) bool {
	b, err := io.ReadAll(io.LimitReader(part, maxBoolFieldBytes))
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

// supportedFormatsHint 是「目前支持 TXT、EPUB 与 ZIP」这类提示的唯一出处。
func supportedFormatsHint() string {
	return "目前支持 " + joinAnd(importer.SupportedFormats())
}

// joinAnd 把列表拼成「A、B 与 C」。
func joinAnd(items []string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], "、") + " 与 " + items[len(items)-1]
}

// humanBytes 把字节数写成人话：整 MiB 用 MiB，其余用字节（测试会注入小上限）。
func humanBytes(n int64) string {
	if n >= 1<<20 && n%(1<<20) == 0 {
		return fmt.Sprintf("%d MiB", n>>20)
	}
	return fmt.Sprintf("%d 字节", n)
}
