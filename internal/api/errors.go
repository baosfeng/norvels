package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"norvels/internal/store"
)

// errorDetail 是错误响应中的明细。
type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// errorBody 是统一错误响应 {"error":{"code":...,"message":...}}。
type errorBody struct {
	Error errorDetail `json:"error"`
}

// 错误码：前端可据此分支，消息面向人类。
const (
	codeInvalidParam     = "invalid_param"
	codeInvalidID        = "invalid_id"
	codeNotFound         = "not_found"
	codeConflict         = "conflict"
	codeInternal         = "internal"
	codeMethodNotAllowed = "method_not_allowed"
	codeQueryTooShort    = "query_too_short"
	codeUnauthorized     = "unauthorized"
	// codeContentCorrupted 表示这一章的正文压缩块损坏（局部故障，非整库不可用）。
	codeContentCorrupted = "content_corrupted"

	// 上传接口的错误码（docs/specs/2026-10-25-import-formats.md §2）：
	// 前端按 code 分支给「逐个文件的友好提示」，message 才是给用户看的人话。
	codeUnsupportedFormat = "unsupported_format"
	// codeNotAppendable 表示追更时新文件不是已有章节的追加（章节变少或标题改了）：
	// 拒绝覆盖，用户可换回正确文件，或显式用 force 整本重导。
	codeNotAppendable   = "not_appendable"
	codePayloadTooLarge = "payload_too_large"
	codeTooManyFiles    = "too_many_files"
	codeEncryptedEPUB   = "encrypted_epub"
)

// writeJSON 以 JSON 写响应；序列化失败时退化为 500。
func writeJSON(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{\"error\":{\"code\":\"internal\",\"message\":\"响应序列化失败\"}}\n"))
		return
	}
	body = append(body, '\n')
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeError 写统一错误响应。
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// fail 把 store 层错误映射为 HTTP 响应：
// 未找到 → 404，入参非法 → 400，并发冲突 → 409，正文块损坏 → 503，其余 → 500。
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, store.ErrNotFound) || errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, codeNotFound, notFoundMsg)
	case errors.Is(err, store.ErrInvalidArgument):
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求参数非法")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, codeConflict, "存在进行中的任务或并发冲突")
	case errors.Is(err, store.ErrNotAppendable):
		s.logf("api: %s %s 追更被拒: %v", r.Method, r.URL.Path, err)
		writeError(w, http.StatusConflict, codeNotAppendable,
			"这份文件不是已有章节的追加（章节变少或标题不一致），已拒绝覆盖；确要整本重导请用 force")
	case errors.Is(err, store.ErrBlockCorrupted):
		s.failBlockCorrupted(w, r, err)
	default:
		s.logf("api: %s %s 内部错误: %v", r.Method, r.URL.Path, err)
		writeError(w, http.StatusInternalServerError, codeInternal, "内部错误")
	}
}

// failBlockCorrupted 处理「正文压缩块损坏」。
//
// 关键点：这是**局部故障**，不能让前端以为整个库坏了。
//   - HTTP 503 + 独立错误码 content_corrupted，前端可据此只在当前章显示
//     「本章读取失败」，其余 UI 照常；
//   - 消息里带上定位信息（哪一章、哪一块），但不暴露库内路径；
//   - 详情只进服务端日志，响应体保持精简。
func (s *Server) failBlockCorrupted(w http.ResponseWriter, r *http.Request, err error) {
	var be *store.BlockError
	if errors.As(err, &be) {
		s.logf("api: %s %s 正文块损坏: %s", r.Method, r.URL.Path, be.Error())
		writeError(w, http.StatusServiceUnavailable, codeContentCorrupted,
			fmt.Sprintf("本章正文所在的压缩块损坏（book_id=%d block_seq=%d），请联系管理员从备份恢复",
				be.BookID, be.Seq))
		return
	}
	s.logf("api: %s %s 正文块损坏: %v", r.Method, r.URL.Path, err)
	writeError(w, http.StatusServiceUnavailable, codeContentCorrupted, "本章正文读取失败")
}

// isNotFound 识别「未找到」：store.ErrNotFound / sql.ErrNoRows / 常见文案兜底。
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, frag := range []string{"not found", "no rows", "不存在", "未找到"} {
		if strings.Contains(msg, frag) {
			return true
		}
	}
	return false
}
