package acceptance

// 单入口（不存在就建号）与删账号的验收步骤。
//
// 与 account_steps_test.go 分两个文件：那个文件放「注册 / 登录 / 登出 / 书架」的既有口径，
// 这里放本轮的收敛——一个入口进来、能删掉自己的账号（个人数据跟着走，书不动）。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// enterAccount 走单入口（不存在就建号）并断言 2xx，同时记下会话 token。
func (w *world) enterAccount(name string) error {
	code, body, hdr, err := w.acctDo(http.MethodPost, "/api/auth/enter", nameJSONBody(name))
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusCreated {
		return fmt.Errorf("进入 %q 状态码 = %d，期望 200/201，body=%s", name, code, body)
	}
	w.lastSessionToken = headerSessionToken(hdr)
	return nil
}

// listUsers 读账号列表（免鉴权端点，用带 jar 的 client）。
func (w *world) listUsers() ([]userRow, error) {
	code, body, _, err := w.acctDo(http.MethodGet, "/api/auth/users", "")
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("GET /api/auth/users 状态码 = %d，body=%s", code, body)
	}
	var res struct {
		Items []userRow `json:"items"`
		Total int       `json:"total"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("解析账号列表失败: %w (body=%s)", err, body)
	}
	return res.Items, nil
}

// userRow 是账号列表的一行（断言用）。
type userRow struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	LastSeenAt *string `json:"lastSeenAt"`
}

// assertUserCount 断言账号总数（列表 total）。
func (w *world) assertUserCount(want int) error {
	users, err := w.listUsers()
	if err != nil {
		return err
	}
	// total 与 items 在本组场景里一致（账号数远小于 50 的封顶）。
	if len(users) != want {
		return fmt.Errorf("账号总数 = %d，期望 %d", len(users), want)
	}
	return nil
}

// assertLastSeenPresent 断言某个账号在列表里带真实的活跃时间（不再是「从未登录」）。
func (w *world) assertLastSeenPresent(name string) error {
	users, err := w.listUsers()
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.Name == name {
			if u.LastSeenAt == nil || *u.LastSeenAt == "" {
				return fmt.Errorf("账号 %q 的 lastSeenAt = null：进入之后仍显示「从未登录」", name)
			}
			return nil
		}
	}
	return fmt.Errorf("账号列表里没有 %q", name)
}

// userIDByName 从账号列表里取 id（删账号步骤用）。
func (w *world) userIDByName(name string) (int64, error) {
	users, err := w.listUsers()
	if err != nil {
		return 0, err
	}
	for _, u := range users {
		if u.Name == name {
			return u.ID, nil
		}
	}
	return 0, fmt.Errorf("账号列表里没有 %q", name)
}

// deleteCurrentAccount 删当前登录的账号，断言 204。
func (w *world) deleteCurrentAccount() error {
	code, body, _, err := w.acctDo(http.MethodGet, "/api/auth/me", "")
	if err != nil {
		return err
	}
	var me struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return fmt.Errorf("解析当前账号失败: %w (body=%s)", err, body)
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/auth/me 状态码 = %d，body=%s", code, body)
	}
	delCode, delBody, _, err := w.acctDo(http.MethodDelete, fmt.Sprintf("/api/auth/users/%d", me.ID), "")
	if err != nil {
		return err
	}
	if delCode != http.StatusNoContent {
		return fmt.Errorf("删除账号 %d 状态码 = %d，期望 204，body=%s", me.ID, delCode, delBody)
	}
	return nil
}

// deleteAccountByName 按名字删账号（状态码由调用方断言）。
func (w *world) deleteAccountByName(name string) error {
	id, err := w.userIDByName(name)
	if err != nil {
		return err
	}
	_, _, _, err = w.acctDo(http.MethodDelete, fmt.Sprintf("/api/auth/users/%d", id), "")
	return err
}

// clearAccountCookies 用**不带任何 Cookie** 的请求删账号，模拟「未登录点删除」。
//
// 不复用 acctDo：它走的 client 带 jar，场景里已经登录过，抽掉 Cookie 反而要多删状态；
// 这里直接发裸请求，语义就是「门外的人拿 id 来删」，期望 401。
func (w *world) clearAccountCookies() error {
	if err := w.ensureDB(); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodDelete, w.srv.URL+"/api/auth/users/1", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	w.lastCode, w.lastBody = resp.StatusCode, data
	return err
}
