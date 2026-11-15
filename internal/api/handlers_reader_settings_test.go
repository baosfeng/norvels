package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 阅读设置同步接口（0028）：GET/PUT /api/settings/reader（以及 /api/reader/settings 别名）。
//
// 口径：
//   - 从没写过时 GET 返回 {"settings":null}，前端据此回落到本机 localStorage；
//   - PUT 接受 {"settings": ...}（settings 原文落盘）与裸对象（整包落盘）；
//   - 请求体不是单个 JSON 对象 → 400 invalid_param，且不覆盖已有设置；
//   - 设置跟会话账号走：两个账号各存各的。

const readerSettingsPath = "/api/settings/reader"

// readerSettingsAliasPath 是同一 handler 的第二组路径（前端历史拼写）。
const readerSettingsAliasPath = "/api/reader/settings"

// readerSettingsBody 是响应结构（settings 可以是 null 或任意 JSON）。
type readerSettingsBody struct {
	Settings json.RawMessage `json:"settings"`
}

// readerSettingsPut 以 JSON 请求体发一次 PUT。
func readerSettingsPut(e *testEnv, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodPut, readerSettingsPath, strings.NewReader(body),
		map[string]string{"Content-Type": "application/json"})
}

// sessionPut 以会话 Cookie 发一次 PUT（测账号隔离用）。
func sessionPut(e *testEnv, token, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodPut, readerSettingsPath, strings.NewReader(body), map[string]string{
		"Content-Type": "application/json", "Cookie": SessionCookieName + "=" + token,
	})
}

// TestReaderSettingsDefaultNull 没配置时返回 null（不是空串、也不是 404）。
func TestReaderSettingsDefaultNull(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{readerSettingsPath, readerSettingsAliasPath} {
		rr := e.get(path)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s = %d，body=%s", path, rr.Code, rr.Body.String())
		}
		if body := decodeBody[readerSettingsBody](t, rr); string(body.Settings) != "null" {
			t.Errorf("GET %s settings = %s，期望 null", path, body.Settings)
		}
	}
}

// TestReaderSettingsPutAndGet 写进去的设置原样读回，两个路径读到同一份。
func TestReaderSettingsPutAndGet(t *testing.T) {
	e := newEnv(t)
	payload := `{"theme":"sepia","fontSize":18,"paged":true}`
	rr := readerSettingsPut(e, `{"settings":`+payload+`}`)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 状态码 = %d，期望 204，body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Errorf("204 不该有响应体，实际 %q", rr.Body.String())
	}
	for _, path := range []string{readerSettingsPath, readerSettingsAliasPath} {
		if body := decodeBody[readerSettingsBody](t, e.get(path)); string(body.Settings) != payload {
			t.Errorf("GET %s settings = %s，期望 %s", path, body.Settings, payload)
		}
	}
}

// TestReaderSettingsPutBareObject 不带 settings 包装时整包落盘（前端直接 PUT 设置本体）。
func TestReaderSettingsPutBareObject(t *testing.T) {
	e := newEnv(t)
	rr := e.putJSON(readerSettingsPath, map[string]any{"theme": "dark", "fontSize": 22})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("裸对象 PUT = %d，期望 204，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[readerSettingsBody](t, e.get(readerSettingsPath))
	if string(got.Settings) != `{"fontSize":22,"theme":"dark"}` {
		t.Errorf("整包落盘 = %s，期望序列化后的对象", got.Settings)
	}
}

// TestReaderSettingsPutRejectsBadBody 请求体不合规一律 400 invalid_param，且不覆盖已有设置。
func TestReaderSettingsPutRejectsBadBody(t *testing.T) {
	e := newEnv(t)
	good := `{"theme":"sepia"}`
	if rr := readerSettingsPut(e, `{"settings":`+good+`}`); rr.Code != http.StatusNoContent {
		t.Fatalf("前置写入 = %d，body=%s", rr.Code, rr.Body.String())
	}

	cases := []struct{ name, body string }{
		{"空请求体", ""},
		{"不是对象", "[1,2,3]"},
		{"两个对象", `{"settings":{}}{"settings":{}}`},
		{"截断的 JSON", `{"settings":{"theme":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireError(t, readerSettingsPut(e, tc.body), http.StatusBadRequest, codeInvalidParam)
		})
	}
	if got := decodeBody[readerSettingsBody](t, e.get(readerSettingsPath)); string(got.Settings) != good {
		t.Errorf("被拒的请求改动了设置：%s", got.Settings)
	}
}

// TestReaderSettingsPerSession 设置跟着会话账号走：两个账号各存各的，互不可见。
func TestReaderSettingsPerSession(t *testing.T) {
	e := newEnv(t)
	_, _, tokenA := enterAs(t, e, "读者甲")
	_, _, tokenB := enterAs(t, e, "读者乙")
	if rr := sessionPut(e, tokenA, `{"settings":{"theme":"sepia"}}`); rr.Code != http.StatusNoContent {
		t.Fatalf("甲写入 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if rr := sessionPut(e, tokenB, `{"settings":{"theme":"dark"}}`); rr.Code != http.StatusNoContent {
		t.Fatalf("乙写入 = %d，body=%s", rr.Code, rr.Body.String())
	}
	for _, tc := range []struct{ name, token, want string }{
		{"甲", tokenA, `{"theme":"sepia"}`},
		{"乙", tokenB, `{"theme":"dark"}`},
	} {
		rr := e.do(http.MethodGet, readerSettingsPath, nil, withSession(tc.token))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s GET = %d，body=%s", tc.name, rr.Code, rr.Body.String())
		}
		if got := decodeBody[readerSettingsBody](t, rr); string(got.Settings) != tc.want {
			t.Errorf("%s settings = %s，期望 %s", tc.name, got.Settings, tc.want)
		}
	}
}

// TestReaderSettingsStoreDown store 挂了读写都是 500 internal（不是 404、不是静默成功）。
func TestReaderSettingsStoreDown(t *testing.T) {
	e := closedEnv(t)
	requireInternal(t, e.get(readerSettingsPath), "GET 阅读设置")
	requireInternal(t, readerSettingsPut(e, `{"settings":{"theme":"sepia"}}`), "PUT 阅读设置")
}
