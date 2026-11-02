package acceptance

// 账号（0016）的验收步骤：只注册 + 输账号登录（无密码），个人记录按账号隔离。
//
// 为什么单独一个带 cookie jar 的 client：godog 的默认 client 不保存 Cookie，
// 而「登录之后我是谁」正是这一组场景要验的东西。jar 在场景 reset 时随 world 一起清空。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"

	"github.com/cucumber/godog"
)

// sessionCookieName 是账号会话 Cookie 名（与 internal/api.SessionCookieName 同值）。
const sessionCookieName = "norvels_session"

// registerAccountSteps 注册账号与我的书架的步骤。
func registerAccountSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^我注册账号 \"(.+)\"$", func(ctx context.Context, name string) error {
		return w.registerAccount(name)
	})
	sc.Step("^我尝试注册账号 \"(.+)\"$", func(ctx context.Context, name string) error {
		return w.attemptRegister(name)
	})
	sc.Step("^我登录不存在的账号 \"(.+)\"$", func(ctx context.Context, name string) error {
		return w.loginMissing(name)
	})
	sc.Step("^我把这本书加入书架$", func(ctx context.Context) error {
		return w.addToShelf()
	})
	sc.Step("^我的书架里应当有 (\\d+) 本书$", func(ctx context.Context, want string) error {
		return w.assertMyShelfCount(atoiSafe(want))
	})
	sc.Step("^我登出$", func(ctx context.Context) error {
		return w.logoutAccount()
	})
	sc.Step("^当前账号应当为 \"(.+)\"$", func(ctx context.Context, want string) error {
		return w.assertCurrentUser(want)
	})
	sc.Step("^接口应当返回 (\\d+) 账号不存在$", func(ctx context.Context, want string) error {
		return w.assertLastError(atoiSafe(want), "account_not_found")
	})
	sc.Step("^接口应当返回 (\\d+) 账号名冲突$", func(ctx context.Context, want string) error {
		return w.assertLastError(atoiSafe(want), "conflict")
	})
	// 单入口（不存在就建号）+ 删账号。
	sc.Step("^我进入账号 \"(.+)\"$", func(ctx context.Context, name string) error {
		return w.enterAccount(name)
	})
	sc.Step("^我再次进入账号 \"(.+)\"$", func(ctx context.Context, name string) error {
		return w.enterAccount(name)
	})
	sc.Step("^账号总数应当为 (\\d+)$", func(ctx context.Context, want string) error {
		return w.assertUserCount(atoiSafe(want))
	})
	sc.Step("^账号列表里 \"(.+)\" 的活跃时间应当非空$", func(ctx context.Context, name string) error {
		return w.assertLastSeenPresent(name)
	})
	sc.Step("^我删除当前账号$", func(ctx context.Context) error {
		return w.deleteCurrentAccount()
	})
	sc.Step("^我尝试删除账号 \"(.+)\"$", func(ctx context.Context, name string) error {
		return w.deleteAccountByName(name)
	})
	sc.Step("^我未登录时尝试删除账号 \"(.+)\"$", func(ctx context.Context, name string) error {
		return w.clearAccountCookies()
	})
	sc.Step("^接口应当返回 (\\d+) 默认账号不可删$", func(ctx context.Context, want string) error {
		return w.assertLastError(atoiSafe(want), "default_account")
	})
	sc.Step("^接口应当返回 (\\d+) 未授权$", func(ctx context.Context, want string) error {
		return w.assertLastError(atoiSafe(want), "unauthorized")
	})
}

// accountClient 返回本场景的账号 client（首次调用时建 jar）。
func (w *world) accountClient() (*http.Client, error) {
	if w.acct != nil {
		return w.acct, nil
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("建 cookie jar: %w", err)
	}
	w.acct = &http.Client{Jar: jar}
	return w.acct, nil
}

// acctDo 用账号 client 发一次请求；bodyJSON 非空时按 JSON 提交。
// 返回状态码与响应体，并同步 lastCode / lastBody。
func (w *world) acctDo(method, path, bodyJSON string) (int, []byte, http.Header, error) {
	if err := w.ensureDB(); err != nil {
		return 0, nil, nil, err
	}
	c, err := w.accountClient()
	if err != nil {
		return 0, nil, nil, err
	}
	var rdr io.Reader
	if bodyJSON != "" {
		rdr = strings.NewReader(bodyJSON)
	}
	req, err := http.NewRequest(method, w.srv.URL+path, rdr)
	if err != nil {
		return 0, nil, nil, err
	}
	if bodyJSON != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	w.lastCode, w.lastBody = resp.StatusCode, data
	return resp.StatusCode, data, resp.Header, err
}

