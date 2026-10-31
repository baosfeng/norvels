package store

import (
	"context"
	"testing"
)

// TestCleanRulesEmpty 未配置时返回空切片而不是 nil（api 直接序列化成 {"rules":[]}）。
func TestCleanRulesEmpty(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	rules, err := st.ListCleanRules(ctx)
	if err != nil {
		t.Fatalf("ListCleanRules: %v", err)
	}
	if rules == nil {
		t.Fatal("未配置时应返回空切片而不是 nil")
	}
	if len(rules) != 0 {
		t.Fatalf("期望 0 条，实际 %d", len(rules))
	}
}

// TestCleanRulesRoundTrip 存取往返：所有字段（含负数 order 与布尔位）保真。
func TestCleanRulesRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	saved := []CleanRule{
		{ID: 1, Name: "去广告", Group: "内容", Order: -5, Enabled: true, IsRegex: true,
			Pattern: `广告\d+`, Replacement: "", ScopeTitle: false, ScopeContent: true},
		{ID: 2, Name: "字面量", Group: "格式", Order: 10, Enabled: false, IsRegex: false,
			Pattern: "a.b", Replacement: "X", ScopeTitle: true, ScopeContent: false},
	}
	if err := st.SaveCleanRules(ctx, saved); err != nil {
		t.Fatalf("SaveCleanRules: %v", err)
	}
	got, err := st.ListCleanRules(ctx)
	if err != nil {
		t.Fatalf("ListCleanRules: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("期望 2 条，实际 %d", len(got))
	}
	if got[0] != saved[0] {
		t.Fatalf("第 1 条字段不保真:\n got %+v\nwant %+v", got[0], saved[0])
	}
	if got[1] != saved[1] {
		t.Fatalf("第 2 条字段不保真:\n got %+v\nwant %+v", got[1], saved[1])
	}
}

// TestCleanRulesReplaceAll 全量替换：旧集合被新集合整体覆盖。
func TestCleanRulesReplaceAll(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.SaveCleanRules(ctx, []CleanRule{
		{ID: 1, Name: "旧1", Pattern: "a", Enabled: true},
		{ID: 2, Name: "旧2", Pattern: "b", Enabled: true},
		{ID: 3, Name: "旧3", Pattern: "c", Enabled: true},
	}); err != nil {
		t.Fatalf("SaveCleanRules: %v", err)
	}
	if err := st.SaveCleanRules(ctx, []CleanRule{{ID: 9, Name: "新", Pattern: "z", Enabled: true}}); err != nil {
		t.Fatalf("覆盖 SaveCleanRules: %v", err)
	}
	got, err := st.ListCleanRules(ctx)
	if err != nil {
		t.Fatalf("ListCleanRules: %v", err)
	}
	if len(got) != 1 || got[0].Name != "新" {
		t.Fatalf("全量替换后应只剩新规则，实际 %+v", got)
	}
	// 传 nil 等于清空。
	if err := st.SaveCleanRules(ctx, nil); err != nil {
		t.Fatalf("清空 SaveCleanRules: %v", err)
	}
	got, _ = st.ListCleanRules(ctx)
	if len(got) != 0 {
		t.Fatalf("清空后期望 0 条，实际 %d", len(got))
	}
}

// TestCleanRulesCorruptJSON 坏配置报错而不是静默清空。
func TestCleanRulesCorruptJSON(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.PutSetting(ctx, SettingKeyReplaceRules, "{not-json"); err != nil {
		t.Fatalf("PutSetting: %v", err)
	}
	if _, err := st.ListCleanRules(ctx); err == nil {
		t.Fatal("坏 JSON 应报错，不能静默当成空规则")
	}
}

// TestCleanRulesNotConfigured 空串与 JSON null 都归一成空切片（不是 nil）。
func TestCleanRulesNotConfigured(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{"  ", "null"} {
		t.Run(raw, func(t *testing.T) {
			st := newTestStore(t)
			if err := st.PutSetting(ctx, SettingKeyReplaceRules, raw); err != nil {
				t.Fatalf("PutSetting: %v", err)
			}
			rules, err := st.ListCleanRules(ctx)
			if err != nil || rules == nil || len(rules) != 0 {
				t.Fatalf("%q 应返回空切片，实际 %+v err=%v", raw, rules, err)
			}
		})
	}
}

// TestCleanRulesEncodeBoundaries 编解码边界：条数上限由 api 拦，store 只保证能原样存取。
func TestCleanRulesEncodeBoundaries(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	many := make([]CleanRule, 0, 150)
	for i := 0; i < 150; i++ {
		many = append(many, CleanRule{ID: i + 1, Name: "规则", Pattern: "p", Replacement: "r", Enabled: true})
	}
	if err := st.SaveCleanRules(ctx, many); err != nil {
		t.Fatalf("SaveCleanRules: %v", err)
	}
	got, err := st.ListCleanRules(ctx)
	if err != nil {
		t.Fatalf("ListCleanRules: %v", err)
	}
	if len(got) != 150 {
		t.Fatalf("store 层不拦条数上限，应能存取 150 条，实际 %d", len(got))
	}
	// 中文、正则元字符、空替换串都要原样往返。
	tricky := []CleanRule{{ID: 1, Name: "特殊", Pattern: `^第[一二三]+[章节]$##x`, Replacement: "", Enabled: true}}
	if err := st.SaveCleanRules(ctx, tricky); err != nil {
		t.Fatalf("SaveCleanRules: %v", err)
	}
	got, _ = st.ListCleanRules(ctx)
	if len(got) != 1 || got[0].Pattern != tricky[0].Pattern {
		t.Fatalf("特殊字符往返失败: %+v", got)
	}
}
