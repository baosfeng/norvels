package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// 阅读设置（0028）：每账号一行 JSON，读写都按账号隔离。
//
// 口径：
//   - 从没写过的账号读到 ok=false（不是错误）——前端据此回落到本机 localStorage；
//   - 写入是 upsert（同一个账号再写只更新那一行）；
//   - 值必须是合法 JSON，空值与不可序列化的值一律拒绝，绝不落半截数据。

const readerSettingsSample = `{"theme":"sepia","fontSize":18}`

// TestReaderSettingsRoundTrip 空态 / 写入 / 覆盖，以及四个读别名给出同一份值。
func TestReaderSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	uid := mustRegister(t, st, "读者甲").ID
	scoped := WithUserID(ctx, uid)

	if raw, ok, err := st.GetReaderSettings(scoped); err != nil || ok || raw != "" {
		t.Fatalf("从未写过 = (%q, %v, %v)，期望空值且 ok=false", raw, ok, err)
	}
	requireOK(t, "PutReaderSettings", st.PutReaderSettings(scoped, readerSettingsSample))
	got, ok, err := st.GetReaderSettings(scoped)
	requireOK(t, "GetReaderSettings", err)
	if !ok || got != readerSettingsSample {
		t.Fatalf("回读 = (%q, %v)，期望 (%q, true)", got, ok, readerSettingsSample)
	}

	// 覆盖：同一账号只更新那一行，不新增。
	requireOK(t, "PutReaderSettings 覆盖", st.PutReaderSettings(scoped, `{"theme":"dark"}`))
	requireEqual(t, "覆盖后的值", mustReaderSettings(t, st, scoped), `{"theme":"dark"}`)
	requireEqual(t, "行数", readerSettingsRows(t, ctx, st, uid), 1)
}

// TestReaderSettingsAliases 四个读别名与六个写别名都落到同一行同一份值。
func TestReaderSettingsAliases(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	uid := mustRegister(t, st, "读者甲").ID
	scoped := WithUserID(ctx, uid)

	requireOK(t, "PutReaderSettingsJSON", st.PutReaderSettingsJSON(scoped, readerSettingsSample))
	reads := map[string]func() (string, bool, error){
		"GetReaderSettingsJSON": func() (string, bool, error) {
			raw, ok, err := st.GetReaderSettingsJSON(scoped)
			return string(raw), ok, err
		},
		"GetUserReaderSettings": func() (string, bool, error) { return st.GetUserReaderSettings(scoped) },
		"FetchReaderSettings":   func() (string, bool, error) { return st.FetchReaderSettings(scoped) },
		"LoadReaderSettings":    func() (string, bool, error) { return st.LoadReaderSettings(scoped) },
	}
	for name, read := range reads {
		raw, ok, err := read()
		requireOK(t, name, err)
		if !ok || raw != readerSettingsSample {
			t.Errorf("%s = (%q, %v)，期望 (%q, true)", name, raw, ok, readerSettingsSample)
		}
	}

	// 用有序切片而不是 map：断言「最后一个写别名生效」依赖执行顺序，map 迭代顺序随机。
	writes := []struct {
		name  string
		write func() error
	}{
		{"SaveReaderSettings", func() error { return st.SaveReaderSettings(scoped, `{"fontSize":20}`) }},
		{"SetReaderSettings", func() error { return st.SetReaderSettings(scoped, `{"fontSize":21}`) }},
		{"UpdateReaderSettings", func() error { return st.UpdateReaderSettings(scoped, `{"fontSize":22}`) }},
		{"UpsertReaderSettings", func() error { return st.UpsertReaderSettings(scoped, `{"fontSize":23}`) }},
	}
	for _, w := range writes {
		requireOK(t, w.name, w.write())
	}
	requireEqual(t, "最后一个写别名生效", mustReaderSettings(t, st, scoped), `{"fontSize":23}`)
}

