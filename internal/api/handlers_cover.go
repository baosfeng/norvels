package api

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"norvels/internal/store"
)

// 封面写入（管理页 M1）：上传 / 删除。
//
// 封面是全库唯一入库的二进制资源（2 MiB 上限见 store.maxCoverBytes），
// 因此这里有独立的请求体上限：不能拿导入上传那套 64 MiB 的限额。

const (
	// maxCoverBodyBytes 是封面请求体的硬上限：2 MiB 图片 + multipart 边界开销。
	maxCoverBodyBytes = 3 << 20
	// coverFormField 是 multipart 里的文件字段名。
	coverFormField = "cover"
)

// handlePutBookCover 处理 PUT /api/books/{id}/cover：multipart 单文件，成功 204。
//
// 成功时不回图：前端 reload 详情拿到新的 updatedAt，封面地址的版本参数随之变化，
// 浏览器缓存自然失效（Cover 组件就是按 updatedAt 做版本号的）。
func (s *Server) handlePutBookCover(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	data, mime, ok := s.readCoverUpload(w, r)
	if !ok {
		return
	}
	if err := s.st.SaveCover(r.Context(), bookID, mime, data); err != nil {
		s.failCoverWrite(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readCoverUpload 读出封面上传，失败时已写好响应。
//
// 先卡请求体总大小再解析，避免 3 MiB 以上的垃圾数据把内存吃满；
// MIME 优先用客户端声明，缺省时按内容嗅探（curl 上传常不带 Content-Type）。
func (s *Server) readCoverUpload(w http.ResponseWriter, r *http.Request) (data []byte, mime string, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCoverBodyBytes)
	if err := r.ParseMultipartForm(maxCoverBodyBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge,
			"封面最大 2 MiB，且必须是 multipart/form-data（字段名 cover）")
		return nil, "", false
	}
	file, header, err := r.FormFile(coverFormField)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求里没有 cover 字段")
		return nil, "", false
	}
	defer file.Close() //nolint:errcheck // 只读句柄

	data, err = io.ReadAll(io.LimitReader(file, maxCoverBodyBytes))
	if err != nil {
		s.logf("api: 读取封面上传失败: %v", err)
		writeError(w, http.StatusBadRequest, codeInvalidParam, "封面读取失败，请重试")
		return nil, "", false
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "封面文件是空的")
		return nil, "", false
	}
	mime = strings.TrimSpace(header.Header.Get("Content-Type"))
	if mime == "" || mime == "application/octet-stream" {
		mime = http.DetectContentType(data)
	}
	return data, mime, true
}

// handleDeleteBookCover 删掉封面（幂等：本来没有也返回 204）。
func (s *Server) handleDeleteBookCover(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	if err := s.st.DeleteCover(r.Context(), bookID); err != nil {
		s.failCoverWrite(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// failCoverWrite 把封面写入错误翻成响应：格式/体积不合法是 400/413，书不存在是 404。
func (s *Server) failCoverWrite(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrInvalidArgument) {
		writeError(w, http.StatusBadRequest, codeInvalidParam, coverParamMessage(err))
		return
	}
	s.fail(w, r, err, "这本书不存在")
}

// coverParamMessage 把 store 的封面入参错误翻成用户能懂的话。
func coverParamMessage(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "mime"):
		return "封面只支持 JPEG / PNG / WebP / GIF"
	case strings.Contains(msg, "exceeds"):
		return "封面最大 2 MiB"
	case strings.Contains(msg, "empty"):
		return "封面文件是空的"
	}
	return paramMessage(err)
}
