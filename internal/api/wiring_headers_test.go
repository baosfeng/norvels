package api

import (
	"net/http"
	"testing"
)

// 本文件是 P4-8 的接线证据：配置打开后**响应真的带上了头**，关闭时一个都不带。
// 既有的 TestCORSMiddleware / TestRequestIDMiddleware 直接构造 api.Config，
// 证明的是中间件本身；这里补的是「cmd → api.Config → 中间件」这条链没断。

// TestWiringCORSOnOff 开关打开时响应头含 Access-Control-Allow-Origin，关闭时不含。
func TestWiringCORSOnOff(t *testing.T) {
	on := newEnvWith(t, Config{Version: "cors-on", EnableCORS: true, DisableAccessLog: true}, testFS())
	rr := on.do(http.MethodGet, "/api/health", nil, map[string]string{"Origin": "http://localhost:5173"})
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("打开 CORS 后 ACAO = %q，期望 http://localhost:5173", got)
	}

	off := newEnv(t)
	rr2 := off.do(http.MethodGet, "/api/health", nil, map[string]string{"Origin": "http://localhost:5173"})
	if got := rr2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("关闭 CORS 后不应有 ACAO，实际 %q", got)
	}
}

// TestWiringRequestIDOnOff 开关打开时响应头含非空 X-Request-Id，关闭时不含。
func TestWiringRequestIDOnOff(t *testing.T) {
	on := newEnvWith(t, Config{Version: "rid-on", RequestID: true, DisableAccessLog: true}, testFS())
	rr := on.get("/api/health")
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("打开 RequestID 后 X-Request-Id 不应为空")
	}

	off := newEnv(t)
	if got := off.get("/api/health").Header().Get("X-Request-Id"); got != "" {
		t.Fatalf("关闭 RequestID 后不应有 X-Request-Id，实际 %q", got)
	}
}
