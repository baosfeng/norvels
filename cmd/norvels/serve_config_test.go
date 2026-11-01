package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"norvels/internal/api"
	"norvels/internal/config"
	"norvels/internal/store"
)

// 本文件锁死 P4-8 的 serve 配置装配：cmd 只把标量搬进 api.Config，
// 不依赖 internal/recommend（权重类型的叠加留在 internal/api）。

// TestBuildAPIConfigPassesWiring 三项开关与三项权重覆盖原样透传。
func TestBuildAPIConfigPassesWiring(t *testing.T) {
	w := 4.5
	cfg := config.Config{
		EnableCORS:          true,
		RequestID:           true,
		RecWeightInterestL1: &w,
	}
	apiCfg := buildAPIConfig(cfg, context.Background(), "", "/tmp/data/norvels.db")
	if !apiCfg.EnableCORS || !apiCfg.RequestID {
		t.Errorf("开发期开关没透传: CORS=%v RequestID=%v", apiCfg.EnableCORS, apiCfg.RequestID)
	}
	if apiCfg.RecWeightInterestL1 == nil || *apiCfg.RecWeightInterestL1 != 4.5 {
		t.Errorf("InterestL1 覆盖没透传: %v", apiCfg.RecWeightInterestL1)
	}
}

// TestBuildAPIConfigWiringDefaultsOff 不配置时全部为零值 / nil：默认行为不变。
func TestBuildAPIConfigWiringDefaultsOff(t *testing.T) {
	apiCfg := buildAPIConfig(config.Config{}, context.Background(), "", "/tmp/data/norvels.db")
	if apiCfg.EnableCORS || apiCfg.RequestID {
		t.Errorf("默认应全关，得到 CORS=%v RequestID=%v", apiCfg.EnableCORS, apiCfg.RequestID)
	}
	if apiCfg.RecWeightInterestL1 != nil || apiCfg.RecWeightHotReads != nil || apiCfg.RecWeightFreshness != nil {
		t.Errorf("默认权重覆盖应为 nil: %+v", apiCfg)
	}
}

// TestServeWiringEndToEnd 端到端：config → buildAPIConfig → api.New → 真实 HTTP 响应。
//
// 这是「接入后真的可用」的最终证据：断言的不是结构体字段，而是开关打开时
// 响应里真的多了 Allow-Origin / X-Request-Id 两个头，关闭时一个都没有。
func TestServeWiringEndToEnd(t *testing.T) {
	on := newServeHandler(t, config.Config{EnableCORS: true, RequestID: true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	on.ServeHTTP(rr, req)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("打开 CORS 后 ACAO = %q", got)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("打开 RequestID 后 X-Request-Id 不应为空")
	}

	off := newServeHandler(t, config.Config{})
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req2.Header.Set("Origin", "http://localhost:5173")
	off.ServeHTTP(rr2, req2)
	if got := rr2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("关闭 CORS 后不应有 ACAO，实际 %q", got)
	}
	if got := rr2.Header().Get("X-Request-Id"); got != "" {
		t.Fatalf("关闭 RequestID 后不应有 X-Request-Id，实际 %q", got)
	}
}

// newServeHandler 按 cfg 组装一个和 serve 子命令同构的 handler。
func newServeHandler(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "norvels.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	apiCfg := buildAPIConfig(cfg, ctx, "", dbPath)
	apiCfg.DisableAccessLog = true
	return api.New(st, apiCfg)
}