// registerAccount 注册并断言 201，同时记下会话 token。
func (w *world) registerAccount(name string) error {
	code, body, hdr, err := w.acctDo(http.MethodPost, "/api/auth/register", nameJSONBody(name))
	if err != nil {
		return err
	}
	if code != http.StatusCreated {
		return fmt.Errorf("注册 %q 状态码 = %d，期望 201，body=%s", name, code, body)
	}
	w.lastSessionToken = headerSessionToken(hdr)
	return nil
}

// attemptRegister 只发注册请求（状态码由调用方断言）。
func (w *world) attemptRegister(name string) error {
	_, _, _, err := w.acctDo(http.MethodPost, "/api/auth/register", nameJSONBody(name))
	return err
}

// loginMissing 登录一个没注册过的账号（期望 404）。
func (w *world) loginMissing(name string) error {
	_, _, _, err := w.acctDo(http.MethodPost, "/api/auth/login", nameJSONBody(name))
	return err
}

// logoutAccount 登出，断言 204。
func (w *world) logoutAccount() error {
	code, body, _, err := w.acctDo(http.MethodPost, "/api/auth/logout", "")
	if err != nil {
		return err
	}
	if code != http.StatusNoContent {
		return fmt.Errorf("登出状态码 = %d，期望 204，body=%s", code, body)
	}
	return nil
}

// addToShelf 把当前书加入书架，断言 204。
func (w *world) addToShelf() error {
	if w.lastBookID == 0 {
		return fmt.Errorf("尚未导入书籍")
	}
	code, body, _, err := w.acctDo(http.MethodPut, fmt.Sprintf("/api/shelf/%d", w.lastBookID), "")
	if err != nil {
		return err
	}
	if code != http.StatusNoContent {
		return fmt.Errorf("加入书架状态码 = %d，期望 204，body=%s", code, body)
	}
	return nil
}

// shelfIDs 读当前账号书架的 bookId 列表。
//
// 显式带上 lastSessionToken：登出后 jar 里的 Cookie 已经没了，而「服务端是不是
// 真的撤销了会话」只能靠「拿旧 token 再请求一次」才验得出来。
func (w *world) shelfIDs() ([]int64, error) {
	c, err := w.accountClient()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, w.srv.URL+"/api/shelf/ids", nil)
	if err != nil {
		return nil, err
	}
	if w.lastSessionToken != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: w.lastSessionToken})
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /api/shelf/ids 状态码 = %d，body=%s", resp.StatusCode, data)
	}
	var res struct {
		BookIDs []int64 `json:"bookIds"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("解析书架响应失败: %w (body=%s)", err, data)
	}
	return res.BookIDs, nil
}

// assertMyShelfCount 断言当前账号「我的书架」里的书数。
//
// 与 steps_test.go 的 assertShelfCount 区分开：那个断言的是 /api/books
// 的**书库**总数（全部书都可浏览），这里断言的是 /api/shelf 的**个人收藏**。
func (w *world) assertMyShelfCount(want int) error {
	ids, err := w.shelfIDs()
	if err != nil {
		return err
	}
	if len(ids) != want {
		return fmt.Errorf("书架里的书 = %d 本，期望 %d 本（%v）", len(ids), want, ids)
	}
	return nil
}

// assertCurrentUser 断言 GET /api/auth/me 的账号名。
func (w *world) assertCurrentUser(want string) error {
	code, body, _, err := w.acctDo(http.MethodGet, "/api/auth/me", "")
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/auth/me 状态码 = %d，body=%s", code, body)
	}
	var u struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return fmt.Errorf("解析账号响应失败: %w (body=%s)", err, body)
	}
	if u.Name != want {
		return fmt.Errorf("当前账号 = %q，期望 %q", u.Name, want)
	}
	return nil
}

// assertLastError 断言最近一次响应的状态码与错误码。
func (w *world) assertLastError(wantCode int, wantErrCode string) error {
	if w.lastCode != wantCode {
		return fmt.Errorf("状态码 = %d，期望 %d，body=%s", w.lastCode, wantCode, w.lastBody)
	}
	var res struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.lastBody, &res); err != nil {
		return fmt.Errorf("解析错误响应失败: %w (body=%s)", err, w.lastBody)
	}
	if res.Error.Code != wantErrCode {
		return fmt.Errorf("错误码 = %q，期望 %q，body=%s", res.Error.Code, wantErrCode, w.lastBody)
	}
	return nil
}

// nameJSONBody 把账号名包成 {"name":...} 请求体。
func nameJSONBody(name string) string {
	b, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// headerSessionToken 从响应头里取出会话 token（没有则空串）。
func headerSessionToken(h http.Header) string {
	for _, line := range h.Values("Set-Cookie") {
		name, rest, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != sessionCookieName {
			continue
		}
		tok, _, _ := strings.Cut(rest, ";")
		return strings.TrimSpace(tok)
	}
	return ""
}
