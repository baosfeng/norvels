package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// authEnv 构造一个启用鉴权的测试环境。
func authEnv(t *testing.T, token string) *testEnv {
	t.Helper()
	return newEnvWith(t, Config{
		Version:          "slim",
		DisableAccessLog: true,
		AuthToken:        token,
	}, testFS())
}

func TestAuthOpenModeAllowsEverything(t *testing.T) {
	e := newEnv(t) // 默认 Config 无 token = 开放模式
	if rr := e.get("/api/books"); rr.Code != http.StatusOK {
		t.Errorf("开放模式 /api/books = %d，期望 200", rr.Code)
	}
	if rr := e.get("/api/health"); rr.Code != http.StatusOK {
		t.Errorf("开放模式 /api/health = %d，期望 200", rr.Code)
	}
	// 开放模式下访问 /login 直接跳首页（不渲染登录表单）。
	rr := e.get("/login")
	if rr.Code != http.StatusSeeOther {
		t.Errorf("开放模式 /login = %d，期望 303", rr.Code)
	}
}

func TestAuthRequiresTokenForAPI(t *testing.T) {
	e := authEnv(t, "test123")

	if rr := e.get("/api/books"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("无 token /api/books = %d，期望 401", rr.Code)
	} else {
		requireError(t, rr, http.StatusUnauthorized, codeUnauthorized)
		if rr.Header().Get("WWW-Authenticate") == "" {
			t.Error("401 应带 WWW-Authenticate 头")
		}
	}
	if rr := e.do(http.MethodGet, "/api/books", nil, map[string]string{
		"Authorization": "Bearer test123",
	}); rr.Code != http.StatusOK {
		t.Errorf("正确 Bearer token = %d，期望 200", rr.Code)
	}
	if rr := e.do(http.MethodGet, "/api/books", nil, map[string]string{
		"Authorization": "Bearer wrong",
	}); rr.Code != http.StatusUnauthorized {
		t.Errorf("错误 Bearer token = %d，期望 401", rr.Code)
	}
	if rr := e.do(http.MethodGet, "/api/books", nil, map[string]string{
		"Authorization": "Basic test123",
	}); rr.Code != http.StatusUnauthorized {
		t.Errorf("非 Bearer scheme = %d，期望 401", rr.Code)
	}
	// 大小写不敏感。
	if rr := e.do(http.MethodGet, "/api/books", nil, map[string]string{
		"Authorization": "bearer test123",
	}); rr.Code != http.StatusOK {
		t.Errorf("小写 bearer = %d，期望 200", rr.Code)
	}
}

func TestAuthHealthIsPublic(t *testing.T) {
	e := authEnv(t, "test123")
	rr := e.get("/api/health")
	if rr.Code != http.StatusOK {
		t.Fatalf("/api/health 免鉴权 = %d，期望 200（body=%s）", rr.Code, rr.Body.String())
	}
	body := decodeBody[healthResponse](t, rr)
	if body.Status != "ok" {
		t.Errorf("health status = %q", body.Status)
	}
	// 其余 API 仍需鉴权。
	if rr := e.get("/api/stats"); rr.Code != http.StatusUnauthorized {
		t.Errorf("/api/stats = %d，期望 401", rr.Code)
	}
}

func TestAuthPageRedirectsToLogin(t *testing.T) {
	e := authEnv(t, "test123")
	rr := e.get("/some/page?x=1")
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("页面无 token = %d，期望 303（body=%s）", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, LoginPath+"?next=") {
		t.Fatalf("Location = %q，期望 %s?next=…", loc, LoginPath)
	}
	next := strings.TrimPrefix(loc, LoginPath+"?next=")
	if decoded, err := url.QueryUnescape(next); err != nil || decoded != "/some/page?x=1" {
		t.Errorf("next 参数 = %q（decoded %q, err %v）", next, decoded, err)
	}
	// 静态资源也要鉴权（启用鉴权后不泄露前端产物）。
	if rr := e.get("/assets/app-abc123.js"); rr.Code != http.StatusSeeOther {
		t.Errorf("静态资源无 token = %d，期望 303", rr.Code)
	}
}

func TestLoginFormAndCookieFlow(t *testing.T) {
	e := authEnv(t, "test123")
	assertLoginPage(t, e)
	assertBadPassword(t, e)
	cookie := assertGoodPassword(t, e)
	assertCookieGrantsAPI(t, e, cookie)
	assertLogout(t, e)
}

