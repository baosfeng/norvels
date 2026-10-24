package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{name: "健康", status: http.StatusOK, body: `{"status":"ok","db":"ok"}`},
		{name: "状态带空格也健康", status: http.StatusOK, body: `{"status": "ok", "db": "ok"}`},
		{name: "状态非 ok", status: http.StatusOK, body: `{"status":"degraded","db":"ok"}`, wantErr: true},
		{name: "500", status: http.StatusInternalServerError, body: `{"status":"ok"}`, wantErr: true},
		{name: "非 JSON", status: http.StatusOK, body: `oops`, wantErr: true},
		{name: "空对象", status: http.StatusOK, body: `{}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/health" {
					t.Errorf("请求路径 = %q，期望 /api/health", r.URL.Path)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			err := check(srv.URL+"/api/health", time.Second)
			if tt.wantErr && err == nil {
				t.Fatal("期望返回错误，实际为 nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("期望成功，实际错误: %v", err)
			}
		})
	}
}

func TestCheckInvalidURL(t *testing.T) {
	for _, raw := range []string{"http://", "https://127.0.0.1:8080/api/health", "127.0.0.1:8080"} {
		if err := check(raw, 200*time.Millisecond); err == nil {
			t.Fatalf("地址 %q 期望返回错误，实际为 nil", raw)
		}
	}
}

func TestCheckConnectionError(t *testing.T) {
	// 关闭后的服务应连接失败。
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	raw := srv.URL
	srv.Close()

	err := check(raw+"/api/health", 300*time.Millisecond)
	if err == nil {
		t.Fatal("期望连接失败，实际为 nil")
	}
	if !strings.Contains(err.Error(), "连接") {
		t.Fatalf("错误信息应包含连接失败原因，实际: %v", err)
	}
}
