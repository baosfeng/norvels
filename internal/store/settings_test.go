package store

import (
	"context"
	"errors"
	"testing"
)

// TestSettingRoundTrip 设置项读写：缺失不报错，覆盖生效。
func TestSettingRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	if _, ok, err := st.GetSetting(ctx, "no_such_key"); err != nil || ok {
		t.Fatalf("缺失键 = ok:%v err:%v，期望 ok=false 且无错误", ok, err)
	}
	if err := st.PutSetting(ctx, "some_key", "v1"); err != nil {
		t.Fatalf("PutSetting: %v", err)
	}
	got, ok, err := st.GetSetting(ctx, "some_key")
	if err != nil || !ok || got != "v1" {
		t.Fatalf("GetSetting = (%q, %v, %v)，期望 (v1, true, nil)", got, ok, err)
	}
	if err := st.PutSetting(ctx, "some_key", "v2"); err != nil {
		t.Fatalf("PutSetting 覆盖: %v", err)
	}
	if got, _, _ := st.GetSetting(ctx, "some_key"); got != "v2" {
		t.Errorf("覆盖后 = %q，期望 v2", got)
	}
	if err := st.PutSetting(ctx, "  ", "x"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空键 err = %v，期望 ErrInvalidArgument", err)
	}
}

// TestChapterRulesRoundTrip 章节规则全量替换与空态。
func TestChapterRulesRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	requireNoChapterRules(t, ctx, st)

	want := []ChapterRule{
		{Name: "分节符", Pattern: "^=== 第\\d+ 节 ===$"},
		{Name: "卷标题", Pattern: "^第[一二三四五六七八九十]+卷"},
	}
	if err := st.SaveChapterRules(ctx, want); err != nil {
		t.Fatalf("SaveChapterRules: %v", err)
	}
	got, err := st.ListChapterRules(ctx)
	if err != nil {
		t.Fatalf("ListChapterRules: %v", err)
	}
	requireSameChapterRules(t, got, want)

	// 全量替换：传空即清空。
	if err := st.SaveChapterRules(ctx, nil); err != nil {
		t.Fatalf("SaveChapterRules(nil): %v", err)
	}
	requireNoChapterRules(t, ctx, st)
}

// TestChapterRulesCorruptValue 库里规则 JSON 坏掉时报错，不静默当成空规则。
func TestChapterRulesCorruptValue(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.PutSetting(ctx, SettingKeyChapterRules, "{not json"); err != nil {
		t.Fatalf("PutSetting: %v", err)
	}
	if _, err := st.ListChapterRules(ctx); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
}

// requireNoChapterRules 断言当前没有任何自定义规则（且不是 nil）。
func requireNoChapterRules(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	rules, err := st.ListChapterRules(ctx)
	if err != nil {
		t.Fatalf("ListChapterRules: %v", err)
	}
	if rules == nil || len(rules) != 0 {
		t.Fatalf("期望空切片（非 nil）: %#v", rules)
	}
}

// requireSameChapterRules 逐条比对规则（顺序即语义）。
func requireSameChapterRules(t *testing.T, got, want []ChapterRule) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("规则数 = %d，期望 %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 条 = %+v，期望 %+v", i, got[i], want[i])
		}
	}
}
