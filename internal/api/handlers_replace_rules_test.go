package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"norvels/internal/source"
	"norvels/internal/store"
)

var jsonHdr = map[string]string{"Content-Type": "application/json"}

func TestReplaceRulesCRUD(t *testing.T) {
	env := newEnv(t)
	rr := env.get("/api/settings/replace-rules")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET 状态码 = %d", rr.Code)
	}
	empty := decodeBody[struct {
		Rules []store.CleanRule `json:"rules"`
	}](t, rr)
	if len(empty.Rules) != 0 {
		t.Fatalf("初始应为空，实际 %d 条", len(empty.Rules))
	}

	put := `{"rules":[{"name":"去广告","group":"内容","pattern":"广告","replacement":"","isRegex":false,"enabled":true,"order":10,"scopeContent":true}]}`
	if rr = env.do(http.MethodPut, "/api/settings/replace-rules", strings.NewReader(put), jsonHdr); rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	rules := decodeBody[struct {
		Rules []store.CleanRule `json:"rules"`
	}](t, env.get("/api/settings/replace-rules"))
	if len(rules.Rules) != 1 || rules.Rules[0].Name != "去广告" || rules.Rules[0].ID != 1 {
		t.Fatalf("回读结果不符: %+v", rules.Rules)
	}
}

func TestReplaceRulesValidation(t *testing.T) {
	env := newEnv(t)
	bad := map[string]string{
		"坏正则":  `{"rules":[{"name":"ok","pattern":"a"},{"name":"坏","pattern":"a(","isRegex":true}]}`,
		"空模式":  `{"rules":[{"name":"x","pattern":"  "}]}`,
		"超长模式": `{"rules":[{"name":"x","pattern":"` + strings.Repeat("a", 201) + `"}]}`,
	}
	for name, body := range bad {
		if rr := env.do(http.MethodPut, "/api/settings/replace-rules", strings.NewReader(body), jsonHdr); rr.Code != http.StatusBadRequest {
			t.Fatalf("%s: 状态码 = %d，body=%s", name, rr.Code, rr.Body.String())
		}
	}
	// 坏正则的报错要指出第几条
	rr := env.do(http.MethodPut, "/api/settings/replace-rules", strings.NewReader(`{"rules":[{"pattern":"a"},{"pattern":"a(","isRegex":true}]}`), jsonHdr)
	err := decodeBody[errResp](t, rr)
	if !strings.Contains(err.Error.Message, "第 2 条") {
		t.Fatalf("错误消息应指出第 2 条，实际 %q", err.Error.Message)
	}
}

func TestReplaceRulesPreview(t *testing.T) {
	env := newEnv(t)
	put := `{"rules":[
		{"name":"字面量","pattern":"a.b","replacement":"X","isRegex":false,"enabled":true,"order":1},
		{"name":"正则","pattern":"广告\\d+","replacement":"","isRegex":true,"enabled":true,"order":2},
		{"name":"停用","pattern":"保留","replacement":"删掉","isRegex":false,"enabled":false,"order":3}
	]}`
	if rr := env.do(http.MethodPut, "/api/settings/replace-rules", strings.NewReader(put), jsonHdr); rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 失败: %s", rr.Body.String())
	}
	preview := `{"text":"a.b 与 axb；广告123 保留"}`
	rr := env.do(http.MethodPost, "/api/settings/replace-rules/preview", strings.NewReader(preview), jsonHdr)
	if rr.Code != http.StatusOK {
		t.Fatalf("preview 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Before string `json:"before"`
		After  string `json:"after"`
		Hits   []struct {
			RuleID int    `json:"ruleId"`
			Name   string `json:"name"`
			Count  int    `json:"count"`
		} `json:"hits"`
	}
	out = decodeBody[struct {
		Before string `json:"before"`
		After  string `json:"after"`
		Hits   []struct {
			RuleID int    `json:"ruleId"`
			Name   string `json:"name"`
			Count  int    `json:"count"`
		} `json:"hits"`
	}](t, rr)
	if !strings.Contains(out.After, "X 与 axb") {
		t.Fatalf("IsRegex=false 应只替换字面量 a.b，实际 %q", out.After)
	}
	if strings.Contains(out.After, "广告123") {
		t.Fatalf("正则规则未生效: %q", out.After)
	}
	if strings.Contains(out.After, "删掉") {
		t.Fatalf("停用规则不应生效: %q", out.After)
	}
	if len(out.Hits) != 2 {
		t.Fatalf("hits 应只含启用规则（2 条），实际 %d", len(out.Hits))
	}
}

func TestCleanFiltersOrderAndMerge(t *testing.T) {
	rules := []store.CleanRule{
		{ID: 1, Name: "后", Pattern: "b", Replacement: "B", Enabled: true, Order: 20},
		{ID: 2, Name: "前", Pattern: "a", Replacement: "A", Enabled: true, Order: 10},
		{ID: 3, Name: "停", Pattern: "c", Replacement: "C", Enabled: false, Order: 5},
	}
	filters := cleanFilters(rules)
	if len(filters) != 2 {
		t.Fatalf("停用规则不应编译，实际 %d 条", len(filters))
	}
	if filters[0].Priority() != 10 || filters[1].Priority() != 20 {
		t.Fatalf("应按 order 升序，实际 %d / %d", filters[0].Priority(), filters[1].Priority())
	}
	profiles := []source.Profile{{
		ID:      "p1",
		Filters: source.Filters{Extra: []source.FilterRule{{Name: "自带", Priority: 99, Pattern: "z", Replace: "Z"}}},
	}}
	merged := withCleanRules(profiles, rules)
	if len(merged[0].Filters.Extra) != 3 {
		t.Fatalf("合并后应有 3 条，实际 %d", len(merged[0].Filters.Extra))
	}
	if merged[0].Filters.Extra[0].Name != "前" || merged[0].Filters.Extra[2].Name != "自带" {
		t.Fatalf("全局规则应前置、规则包自带在后: %+v", merged[0].Filters.Extra)
	}
}

func TestReplaceRulesPreviewTooLarge(t *testing.T) {
	env := newEnv(t)
	body := `{"text":"` + strings.Repeat("x", (1<<20)+10) + `"}`
	rr := env.do(http.MethodPost, "/api/settings/replace-rules/preview", strings.NewReader(body), jsonHdr)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("超 1 MiB 应 400，实际 %d", rr.Code)
	}
}

var _ = httptest.NewRecorder
