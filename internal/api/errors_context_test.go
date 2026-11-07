package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 客户端在响应前断开（fetch abort / 切页 / 关标签页）不是服务端故障：
// 必须回 499 + client_closed，并且不写「内部错误」日志、不计 5xx。
// 之前的实现把 context.Canceled 兜底成 500，实测 915 次请求里 77 次「500」全是取消，
// /api/metrics 的 errors 被它占满，真实故障反而看不见。
func TestFailContextCanceled(t *testing.T) {
	env := newEnv(t)
	rr := httptest.NewRecorder()
	env.srv.fail(rr, httptest.NewRequest(http.MethodGet, "/api/books", nil), context.Canceled, "书不存在")
	requireError(t, rr, statusClientClosedRequest, codeClientClosed)
}

// 服务端自己的超时（context.DeadlineExceeded）是 504，不是 500。
func TestFailDeadlineExceeded(t *testing.T) {
	env := newEnv(t)
	rr := httptest.NewRecorder()
	env.srv.fail(rr, httptest.NewRequest(http.MethodGet, "/api/books", nil), context.DeadlineExceeded, "书不存在")
	requireError(t, rr, http.StatusGatewayTimeout, codeTimeout)
}

// 普通错误仍然走 500：canceled 分支不能把真正的故障吞掉。
func TestFailOtherErrorStill500(t *testing.T) {
	env := newEnv(t)
	rr := httptest.NewRecorder()
	env.srv.fail(rr, httptest.NewRequest(http.MethodGet, "/api/books", nil), errors.New("db down"), "书不存在")
	requireError(t, rr, http.StatusInternalServerError, codeInternal)
}

// 推荐库同口径：取数途中客户端断开，不算推荐库故障。
func TestFailRecContextCanceled(t *testing.T) {
	env := newEnv(t)
	rr := httptest.NewRecorder()
	env.srv.failRec(rr, httptest.NewRequest(http.MethodPost, "/api/events", nil), context.Canceled, "埋点没记上")
	requireError(t, rr, statusClientClosedRequest, codeClientClosed)
}
