package api

// 净化规则（replace-rules）与导入任务收尾的边界分支。
// 这组用例守的是「坏规则只跳过、不整批失败」与「任务终态一定会写上」两条约定。

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"norvels/internal/source"
	"norvels/internal/store"
)

// TestPutReplaceRulesValidation 三类非法载荷：条数超限、空模式、正则非法。
func TestPutReplaceRulesValidation(t *testing.T) {
	e := newEnv(t)
	tooMany := make([]string, 0, maxCleanRules+1)
	for i := 0; i <= maxCleanRules; i++ {
		tooMany = append(tooMany, `{"name":"r","pattern":"广告"}`)
	}
	cases := []struct{ name, body, wantCode string }{
		{"条数超限", `{"rules":[` + strings.Join(tooMany, ",") + `]}`, codeInvalidParam},
		{"空模式", `{"rules":[{"name":"r","pattern":"  "}]}`, codeInvalidParam},
		{"超长模式", `{"rules":[{"name":"r","pattern":"` + strings.Repeat("长", maxCleanPatternRunes+1) + `"}]}`, codeInvalidParam},
		{"正则非法", `{"rules":[{"name":"r","pattern":"(","isRegex":true}]}`, codeInvalidPattern},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := e.do(http.MethodPut, "/api/settings/replace-rules", strings.NewReader(c.body),
				map[string]string{"Content-Type": "application/json"})
			requireError(t, rr, http.StatusBadRequest, c.wantCode)
		})
	}
}

// TestPutReplaceRulesAssignsIDAndName ID 与名字缺省时补全，字面量模式自动转义。
func TestPutReplaceRulesAssignsIDAndName(t *testing.T) {
	e := newEnv(t)
	body := `{"rules":[{"pattern":"广告"},{"name":"` + strings.Repeat("名", maxCleanNameRunes+3) + `","pattern":"水印","isRegex":false}]}`
	if rr := e.do(http.MethodPut, "/api/settings/replace-rules", strings.NewReader(body),
		map[string]string{"Content-Type": "application/json"}); rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[cleanRulesBody](t, e.get("/api/settings/replace-rules"))
	if len(got.Rules) != 2 {
		t.Fatalf("规则条数 = %d，期望 2", len(got.Rules))
	}
	if got.Rules[0].Name != "规则1" || got.Rules[0].ID != 1 {
		t.Errorf("第一条 = %+v，期望补名「规则1」且 id=1", got.Rules[0])
	}
	if n := len([]rune(got.Rules[1].Name)); n != maxCleanNameRunes {
		t.Errorf("超长名截断后 = %d 字，期望 %d", n, maxCleanNameRunes)
	}
}

// TestCleanPatternLiteralEscaped 非正则规则按字面量处理：. 不该被当通配符。
func TestCleanPatternLiteralEscaped(t *testing.T) {
	re, repl, err := cleanPattern(store.CleanRule{Pattern: "a.b", Replacement: "x", IsRegex: false})
	if err != nil {
		t.Fatalf("cleanPattern: %v", err)
	}
	if re.MatchString("axb") {
		t.Error("字面量模式不该匹配 axb（. 必须被转义）")
	}
	if !re.MatchString("a.b") || repl != "x" {
		t.Errorf("字面量模式应匹配 a.b，replace=%q", repl)
	}
}

// TestCleanFiltersSkipsBadRules 未启用的规则不进结果；编译不过的启用规则被跳过。
func TestCleanFiltersSkipsBadRules(t *testing.T) {
	rules := []store.CleanRule{
		{Name: "启用", Pattern: "广告", Enabled: true, Order: 2},
		{Name: "未启用", Pattern: "水印", Enabled: false},
		{Name: "坏的", Pattern: "(", IsRegex: true, Enabled: true, Order: 1},
	}
	filters := cleanFilters(rules)
	if len(filters) != 1 {
		t.Fatalf("过滤器数 = %d，期望只剩 1 条（启用且可编译）", len(filters))
	}
	if len(withCleanRules(nil, nil)) != 0 {
		t.Error("没有启用规则时 withCleanRules 不该凭空造出规则包")
	}
}

// TestWithCleanRulesPrepends 启用规则前置进每份规则包，且保持全局在前。
func TestWithCleanRulesPrepends(t *testing.T) {
	rules := []store.CleanRule{
		{Name: "全局", Pattern: "广告", Enabled: true, Order: 2, Replacement: ""},
		{Name: "未启用", Pattern: "水印", Enabled: false},
	}
	profiles := []source.Profile{{
		ID:      "p1",
		Filters: source.Filters{Extra: []source.FilterRule{{Name: "自带", Pattern: "^x$"}}},
	}}
	got := withCleanRules(profiles, rules)
	if len(got) != 1 {
		t.Fatalf("规则包数 = %d，期望 1", len(got))
	}
	extra := got[0].Filters.Extra
	if len(extra) != 2 {
		t.Fatalf("extra 条数 = %d，期望 2（全局 1 + 自带 1）", len(extra))
	}
	if extra[0].Name != "全局" || extra[1].Name != "自带" {
		t.Errorf("顺序 = %q,%q，期望全局规则在前", extra[0].Name, extra[1].Name)
	}
	// 没有启用规则时原样返回，不复制、不改变顺序。
	untouched := []source.Profile{{ID: "p2"}}
	if out := withCleanRules(untouched, rules[1:]); len(out) != 1 || out[0].ID != "p2" {
		t.Errorf("无启用规则时应原样返回，实际 %+v", out)
	}
}

// TestFinalizeImportJobTerminal 任务已经终态时收尾是空操作（不覆盖既有终态）。
func TestFinalizeImportJobTerminal(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	job, err := e.st.CreateImportJob(ctx, "/tmp/none")
	requireNoError(t, err, "建任务")
	requireNoError(t, e.srv.finalizeImportJob(job.ID, "failed", "第一次收尾", nil), "首次收尾")
	requireNoError(t, e.srv.finalizeImportJob(job.ID, "completed", "第二次收尾", nil), "重复收尾")

	detail, err := e.st.GetImportJob(ctx, job.ID)
	requireNoError(t, err, "读任务")
	if detail.Status != "failed" {
		t.Errorf("重复收尾后状态 = %q，期望保持第一次的 failed", detail.Status)
	}
	if detail.Message != "第一次收尾" {
		t.Errorf("消息 = %q，期望不被第二次覆盖", detail.Message)
	}
}

// TestFinalizeImportJobMissing 任务不存在时返回错误（调用方据此记日志，而不是假装成功）。
func TestFinalizeImportJobMissing(t *testing.T) {
	e := newEnv(t)
	if err := e.srv.finalizeImportJob(999999, "failed", "", nil); err == nil {
		t.Error("不存在的任务应当报错")
	}
}
