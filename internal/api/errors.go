package api

import (
	"database/sql"
	"encoding/json"
	"errors"
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
// 未找到 → 404，入参非法 → 400，并发冲突 → 409，其余 → 500。
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, store.ErrNotFound) || errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, codeNotFound, notFoundMsg)
	case errors.Is(err, store.ErrInvalidArgument):
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求参数非法")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, codeConflict, "存在进行中的任务或并发冲突")
	default:
		s.logf("api: %s %s 内部错误: %v", r.Method, r.URL.Path, err)
		writeError(w, http.StatusInternalServerError, codeInternal, "内部错误")
	}
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
