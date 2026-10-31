package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// 账号与会话的口径（迁移 0016）：
//   - 账号名去首尾空白后 1–32 字，重名 409（ErrConflict）；
//   - 登录时账号不存在是 ErrNotFound，不是并发冲突；
//   - 会话 token 不可猜、库里只存哈希、过期即失效、登出即撤销。

func TestRegisterUserValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	cases := []struct {
		name string
		want error
	}{
		{"", ErrInvalidArgument},
		{"   ", ErrInvalidArgument},
		{"\t\n ", ErrInvalidArgument},
		{repeatRune('长', maxUserNameRunes+1), ErrInvalidArgument},
	}
	for _, c := range cases {
		if _, err := st.RegisterUser(ctx, c.name); !errors.Is(err, c.want) {
			t.Errorf("RegisterUser(%q) = %v，期望 %v", c.name, err, c.want)
		}
	}
}

// TestRegisterUserTrimsAndConflicts 校验去空白后建号，以及重名 409。
//
// 「 reader」与「reader」必须撞车：唯一索引按 TRIM 建，建号也必须按 TRIM 判重，
// 否则用户会有两个只差空格的账号，自己都分不清在登录哪一个。
// 大小写同理（COLLATE NOCASE）：Alice 与 alice 必须撞车，见 TestRegisterUserCaseInsensitive。
func TestRegisterUserTrimsAndConflicts(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	u, err := st.RegisterUser(ctx, "  读者甲  ")
	requireOK(t, "RegisterUser", err)
	requireEqual(t, "账号名（去空白）", u.Name, "读者甲")
	if u.ID <= 0 {
		t.Errorf("新账号 id = %d，期望正数", u.ID)
	}
	if _, err := st.RegisterUser(ctx, "读者甲"); !errors.Is(err, ErrConflict) {
		t.Errorf("重名注册 = %v，期望 ErrConflict", err)
	}
	if _, err := st.RegisterUser(ctx, "  读者甲 "); !errors.Is(err, ErrConflict) {
		t.Errorf("带空白的重名注册 = %v，期望 ErrConflict", err)
	}
}

// TestRegisterUserCaseInsensitive 大小写折叠后判重，且登录能命中已建账号。
//
// 唯一索引是 (TRIM(name) COLLATE NOCASE)，建号与登录的判重必须同一口径：
// 只对着索引改一处，另一处就会出现「注册说重名、登录说不存在」的鬼故事。
func TestRegisterUserCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	created, err := st.RegisterUser(ctx, "Alice")
	requireOK(t, "RegisterUser(Alice)", err)
	// 存储保留用户输入的大小写，只折叠比较。
	requireEqual(t, "落库的大小写", created.Name, "Alice")

	if _, err := st.RegisterUser(ctx, "alice"); !errors.Is(err, ErrConflict) {
		t.Errorf("小写重名注册 = %v，期望 ErrConflict", err)
	}
	if _, err := st.RegisterUser(ctx, " alice "); !errors.Is(err, ErrConflict) {
		t.Errorf("带空白的大小写重名注册 = %v，期望 ErrConflict", err)
	}
	logged, err := st.LoginUser(ctx, "ALICE")
	requireOK(t, "LoginUser(ALICE)", err)
	requireEqual(t, "登录命中的 id", logged.ID, created.ID)
}

// TestRegisterUserMaxLength 恰好 32 字可以建号（边界内侧）。
func TestRegisterUserMaxLength(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	name := repeatRune('名', maxUserNameRunes)
	u, err := st.RegisterUser(ctx, name)
	requireOK(t, "RegisterUser(32 字)", err)
	requireEqual(t, "32 字账号名", u.Name, name)
}

