package store

import (
	"context"
	"reflect"
	"testing"

	"norvels/internal/source"
)

// TestSourceProfilesRoundTrip 规则包全量替换与空态：读写形态必须一致（GET 要能原样回放）。
func TestSourceProfilesRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	requireNoSourceProfiles(t, ctx, st)

	want := []source.Profile{{
		ID: "sample-dir", Name: "示例目录", SchemaVersion: 1, Enabled: true, Priority: 10,
		Kind:  source.KindLocal,
		Match: source.Match{Ext: []string{"txt"}, PathGlob: "/books/示例/**", NameRegex: "^示例"},
		Filters: source.Filters{
			Disable: []string{"width"},
			Extra:   []source.FilterRule{{Name: "广告行", Priority: 30, Pattern: "^广告：.*$"}},
		},
		Meta:   []source.MetaRule{{Name: "头部作者", Weight: 60, Pattern: "^作者：(.+)$", Author: 1}},
		Split:  source.Split{Rules: []source.Rule{{Name: "分节符", Pattern: "^=== 第\\d+ 节 ===$"}}},
		Tags:   []source.TagRule{{Tag: "奇幻", Keywords: []string{"魔法"}}},
		Limits: source.Limits{MaxChapterChars: 20000, MinChapters: 1},
	}}
	if err := st.SaveSourceProfiles(ctx, want); err != nil {
		t.Fatalf("SaveSourceProfiles: %v", err)
	}
	got, err := st.ListSourceProfiles(ctx)
	if err != nil {
		t.Fatalf("ListSourceProfiles: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("往返不一致：\n got = %+v\nwant = %+v", got, want)
	}

	// 全量替换：传空即清空。
	if err := st.SaveSourceProfiles(ctx, nil); err != nil {
		t.Fatalf("SaveSourceProfiles(nil): %v", err)
	}
	requireNoSourceProfiles(t, ctx, st)
}

// TestSourceProfilesCorruptValue 库里 JSON 坏掉时报错，不静默当成「没有规则包」。
func TestSourceProfilesCorruptValue(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.PutSetting(ctx, SettingKeySourceProfiles, "{not json"); err != nil {
		t.Fatalf("PutSetting: %v", err)
	}
	if _, err := st.ListSourceProfiles(ctx); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
}

// requireNoSourceProfiles 断言当前没有任何规则包（且返回空切片而非 nil）。
func requireNoSourceProfiles(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	profiles, err := st.ListSourceProfiles(ctx)
	if err != nil {
		t.Fatalf("ListSourceProfiles: %v", err)
	}
	if profiles == nil || len(profiles) != 0 {
		t.Fatalf("期望空切片（非 nil）: %#v", profiles)
	}
}
