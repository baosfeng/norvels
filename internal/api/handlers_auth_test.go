package api

import (
	"net/http"
	"strings"
	"testing"
)

// 账号端点的口径（0016）：
//   - 注册即登录 → 201 + 会话 Cookie；
//   - 登录账号不存在 → 404 +「请先注册」；重名注册 → 409；
//   - 会话 Cookie 的安全属性与旧口令 Cookie 一致。

// authUser 是账号响应的断言用结构（只认 id / name）。
type authUser struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// nameJSON 拼一个只含 name 的请求体。
func nameJSON(name string) string { return `{"name":"` + name + `"}` }

// withSession 带上会话 Cookie 的请求头。
func withSession(token string) map[string]string {
	return map[string]string{"Cookie": SessionCookieName + "=" + token}
}

// registerUser 注册一个账号并返回会话 Cookie 值与账号 id。
func registerUser(t *testing.T, e *testEnv, name string) (string, int64) {
	t.Helper()
	rr := e.postJSON("/api/auth/register", nameJSON(name))
	if rr.Code != http.StatusCreated {
		t.Fatalf("注册 %q = %d，期望 201，body=%s", name, rr.Code, rr.Body.String())
	}
	body := decodeBody[authUser](t, rr)
	c := findCookie(t, rr, SessionCookieName)
	return c.Value, body.ID
}

func TestAuthRegisterLoginMeLogout(t *testing.T) {
	e := newEnv(t)
	token, id := registerUser(t, e, "读者甲")

	me := e.do(http.MethodGet, "/api/auth/me", nil, withSession(token))
	if me.Code != http.StatusOK {
		t.Fatalf("/api/auth/me = %d，body=%s", me.Code, me.Body.String())
	}
	if got := decodeBody[authUser](t, me); got.ID != id || got.Name != "读者甲" {
		t.Errorf("/api/auth/me = %+v，期望 id=%d name=读者甲", got, id)
	}

	login := e.postJSON("/api/auth/login", nameJSON("读者甲"))
	if login.Code != http.StatusOK {
		t.Fatalf("登录 = %d，body=%s", login.Code, login.Body.String())
	}
	if c := findCookie(t, login, SessionCookieName); c.Value == "" || c.Value == token {
		t.Errorf("登录应下发新的会话 token（旧=%q 新=%q）", token, c.Value)
	}

	out := e.do(http.MethodPost, "/api/auth/logout", nil, withSession(token))
	if out.Code != http.StatusNoContent {
		t.Fatalf("登出 = %d，期望 204", out.Code)
	}
	if c := findCookie(t, out, SessionCookieName); c.MaxAge >= 0 {
		t.Errorf("登出 Cookie MaxAge = %d，期望负数", c.MaxAge)
	}
	me = e.do(http.MethodGet, "/api/auth/me", nil, withSession(token))
	if got := decodeBody[authUser](t, me); got.Name != "reader" {
		t.Errorf("登出后 me = %+v，期望回落默认账号 reader", got)
	}
}

func TestAuthRegisterValidation(t *testing.T) {
	e := newEnv(t)
	cases := map[string]struct {
		body string
		want int
	}{
		"空名":     {nameJSON(""), http.StatusBadRequest},
		"纯空白":    {nameJSON("   "), http.StatusBadRequest},
		"缺字段":    {"{}", http.StatusBadRequest},
		"非 JSON": {"不是 JSON", http.StatusBadRequest},
		"空 body": {"", http.StatusBadRequest},
		"超长":     {nameJSON(strings.Repeat("名", 33)), http.StatusBadRequest},
	}
	for name, c := range cases {
		if rr := e.postJSON("/api/auth/register", c.body); rr.Code != c.want {
			t.Errorf("%s 注册 = %d，期望 %d，body=%s", name, rr.Code, c.want, rr.Body.String())
		}
	}
}

func TestAuthRegisterConflict(t *testing.T) {
	e := newEnv(t)
	registerUser(t, e, "读者甲")
	rr := e.postJSON("/api/auth/register", nameJSON(" 读者甲 "))
	requireError(t, rr, http.StatusConflict, codeConflict)
	if msg := decodeBody[errResp](t, rr).Error.Message; !strings.Contains(msg, "账号名") {
		t.Errorf("重名提示 = %q，期望提到账号名", msg)
	}
}

func TestAuthLoginUnknownAccount(t *testing.T) {
	e := newEnv(t)
	rr := e.postJSON("/api/auth/login", nameJSON("查无此人"))
	requireError(t, rr, http.StatusNotFound, codeAccountNotFound)
	if msg := decodeBody[errResp](t, rr).Error.Message; !strings.Contains(msg, "请先注册") {
		t.Errorf("提示 = %q，期望包含「请先注册」", msg)
	}
}

// TestAuthSessionCookieAttributes 会话 Cookie 的安全属性与旧口令 Cookie 一致。
func TestAuthSessionCookieAttributes(t *testing.T) {
	e := newEnv(t)
	rr := e.postJSON("/api/auth/register", nameJSON("读者甲"))
	c := findCookie(t, rr, SessionCookieName)
	if !c.HttpOnly {
		t.Error("会话 Cookie 缺 HttpOnly")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("会话 Cookie SameSite = %v，期望 Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("会话 Cookie Path = %q，期望 /", c.Path)
	}
	if c.MaxAge != 30*24*3600 {
		t.Errorf("会话 Cookie MaxAge = %d，期望 30 天", c.MaxAge)
	}
}
