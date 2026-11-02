package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// 单入口 + 删账号 + 活跃时间回显（本轮收敛的三件事）：
//   - POST /api/auth/enter：不存在就建号并登录（201），存在就直接登录（200）；
//   - 注册 / 登录 / 单入口返回的 lastSeenAt 必须是落库值（列表据此显示「上次活跃」）；
//   - DELETE /api/auth/users/{id}：删自己的账号并清个人数据；默认账号 409；未登录 401；不存在 404。

// authUserSeen 是带活跃时间的账号响应（断言「进入之后有真实时间」）。
type authUserSeen struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	LastSeenAt string `json:"lastSeenAt"`
}

// enterSeen 发单入口请求并按带活跃时间的结构解析。
func enterSeen(t *testing.T, e *testEnv, name string) (int, authUserSeen, string) {
	t.Helper()
	rr := e.postJSON("/api/auth/enter", nameJSON(name))
	return rr.Code, decodeBody[authUserSeen](t, rr), sessionCookieValue(rr)
}

// sessionOf 用 name 登录一次，返回下发的会话 token。
func (e *testEnv) sessionOf(t *testing.T, name string) string {
	t.Helper()
	rr := e.postJSON("/api/auth/login", nameJSON(name))
	if rr.Code != http.StatusOK {
		t.Fatalf("登录 %q = %d，body=%s", name, rr.Code, rr.Body.String())
	}
	return findCookie(t, rr, SessionCookieName).Value
}

// sessionCookieValue 取响应里会话 Cookie 的值（没有则空串，不 Fatalf）。
func sessionCookieValue(rr *httptest.ResponseRecorder) string {
	for _, c := range rr.Result().Cookies() {
		if c.Name == SessionCookieName {
			return c.Value
		}
	}
	return ""
}

// enterAs 发一次单入口请求，返回状态码、账号与会话 token。
func enterAs(t *testing.T, e *testEnv, name string) (int, authUser, string) {
	t.Helper()
	rr := e.postJSON("/api/auth/enter", nameJSON(name))
	return rr.Code, decodeBody[authUser](t, rr), sessionCookieValue(rr)
}

func TestAuthEnterCreatesThenLogsIn(t *testing.T) {
	e := newEnv(t)
	code, created, token := enterSeen(t, e, "读者甲")
	if code != http.StatusCreated {
		t.Fatalf("首次进入 = %d，期望 201", code)
	}
	if token == "" {
		t.Error("首次进入没有下发会话 Cookie")
	}

	code, again, token2 := enterSeen(t, e, "读者甲")
	if code != http.StatusOK {
		t.Fatalf("再次进入 = %d，期望 200", code)
	}
	if again.ID != created.ID {
		t.Errorf("再次进入的 id = %d，期望 %d（不该新建账号）", again.ID, created.ID)
	}
	if token2 == "" || token2 == token {
		t.Errorf("再次进入应下发新会话 token（旧=%q 新=%q）", token, token2)
	}
	if again.LastSeenAt == "" {
		t.Error("进入后 lastSeenAt 为空：账号列表会显示「从未登录」")
	}
}

// TestAuthEnterReflectsLastSeenInList 进入之后账号列表里带真实活跃时间与排序。
func TestAuthEnterReflectsLastSeenInList(t *testing.T) {
	e := newEnv(t)
	_, created, _ := enterSeen(t, e, "读者甲")

	list := decodeBody[apiUserList](t, e.get("/api/auth/users"))
	var got *apiUserItem
	for i := range list.Items {
		if list.Items[i].ID == created.ID {
			got = &list.Items[i]
		}
	}
	if got == nil {
		t.Fatalf("列表里没有刚进入的账号：%+v", list.Items)
	}
	if got.LastSeenAt == nil || *got.LastSeenAt == "" {
		t.Errorf("账号 %d 的 lastSeenAt = %v，期望非空", got.ID, got.LastSeenAt)
	}
	if list.Items[0].ID != created.ID {
		t.Errorf("首条 = %d，期望刚进入的 %d 排最前", list.Items[0].ID, created.ID)
	}
}

// TestAuthMeEchoesLastSeen me() 也回显落库的活跃时间（侧栏「当前账号」用它）。
func TestAuthMeEchoesLastSeen(t *testing.T) {
	e := newEnv(t)
	_, created, token := enterSeen(t, e, "读者甲")
	me := e.do(http.MethodGet, "/api/auth/me", nil, withSession(token))
	if me.Code != http.StatusOK {
		t.Fatalf("/api/auth/me = %d，body=%s", me.Code, me.Body.String())
	}
	got := decodeBody[struct {
		ID         int64  `json:"id"`
		LastSeenAt string `json:"lastSeenAt"`
	}](t, me)
	if got.ID != created.ID || got.LastSeenAt == "" {
		t.Errorf("/api/auth/me = %+v，期望 id=%d 且 lastSeenAt 非空", got, created.ID)
	}
}