// TestLoginUserUpdatesLastSeen 登录会刷新 last_seen_at；账号不存在是 ErrNotFound。
func TestLoginUserUpdatesLastSeen(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	created, err := st.RegisterUser(ctx, "读者甲")
	requireOK(t, "RegisterUser", err)
	requireEqual(t, "建号后 last_seen_at", created.LastSeenAt, "")

	logged, err := st.LoginUser(ctx, " 读者甲 ")
	requireOK(t, "LoginUser", err)
	requireEqual(t, "登录返回的 id", logged.ID, created.ID)
	if logged.LastSeenAt == "" {
		t.Error("登录后 last_seen_at 仍为空")
	}
	// 读回来也要有值：不能只改内存里的返回结构。
	again, err := st.UserByID(ctx, created.ID)
	requireOK(t, "UserByID", err)
	requireEqual(t, "落库的 last_seen_at", again.LastSeenAt, logged.LastSeenAt)

	if _, err := st.LoginUser(ctx, "查无此人"); !errors.Is(err, ErrNotFound) {
		t.Errorf("登录不存在的账号 = %v，期望 ErrNotFound", err)
	}
}

// TestUserSessionLifecycle 建会话 → 用 token 换账号 → 登出后失效。
func TestUserSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	u, err := st.RegisterUser(ctx, "读者甲")
	requireOK(t, "RegisterUser", err)
	token, err := st.CreateUserSession(ctx, u.ID)
	requireOK(t, "CreateUserSession", err)
	if len(token) != sessionTokenBytes*2 {
		t.Errorf("token 长度 = %d，期望 %d（32 字节 hex）", len(token), sessionTokenBytes*2)
	}
	// 库里存的不是明文：拿到库的人不能直接拿去冒充登录。
	var stored string
	requireOK(t, "读会话行", st.DB().QueryRowContext(ctx,
		"SELECT token_hash FROM user_sessions WHERE user_id = ?", u.ID).Scan(&stored))
	if stored == token {
		t.Error("库里存了明文 token")
	}
	requireEqual(t, "库里存的是 sha256", stored, hashSessionToken(token))

	got, err := st.UserBySessionToken(ctx, token)
	requireOK(t, "UserBySessionToken", err)
	requireEqual(t, "会话换出的账号", got.ID, u.ID)

	requireOK(t, "DeleteUserSession", st.DeleteUserSession(ctx, token))
	if _, err := st.UserBySessionToken(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("登出后换账号 = %v，期望 ErrNotFound", err)
	}
	// 二次登出不报错（幂等）。
	requireOK(t, "DeleteUserSession(重复)", st.DeleteUserSession(ctx, token))
}

// TestUserSessionRejectsUnknownAndExpired 空 token、乱 token、过期 token 都按不存在处理。
func TestUserSessionRejectsUnknownAndExpired(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	u, err := st.RegisterUser(ctx, "读者甲")
	requireOK(t, "RegisterUser", err)
	token, err := st.CreateUserSession(ctx, u.ID)
	requireOK(t, "CreateUserSession", err)

	for _, tok := range []string{"", "   ", "not-a-real-token"} {
		if _, err := st.UserBySessionToken(ctx, tok); !errors.Is(err, ErrNotFound) {
			t.Errorf("UserBySessionToken(%q) = %v，期望 ErrNotFound", tok, err)
		}
	}
	// 手工把过期时间推到过去：读会话必须认过期，而不是只看行在不在。
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE user_sessions SET expires_at = ? WHERE user_id = ?",
		formatTime(nowUTC().Add(-time.Hour)), u.ID); err != nil {
		t.Fatalf("改过期时间: %v", err)
	}
	if _, err := st.UserBySessionToken(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("过期会话 = %v，期望 ErrNotFound", err)
	}
}

// TestDefaultUserIDIsStable 默认账号 id 是外部契约的一部分（开放模式兜底靠它）。
func TestDefaultUserIDIsStable(t *testing.T) {
	requireEqual(t, "DefaultUserID", DefaultUserID(), int64(1))
}

// TestScopeUserIDFallsBackToDefault 没有注入账号时回落默认账号，不 panic 也不返回 0。
func TestScopeUserIDFallsBackToDefault(t *testing.T) {
	requireEqual(t, "未注入", ScopeUserID(context.Background()), defaultUserID)
	requireEqual(t, "显式注入", ScopeUserID(WithUserID(context.Background(), 7)), int64(7))
	requireEqual(t, "注入非法值", ScopeUserID(WithUserID(context.Background(), 0)), defaultUserID)
}