// assertLoginPage 校验登录页是带 next 回填的 HTML 表单。
func assertLoginPage(t *testing.T, e *testEnv) {
	t.Helper()
	rr := e.get("/login?next=/api/books")
	if rr.Code != http.StatusOK {
		t.Fatalf("/login = %d，期望 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q，期望 text/html", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `action="/login"`) || !strings.Contains(body, `name="token"`) {
		t.Error("登录页缺少表单或口令输入框")
	}
	if !strings.Contains(body, `value="/api/books"`) {
		t.Error("登录页未回填 next")
	}
}

// assertBadPassword 校验错误口令返回 401 且不下发 Cookie。
func assertBadPassword(t *testing.T, e *testEnv) {
	t.Helper()
	bad := e.postForm("/login", url.Values{"token": {"nope"}, "next": {"/api/books"}})
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("错误口令 = %d，期望 401", bad.Code)
	}
	if !strings.Contains(bad.Body.String(), "口令不正确") {
		t.Error("错误口令页缺少提示文案")
	}
	if len(bad.Result().Cookies()) != 0 {
		t.Error("错误口令不应下发 Cookie")
	}
}

// assertGoodPassword 校验正确口令下发合法 Cookie 并返回该 Cookie。
func assertGoodPassword(t *testing.T, e *testEnv) *http.Cookie {
	t.Helper()
	ok := e.postForm("/login", url.Values{"token": {"test123"}, "next": {"/api/books"}})
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("正确口令 = %d，期望 303", ok.Code)
	}
	if loc := ok.Header().Get("Location"); loc != "/api/books" {
		t.Errorf("Location = %q，期望 /api/books", loc)
	}
	cookie := findCookie(t, ok, AuthCookieName)
	if !cookie.HttpOnly {
		t.Error("Cookie 缺 HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("Cookie SameSite = %v，期望 Lax", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("Cookie Path = %q，期望 /", cookie.Path)
	}
	return cookie
}

// assertCookieGrantsAPI 校验带 Cookie 可访问受保护 API。
func assertCookieGrantsAPI(t *testing.T, e *testEnv, cookie *http.Cookie) {
	t.Helper()
	rr := e.do(http.MethodGet, "/api/books", nil, map[string]string{
		"Cookie": AuthCookieName + "=" + cookie.Value,
	})
	if rr.Code != http.StatusOK {
		t.Errorf("带 Cookie 访问 = %d，期望 200（body=%s）", rr.Code, rr.Body.String())
	}
}

// assertLogout 校验登出把 Cookie 置为过期。
func assertLogout(t *testing.T, e *testEnv) {
	t.Helper()
	out := e.postForm("/logout", url.Values{})
	if out.Code != http.StatusSeeOther {
		t.Errorf("/logout = %d，期望 303", out.Code)
	}
	if c := findCookie(t, out, AuthCookieName); c.MaxAge >= 0 {
		t.Errorf("登出 Cookie MaxAge = %d，期望负数", c.MaxAge)
	}
}

// findCookie 取响应里的指定 Cookie。
func findCookie(t *testing.T, rr *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rr.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("响应里没有 Cookie %q（headers=%v）", name, rr.Header())
	return nil
}

// postForm 提交 application/x-www-form-urlencoded 请求。
func (e *testEnv) postForm(target string, form url.Values) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodPost, target, bytes.NewBufferString(form.Encode()), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

func TestSafeNextRejectsOpenRedirect(t *testing.T) {
	cases := map[string]string{
		"":                  "/",
		"/api/books":        "/api/books",
		"//evil.com":        "/",
		"http://evil.com":   "/",
		"https://evil.com/": "/",
		"relative":          "/",
		"  /books?x=1  ":    "/books?x=1",
	}
	for in, want := range cases {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestTokenMatches(t *testing.T) {
	if !tokenMatches("abc", "abc") {
		t.Error("相同令牌应匹配")
	}
	if tokenMatches("abc", "abd") {
		t.Error("不同令牌不应匹配")
	}
	if tokenMatches("", "") {
		t.Error("空令牌不应匹配（防止开放模式误判）")
	}
}

func TestBearerTokenParsing(t *testing.T) {
	cases := map[string]string{
		"Bearer xyz":   "xyz",
		"bearer xyz":   "xyz",
		"Basic xyz":    "",
		"Bearer":       "",
		"":             "",
		" Bearer tok ": "tok",
	}
	for in, want := range cases {
		if got := bearerToken(in); got != want {
			t.Errorf("bearerToken(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestAuthPublicPaths(t *testing.T) {
	for _, p := range []string{"/api/health", "/login", "/logout"} {
		if !authPublicPath(p) {
			t.Errorf("%s 应免鉴权", p)
		}
	}
	for _, p := range []string{"/api/books", "/", "/assets/app.js", "/api/stats"} {
		if authPublicPath(p) {
			t.Errorf("%s 不应免鉴权", p)
		}
	}
}