// TestAuthRegisterEchoesLastSeen 注册即登录：响应体里也带活跃时间。
func TestAuthRegisterEchoesLastSeen(t *testing.T) {
	e := newEnv(t)
	rr := e.postJSON("/api/auth/register", nameJSON("读者甲"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("注册 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"lastSeenAt":"`) {
		t.Errorf("注册响应没有 lastSeenAt，body=%s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"lastSeenAt":""`) {
		t.Errorf("注册后 lastSeenAt 是空串：账号列表会显示「从未登录」，body=%s", rr.Body.String())
	}
}

// TestAuthDeleteUserRemovesSelfAndData 删当前账号：204 + 清 Cookie + 账号与个人数据都没了。
func TestAuthDeleteUserRemovesSelfAndData(t *testing.T) {
	e := newEnv(t)
	_, id := registerUser(t, e, "读者甲")
	book := e.seedBook("长夜未明", "林砚")
	token := e.sessionOf(t, "读者甲")
	if rr := e.do(http.MethodPut, "/api/shelf/"+strconv.FormatInt(book, 10), nil, withSession(token)); rr.Code != http.StatusNoContent {
		t.Fatalf("加入书架 = %d，body=%s", rr.Code, rr.Body.String())
	}

	rr := e.do(http.MethodDelete, "/api/auth/users/"+strconv.FormatInt(id, 10), nil, withSession(token))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("删账号 = %d，期望 204，body=%s", rr.Code, rr.Body.String())
	}
	if c := sessionCookieValue(rr); c != "" {
		t.Errorf("删账号应清会话 Cookie，实际 = %q", c)
	}
	// 账号没了：旧会话换不出账号，me() 回落默认账号。
	me := e.do(http.MethodGet, "/api/auth/me", nil, withSession(token))
	if got := decodeBody[authUser](t, me); got.Name != "reader" {
		t.Errorf("删后 me = %+v，期望回落默认账号", got)
	}
	list := decodeBody[apiUserList](t, e.get("/api/auth/users"))
	for _, item := range list.Items {
		if item.ID == id {
			t.Errorf("删后列表里还有账号 %d", id)
		}
	}
	// 书还在（删账号不删书）。
	if got := e.get("/api/books/" + strconv.FormatInt(book, 10)); got.Code != http.StatusOK {
		t.Errorf("删账号后书 = %d，期望 200", got.Code)
	}
}

// TestAuthDeleteUserRejectsDefault 默认账号 409 default_account。
func TestAuthDeleteUserRejectsDefault(t *testing.T) {
	e := newEnv(t)
	_, _ = registerUser(t, e, "读者甲")
	token := e.sessionOf(t, "读者甲")
	rr := e.do(http.MethodDelete, "/api/auth/users/1", nil, withSession(token))
	requireError(t, rr, http.StatusConflict, codeDefaultAccount)
}

// TestAuthDeleteUserRequiresLogin 没会话 401（删除是不可逆写操作，门外的人不能按 id 试）。
func TestAuthDeleteUserRequiresLogin(t *testing.T) {
	e := newEnv(t)
	_, id := registerUser(t, e, "读者甲")
	rr := e.do(http.MethodDelete, "/api/auth/users/"+strconv.FormatInt(id, 10), nil, nil)
	requireError(t, rr, http.StatusUnauthorized, codeUnauthorized)
	// 账号仍在
	list := decodeBody[apiUserList](t, e.get("/api/auth/users"))
	found := false
	for _, item := range list.Items {
		if item.ID == id {
			found = true
		}
	}
	if !found {
		t.Error("未登录的删除请求把账号删掉了")
	}
}

// TestAuthDeleteUserUnknown 不存在 404 not_found；id 非正整数 400 invalid_id。
func TestAuthDeleteUserUnknown(t *testing.T) {
	e := newEnv(t)
	_, _ = registerUser(t, e, "读者甲")
	token := e.sessionOf(t, "读者甲")
	requireError(t, e.do(http.MethodDelete, "/api/auth/users/99999", nil, withSession(token)),
		http.StatusNotFound, codeNotFound)
	requireError(t, e.do(http.MethodDelete, "/api/auth/users/abc", nil, withSession(token)),
		http.StatusBadRequest, codeInvalidID)
}
