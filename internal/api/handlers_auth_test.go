package api

import (
	"net/http"
	"strings"
	"testing"
	"time"
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

// apiUserItem / apiUserList 是账号列表响应的断言用结构。
type apiUserItem struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"createdAt"`
	LastSeenAt *string `json:"lastSeenAt"`
}

type apiUserList struct {
	Items []apiUserItem `json:"items"`
	Total int           `json:"total"`
}

// TestAuthListUsersDefaultAccount 新库只有默认账号：name=reader，从未登录序列化成 null。
func TestAuthListUsersDefaultAccount(t *testing.T) {
	e := newEnv(t)
	rr := e.get("/api/auth/users")
	if rr.Code != http.StatusOK {
		t.Fatalf("/api/auth/users = %d，body=%s", rr.Code, rr.Body.String())
	}
	list := decodeBody[apiUserList](t, rr)
	if len(list.Items) != 1 || list.Total != 1 {
		t.Fatalf("初始列表 = %+v，期望只有一条默认账号", list)
	}
	if got := list.Items[0]; got.Name != "reader" || got.LastSeenAt != nil {
		t.Errorf("默认账号 = %+v，期望 name=reader、lastSeenAt=null", got)
	}
	if body := rr.Body.String(); !strings.Contains(body, `"lastSeenAt":null`) {
		t.Errorf("从未登录应序列化成 null，body=%s", body)
	}
}

// TestAuthListUsersOrder 最近活跃在前：注册只建号不刷新 last_seen_at，登录一次才排到最前。
func TestAuthListUsersOrder(t *testing.T) {
	e := newEnv(t)
	_, id := registerUser(t, e, "读者甲")
	if _, err := e.st.RegisterUser(t.Context(), "从未登录"); err != nil {
		t.Fatalf("建号(从未登录): %v", err)
	}
	if login := e.postJSON("/api/auth/login", nameJSON("读者甲")); login.Code != http.StatusOK {
		t.Fatalf("登录 = %d，body=%s", login.Code, login.Body.String())
	}

	list := decodeBody[apiUserList](t, e.get("/api/auth/users"))
	if len(list.Items) != 3 || list.Total != 3 {
		t.Fatalf("列表 = %+v，期望 3 条账号", list)
	}
	if list.Items[0].ID != id || list.Items[0].LastSeenAt == nil {
		t.Errorf("首条 = %+v，期望刚登录的账号且 lastSeenAt 非 null", list.Items[0])
	}
	for i, want := range []string{"从未登录", "reader"} {
		item := list.Items[i+1]
		if item.Name != want || item.LastSeenAt != nil {
			t.Errorf("第 %d 条 = %+v，期望 name=%s、lastSeenAt=null", i+2, item, want)
		}
	}
}

// TestAuthListUsersTimes 时间字段是 RFC3339：前端的相对时间戳直接吃 Date.parse。
func TestAuthListUsersTimes(t *testing.T) {
	e := newEnv(t)
	registerUser(t, e, "读者甲")
	if login := e.postJSON("/api/auth/login", nameJSON("读者甲")); login.Code != http.StatusOK {
		t.Fatalf("登录 = %d，body=%s", login.Code, login.Body.String())
	}

	first := decodeBody[apiUserList](t, e.get("/api/auth/users")).Items[0]
	if first.LastSeenAt == nil {
		t.Fatalf("首条 = %+v，期望 lastSeenAt 非 null", first)
	}
	if _, err := time.Parse(time.RFC3339, *first.LastSeenAt); err != nil {
		t.Errorf("lastSeenAt = %q 不是 RFC3339: %v", *first.LastSeenAt, err)
	}
	if _, err := time.Parse(time.RFC3339, first.CreatedAt); err != nil {
		t.Errorf("createdAt = %q 不是 RFC3339: %v", first.CreatedAt, err)
	}
}

// TestAuthListUsersIsPublic 账号列表免鉴权（登录页得先拿到它），/api/auth/me 仍要求凭证。
func TestAuthListUsersIsPublic(t *testing.T) {
	e := newEnvWith(t, Config{AuthToken: "s3cret", DisableAccessLog: true}, testFS())
	if rr := e.get("/api/auth/users"); rr.Code != http.StatusOK {
		t.Fatalf("未带凭证访问账号列表 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	if rr := e.get("/api/auth/me"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("未带凭证访问 /api/auth/me = %d，期望 401", rr.Code)
	}
}
