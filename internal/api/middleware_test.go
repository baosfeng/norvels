package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPanicRecovery(t *testing.T) {
	env := newEnv(t)
	h := env.srv.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("测试用 panic")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/panic", nil))
	requireError(t, rr, http.StatusInternalServerError, codeInternal)
}

func TestGzipCompression(t *testing.T) {
	env := newEnv(t)
	payload := strings.Repeat("梅宁市华夏国的夜色深沉。", 100)
	h := env.srv.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, "{\"content\":\""+payload+"\"}")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/big", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := rr.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q，期望 gzip", got)
	}
	if !strings.Contains(rr.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("Vary = %q，期望包含 Accept-Encoding", rr.Header().Get("Vary"))
	}
	if cl := rr.Header().Get("Content-Length"); cl != "" {
		t.Fatalf("压缩响应不应保留 Content-Length，实际 %q", cl)
	}
	zr, err := gzip.NewReader(bytes.NewReader(rr.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer zr.Close()
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	if got := string(plain); got != "{\"content\":\""+payload+"\"}" {
		t.Fatalf("解压后内容不一致，长度 %d", len(got))
	}

	// 不带 Accept-Encoding：不压缩
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/api/big", nil))
	if got := rr2.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("未协商 gzip 时不应压缩，Content-Encoding = %q", got)
	}
	if !strings.Contains(rr2.Body.String(), "梅宁市") {
		t.Fatalf("未压缩响应内容异常: %s", rr2.Body.String())
	}
}

func TestGzipSkipsSmallBody(t *testing.T) {
	env := newEnv(t)
	h := env.srv.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, "{\"ok\":true}")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/small", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got := rr.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("小响应不应压缩，Content-Encoding = %q", got)
	}
	if !strings.Contains(rr.Body.String(), "ok") {
		t.Fatalf("响应内容异常: %s", rr.Body.String())
	}
}

func TestGzipSkipsRangeRequests(t *testing.T) {
	env := newEnv(t)
	h := env.srv.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Range", "bytes 0-3/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "abcd")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/range", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-3")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got := rr.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Range 请求不应压缩，Content-Encoding = %q", got)
	}
	if rr.Code != http.StatusPartialContent || rr.Body.String() != "abcd" {
		t.Fatalf("Range 响应 = %d %q", rr.Code, rr.Body.String())
	}
}

func TestCORSMiddleware(t *testing.T) {
	env := newEnvWith(t, Config{Version: "slim", EnableCORS: true, DisableAccessLog: true}, testFS())

	rr := env.do(http.MethodOptions, "/api/health", nil, map[string]string{
		"Origin":                        "http://localhost:5173",
		"Access-Control-Request-Method": "GET",
	})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("预检状态码 = %d，期望 204", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}

	// 非白名单来源：不返回放行头
	rr2 := env.do(http.MethodOptions, "/api/health", nil, map[string]string{"Origin": "http://evil.example"})
	if got := rr2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("非白名单来源不应放行，实际 %q", got)
	}

	// 普通请求带白名单来源
	rr3 := env.do(http.MethodGet, "/api/health", nil, map[string]string{"Origin": "http://127.0.0.1:5173"})
	if got := rr3.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("ACAO = %q", got)
	}
}

func TestCORSDisabledByDefault(t *testing.T) {
	env := newEnv(t)
	rr := env.do(http.MethodGet, "/api/health", nil, map[string]string{"Origin": "http://localhost:5173"})
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("未开启 CORS 时不应返回 ACAO，实际 %q", got)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	env := newEnvWith(t, Config{Version: "slim", RequestID: true, DisableAccessLog: true}, testFS())
	rr := env.get("/api/health")
	if rr.Header().Get("X-Request-Id") == "" {
		t.Fatal("开启 RequestID 后应有 X-Request-Id 响应头")
	}

	// 客户端传入的 ID 应被透传
	rr2 := env.do(http.MethodGet, "/api/health", nil, map[string]string{"X-Request-Id": "req-123"})
	if got := rr2.Header().Get("X-Request-Id"); got != "req-123" {
		t.Fatalf("X-Request-Id = %q，期望透传 req-123", got)
	}

	// 关闭时无该响应头
	off := newEnv(t).get("/api/health")
	if off.Header().Get("X-Request-Id") != "" {
		t.Fatal("关闭 RequestID 后不应有 X-Request-Id")
	}
}

func TestAccessLog(t *testing.T) {
	env := newEnvWith(t, Config{Version: "slim", RequestID: true}, testFS())
	var buf bytes.Buffer
	env.srv.log = newTestLogger(&buf)

	if rr := env.get("/api/health"); rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rr.Code)
	}
	out := buf.String()
	if !strings.Contains(out, "GET /api/health 200") {
		t.Fatalf("访问日志缺少 method/path/status: %q", out)
	}
	if !strings.Contains(out, "rid=") {
		t.Fatalf("开启 RequestID 后访问日志应包含 rid: %q", out)
	}
}

func TestAPI404IsJSON(t *testing.T) {
	env := newEnv(t)
	rr := env.get("/api/does-not-exist")
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q，期望 JSON", ct)
	}
	requireError(t, rr, http.StatusNotFound, codeNotFound)
}