// TestListUsersOrdersByLastSeenWithNullLast 账号列表：最近活跃在前，从未登录的排最后。
//
// NULL 在 SQLite 的 DESC 里反而排最前，最容易写错的就是这一条——所以用例里既有
// 登录过的账号，也有建号后从未登录的（含迁移建出的默认账号 reader）。
func TestListUsersOrdersByLastSeenWithNullLast(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	// 迁移后库里只有默认账号 reader，且从未登录过。
	users, total, err := st.ListUsers(ctx)
	requireOK(t, "ListUsers(仅默认账号)", err)
	requireEqual(t, "仅默认账号时的条数", len(users), 1)
	requireEqual(t, "仅默认账号时的 total", total, 1)
	requireEqual(t, "默认账号名", users[0].Name, defaultUserName)
	requireEqual(t, "默认账号的 last_seen_at", users[0].LastSeenAt, "")

	if _, err := st.RegisterUser(ctx, "从未登录"); err != nil {
		t.Fatalf("RegisterUser(从未登录): %v", err)
	}
	earlier, err := st.RegisterUser(ctx, "早活跃")
	requireOK(t, "RegisterUser(早活跃)", err)
	later, err := st.RegisterUser(ctx, "新活跃")
	requireOK(t, "RegisterUser(新活跃)", err)
	setUserLastSeen(t, st, earlier.ID, "2026-10-30T05:26:24.041Z")
	setUserLastSeen(t, st, later.ID, "2026-10-31T05:26:24.041Z")

	users, total, err = st.ListUsers(ctx)
	requireOK(t, "ListUsers", err)
	requireEqual(t, "账号总数", total, 4)
	want := []string{"新活跃", "早活跃", "从未登录", defaultUserName}
	requireEqual(t, "条数", len(users), len(want))
	for i, name := range want {
		requireEqual(t, fmt.Sprintf("第 %d 条账号名", i+1), users[i].Name, name)
	}
}

// TestListUsersSameTimeTieBreaksByIDDesc 同一时刻的多个账号按 id 倒序（后建的在前）。
func TestListUsersSameTimeTieBreaksByIDDesc(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first, err := st.RegisterUser(ctx, "先建")
	requireOK(t, "RegisterUser(先建)", err)
	second, err := st.RegisterUser(ctx, "后建")
	requireOK(t, "RegisterUser(后建)", err)
	setUserLastSeen(t, st, first.ID, "2026-10-31T05:26:24.041Z")
	setUserLastSeen(t, st, second.ID, "2026-10-31T05:26:24.041Z")

	users, _, err := st.ListUsers(ctx)
	requireOK(t, "ListUsers", err)
	requireEqual(t, "条数", len(users), 3)
	requireEqual(t, "首条是后建的", users[0].ID, second.ID)
	requireEqual(t, "次条是先建的", users[1].ID, first.ID)
	requireEqual(t, "末条是默认账号", users[2].Name, defaultUserName)
}

// TestListUsersCapsAtLimit 超过上限只返回 50 条，total 仍是账号总数。
func TestListUsersCapsAtLimit(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	const extra = 60
	var lastID int64
	for i := 0; i < extra; i++ {
		u, err := st.RegisterUser(ctx, fmt.Sprintf("账号%02d", i))
		requireOK(t, "RegisterUser", err)
		lastID = u.ID
	}
	users, total, err := st.ListUsers(ctx)
	requireOK(t, "ListUsers", err)
	requireEqual(t, "封顶后的条数", len(users), maxListUsers)
	requireEqual(t, "total 是账号总数", total, extra+1)
	// 全都是从未登录的账号：按 id 倒序，首条是最后建的，末条是第 11 个建的。
	requireEqual(t, "首条 id", users[0].ID, lastID)
	requireEqual(t, "末条 id", users[maxListUsers-1].ID, lastID-int64(maxListUsers-1))
}

// setUserLastSeen 直接改 last_seen_at：排序断言要可控的时间，不能指望 nowUTC 的毫秒。
func setUserLastSeen(t *testing.T, st *Store, id int64, at string) {
	t.Helper()
	if _, err := st.DB().ExecContext(context.Background(),
		"UPDATE users SET last_seen_at = ? WHERE id = ?", at, id); err != nil {
		t.Fatalf("改账号 %d 的 last_seen_at: %v", id, err)
	}
}

// repeatRune 生成 n 个相同字符（账号名长度边界用）。
func repeatRune(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}