// TestReaderSettingsPerUser 按账号隔离：显式 id 优先于 context，两账号互不可见。
func TestReaderSettingsPerUser(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	one := mustRegister(t, st, "读者甲").ID
	two := mustRegister(t, st, "读者乙").ID

	requireOK(t, "甲写", st.PutReaderSettingsForUser(ctx, one, `{"theme":"sepia"}`))
	requireOK(t, "乙写", st.PutReaderSettingsForUser(ctx, two, `{"theme":"dark"}`))
	// context 指向甲、显式传乙：显式 id 优先（否则同步会把别人的设置写到当前账号）。
	requireOK(t, "显式 id 优先", st.PutReaderSettings(WithUserID(ctx, one), `{"theme":"night"}`, two))
	requireEqual(t, "乙的值", mustReaderSettings(t, st, WithUserID(ctx, two)), `{"theme":"night"}`)
	requireEqual(t, "甲没被写坏", mustReaderSettings(t, st, WithUserID(ctx, one)), `{"theme":"sepia"}`)

	raw, ok, err := st.GetReaderSettingsForUser(ctx, one)
	requireOK(t, "GetReaderSettingsForUser", err)
	if !ok || raw != `{"theme":"sepia"}` {
		t.Errorf("甲的设置 = (%q, %v)", raw, ok)
	}
	requireOK(t, "SaveReaderSettingsForUser", st.SaveReaderSettingsForUser(ctx, one, `{"theme":"grey"}`))
	requireEqual(t, "甲改写后", mustReaderSettings(t, st, WithUserID(ctx, one)), `{"theme":"grey"}`)
	requireEqual(t, "两个账号两行", readerSettingsRows(t, ctx, st, one)+readerSettingsRows(t, ctx, st, two), 2)
}

// TestReaderSettingsPayload 入参归一：接受的五种形态都落成原文；非法值一律 ErrInvalidArgument。
func TestReaderSettingsPayload(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		want    string
		invalid bool
		wantErr bool
	}{
		{"字符串", readerSettingsSample, readerSettingsSample, false, false},
		{"字节切片", []byte(readerSettingsSample), readerSettingsSample, false, false},
		{"RawMessage", json.RawMessage(readerSettingsSample), readerSettingsSample, false, false},
		// 结构化入参按 json.Marshal 输出（map 键排字母序），不保证与样例同序。
		{"映射", map[string]any{"theme": "sepia", "fontSize": 18},
			`{"fontSize":18,"theme":"sepia"}`, false, false},
		{"首尾空白", "  " + readerSettingsSample + "\n", readerSettingsSample, false, false},
		{"nil", nil, "", true, false},
		{"空白串", "   ", "", true, false},
		{"非法 JSON", "{not json", "", true, false},
		{"不可序列化", make(chan int), "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readerSettingsPayload(tc.value)
			if tc.invalid {
				if !errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("err = %v，期望 ErrInvalidArgument", err)
				}
				return
			}
			if tc.wantErr {
				if err == nil || errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("err = %v，期望编码错误而非参数错误", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("payload: %v", err)
			}
			if got != tc.want {
				t.Errorf("payload = %q，期望 %q", got, tc.want)
			}
		})
	}
}

// TestReaderSettingsClosedDB 库关掉后读写都报错（错误映射到 5xx 的源头）。
func TestReaderSettingsClosedDB(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	requireOK(t, "PutReaderSettings", st.PutReaderSettings(ctx, readerSettingsSample))
	requireOK(t, "Close", st.Close())

	if _, _, err := st.GetReaderSettings(ctx); err == nil {
		t.Error("关库后读应报错")
	}
	if _, _, err := st.GetReaderSettingsJSON(ctx); err == nil {
		t.Error("关库后按 JSON 读应报错（别名不能吞错）")
	}
	if err := st.PutReaderSettings(ctx, readerSettingsSample); err == nil {
		t.Error("关库后写应报错")
	}
}

// TestReaderSettingsRejectsNil 写入 nil 走参数错误（不是 500，也不是静默忽略）。
//
// 调用方把「没有设置」当成 nil 传进来时，必须明确拒绝：静默写入空值会让前端
// 读回一份空字符串，接着整片界面按默认值重绘——用户看起来就是「设置丢了」。
func TestReaderSettingsRejectsNil(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.PutReaderSettings(ctx, nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("PutReaderSettings(nil) = %v，期望 ErrInvalidArgument", err)
	}
	if _, ok, err := st.GetReaderSettings(ctx); err != nil || ok {
		t.Errorf("被拒的写入不该落库：ok=%v err=%v", ok, err)
	}
}

// mustReaderSettings 读当前账号的设置原文（失败即 Fatal）。
func mustReaderSettings(t *testing.T, st *Store, ctx context.Context) string {
	t.Helper()
	raw, ok, err := st.GetReaderSettings(ctx)
	requireOK(t, "GetReaderSettings", err)
	if !ok {
		t.Fatal("期望已写入阅读设置")
	}
	return raw
}

// readerSettingsRows 数某账号的阅读设置行数。
func readerSettingsRows(t *testing.T, ctx context.Context, st *Store, uid int64) int {
	t.Helper()
	var n int
	requireOK(t, "数阅读设置行", st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_reader_settings WHERE user_id = ?", uid).Scan(&n))
	return n
}
