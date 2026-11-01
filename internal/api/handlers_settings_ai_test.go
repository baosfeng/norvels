package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"norvels/internal/store"
)

// 本文件锁死 /api/settings/ai 的对外契约：默认关、读写往返、非法体 400、
// 没配 key 也允许开启、方法用错 405 且 Allow 同时列出 GET 与 PUT。
// 审核链路怎么用这个开关在 reviews_worker 侧（见 reviews_worker.go:52）。

// TestAISettingsGetDefaultsToDisabled 未配置时 GET 回 false，且 key 状态是派生值。
func TestAISettingsGetDefaultsToDisabled(t *testing.T) {
	env := newEnv(t)
	rr := env.get("/api/settings/ai")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET 状态码 = %d，期望 200（body=%s）", rr.Code, rr.Body.String())
	}
	body := decodeBody[aiSettingsBody](t, rr)
	if body.Enabled {
		t.Errorf("未配置时 enabled = true，期望 false")
	}
	if body.APIKeyConfigured {
		t.Errorf("测试配置没有 key，apiKeyConfigured 应为 false")
	}
}

// TestAISettingsPutRoundTrip PUT true → GET 回 true；PUT false → GET 回 false。
//
// 同时验落库的**字面值**是 "true"/"false"：store.AIEnabled 认 1/on/yes 等写法，
// 但写入口只写这两串，运维在库里一眼能看懂。
func TestAISettingsPutRoundTrip(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()

	for _, on := range []bool{true, false} {
		body := "{\"enabled\":false}"
		if on {
			body = "{\"enabled\":true}"
		}
		rr := env.do(http.MethodPut, "/api/settings/ai", strings.NewReader(body),
			map[string]string{"Content-Type": "application/json"})
		if rr.Code != http.StatusNoContent {
			t.Fatalf("PUT %s 状态码 = %d，期望 204（body=%s）", body, rr.Code, rr.Body.String())
		}
		if rr.Body.Len() != 0 {
			t.Errorf("PUT 应无响应体，实际 %q", rr.Body.String())
		}
		if got := decodeBody[aiSettingsBody](t, env.get("/api/settings/ai")).Enabled; got != on {
			t.Fatalf("PUT %s 后 GET enabled = %v", body, got)
		}
		raw, ok, err := env.st.GetSetting(ctx, store.SettingKeyAIEnabled)
		if err != nil || !ok {
			t.Fatalf("读回落库值: ok=%v err=%v", ok, err)
		}
		want := "false"
		if on {
			want = "true"
		}
		if raw != want {
			t.Errorf("落库值 = %q，期望 %q", raw, want)
		}
	}
}

// TestAISettingsRejectsInvalidBody 非 JSON / 空体 / 类型不符一律 400 invalid_param。
func TestAISettingsRejectsInvalidBody(t *testing.T) {
	env := newEnv(t)
	for _, body := range []string{"", "not-json", "[]", "{\"enabled\":\"yes\"}", "{\"enabled\":1}"} {
		rr := env.do(http.MethodPut, "/api/settings/ai", strings.NewReader(body),
			map[string]string{"Content-Type": "application/json"})
		requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	}
	// 非法请求不能顺手改库：迁移 0021 预置的 ai.enabled='false' 保持原样。
	// （库里本来就有这个键，所以判据是「值没被改写」，不是「查不到」。）
	raw, ok, err := env.st.GetSetting(context.Background(), store.SettingKeyAIEnabled)
	if err != nil || !ok || raw != "false" {
		t.Errorf("非法请求不该写库: value=%q ok=%v err=%v，期望预置的 false", raw, ok, err)
	}
}

// TestAISettingsWriteFailureIs500 写库失败走 s.fail → 500 internal。
func TestAISettingsWriteFailureIs500(t *testing.T) {
	env := newEnv(t)
	if err := env.st.Close(); err != nil {
		t.Fatalf("关闭 store: %v", err)
	}
	rr := env.do(http.MethodPut, "/api/settings/ai", strings.NewReader("{\"enabled\":true}"),
		map[string]string{"Content-Type": "application/json"})
	requireError(t, rr, http.StatusInternalServerError, codeInternal)
}

// TestAISettingsEnabledWithoutKeyStillWorks 没配 key 时开启也是 204——
// 提示交给前端（apiKeyConfigured=false），后端不把部署缺项判成非法请求。
func TestAISettingsEnabledWithoutKeyStillWorks(t *testing.T) {
	env := newEnv(t)
	rr := env.do(http.MethodPut, "/api/settings/ai", strings.NewReader("{\"enabled\":true}"),
		map[string]string{"Content-Type": "application/json"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 状态码 = %d，期望 204（body=%s）", rr.Code, rr.Body.String())
	}
	body := decodeBody[aiSettingsBody](t, env.get("/api/settings/ai"))
	if !body.Enabled || body.APIKeyConfigured {
		t.Errorf("GET = %+v，期望 enabled=true 且 apiKeyConfigured=false", body)
	}
}

// TestAISettingsConfiguredKeyIsReported 配了 key 时 GET 报 true（只报有没有，不回显值）。
func TestAISettingsConfiguredKeyIsReported(t *testing.T) {
	env := newEnvWith(t, Config{AIAPIKey: "sk-not-logged", DisableAccessLog: true}, testFS())
	body := decodeBody[aiSettingsBody](t, env.get("/api/settings/ai"))
	if !body.APIKeyConfigured {
		t.Error("配了 key 时 apiKeyConfigured 应为 true")
	}
	if strings.Contains(env.get("/api/settings/ai").Body.String(), "sk-not-logged") {
		t.Error("GET 响应不该回显 API key")
	}
}

// TestAISettingsMethodNotAllowed GET 打 PUT 路径 → 405，Allow 同列 GET 与 PUT。
// 依赖 static.go 的 apiRoutes 表：漏登记就掉 404，前端「动词用错」的分支失效。
func TestAISettingsMethodNotAllowed(t *testing.T) {
	env := newEnv(t)
	// POST 不属于该路径的任何方法：期望 405，而不是「路径不存在」的 404。
	rr := env.do(http.MethodPost, "/api/settings/ai", strings.NewReader("{}"),
		map[string]string{"Content-Type": "application/json"})
	requireError(t, rr, http.StatusMethodNotAllowed, codeMethodNotAllowed)
	allow := rr.Header().Get("Allow")
	for _, m := range []string{http.MethodGet, http.MethodPut} {
		if !strings.Contains(allow, m) {
			t.Errorf("Allow = %q，期望包含 %s", allow, m)
		}
	}
}
