package booksource

import (
	"encoding/json"
	"strings"
	"testing"
)

// legadoSourceJSON 用 map 构造书源 JSON，避免在测试里维护大段转义字符串。
func legadoSourceJSON(t *testing.T, patch map[string]any) []byte {
	t.Helper()
	src := map[string]any{
		"bookSourceUrl":  "https://example.invalid/shorthand",
		"bookSourceName": "示例简写源",
		"bookSourceType": 0,
		"searchUrl":      "https://example.invalid/search?q={{key}}",
	}
	for k, v := range patch {
		src[k] = v
	}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("构造书源 JSON: %v", err)
	}
	return raw
}

// TestParseLegadoConvertsShorthand 导入时把 Legado 简写翻译成标准 CSS 落库。
func TestParseLegadoConvertsShorthand(t *testing.T) {
	raw := legadoSourceJSON(t, map[string]any{
		"ruleSearch": map[string]string{
			"bookList": "class.grid@tr!0",
			"name":     "tag.td.0@tag.a@text",
			"author":   "tag.td.1@text",
			"bookUrl":  "tag.td.0@tag.a@href",
		},
		"ruleBookInfo": map[string]string{"name": "id.book-info@tag.h1@text"},
		"ruleToc":      map[string]string{"chapterList": "class.chapter-list@tag.li"},
		"ruleContent":  map[string]string{"content": "id.content@tag.p@html"},
	})
	profiles, issues, err := ParseLegado(raw)
	if err != nil {
		t.Fatalf("ParseLegado: %v", err)
	}
	if len(profiles) != 1 || len(issues) != 1 {
		t.Fatalf("profiles/issues = %d/%d，期望 1/1", len(profiles), len(issues))
	}
	if issues[0].Status != StatusOK || len(issues[0].DegradedFields) != 0 {
		t.Fatalf("status/degraded = %q/%v，期望 ok/空", issues[0].Status, issues[0].DegradedFields)
	}
	online := profiles[0].Online
	requireRules(t, "ruleSearch", online.RuleSearch, map[string]string{
		"bookList": ".grid tr:not(:nth-child(1))",
		"name":     "td a@text",
		"author":   "td:nth-child(2)@text",
		"bookUrl":  "td a@href",
	})
	requireRule(t, "ruleBookInfo.name", online.RuleBookInfo["name"], "#book-info h1@text")
	requireRule(t, "ruleToc.chapterList", online.RuleToc["chapterList"], ".chapter-list li")
	requireRule(t, "ruleContent.content", online.RuleContent["content"], "#content p@html")
}

// TestParseLegadoSanitizesHeader 请求头里的 CR/LF 值与 Host 等键在导入时被丢弃，
// 安全请求头原样保留，丢弃项在结果里可见。
func TestParseLegadoSanitizesHeader(t *testing.T) {
	raw := legadoSourceJSON(t, map[string]any{
		"ruleSearch": map[string]string{"bookList": ".item"},
		"header": map[string]string{
			"Referer":        "http://ref.example/",
			"User-Agent":     "示例UA",
			"Host":           "evil.example",
			"Content-Length": "999",
			"X-Bad":          "a\r\nInjected: 1",
		},
	})
	profiles, issues, err := ParseLegado(raw)
	if err != nil {
		t.Fatalf("ParseLegado: %v", err)
	}
	if len(profiles) != 1 || len(issues) != 1 {
		t.Fatalf("profiles/issues = %d/%d，期望 1/1", len(profiles), len(issues))
	}
	header := profiles[0].Online.Header
	if len(header) != 2 || header["Referer"] != "http://ref.example/" || header["User-Agent"] != "示例UA" {
		t.Errorf("安全请求头应保留、不安全项应丢弃，实际 %v", header)
	}
	if issues[0].Status != StatusPartial || !strings.Contains(issues[0].Reason, "已丢弃") {
		t.Errorf("status/reason = %q/%q，期望 partial 且说明丢弃", issues[0].Status, issues[0].Reason)
	}
	if len(issues[0].UnsupportedFields) != 0 {
		t.Errorf("header 已生效，不应再进 unsupportedFields：%v", issues[0].UnsupportedFields)
	}
}

// requireRules 逐个字段断言转换结果。
func requireRules(t *testing.T, block string, got map[string]string, want map[string]string) {
	t.Helper()
	for key, value := range want {
		requireRule(t, block+"."+key, got[key], value)
	}
}

// requireRule 断言单条规则等于期望值。
func requireRule(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q，期望 %q", name, got, want)
	}
}

// TestParseLegadoShorthandDegraded 转换不了的规则保持原文、进降级字段并让状态变 partial，
// 同一书源的其它规则照常转换，整源仍可入库。
func TestParseLegadoShorthandDegraded(t *testing.T) {
	const inversion = "-tag.dd@tag.a@text"
	raw := legadoSourceJSON(t, map[string]any{
		"ruleSearch": map[string]string{"bookList": inversion, "name": "tag.td.0@tag.a@text"},
		"ruleToc":    map[string]string{"chapterList": ".chapter"},
	})
	profiles, issues, err := ParseLegado(raw)
	if err != nil {
		t.Fatalf("ParseLegado: %v", err)
	}
	if len(profiles) != 1 || len(issues) != 1 {
		t.Fatalf("profiles/issues = %d/%d，期望 1/1", len(profiles), len(issues))
	}
	requireDegradedIssue(t, issues[0])
	if got := profiles[0].Online.RuleSearch["bookList"]; got != inversion {
		t.Errorf("降级字段应保持原文，实际 %q", got)
	}
	requireRule(t, "ruleSearch.name", profiles[0].Online.RuleSearch["name"], "td a@text")
}

// requireDegradedIssue 断言转换降级后的状态、字段与原因。
func requireDegradedIssue(t *testing.T, issue ImportIssue) {
	t.Helper()
	if issue.Status != StatusPartial {
		t.Fatalf("status = %q，期望 partial", issue.Status)
	}
	if !contains(issue.DegradedFields, "ruleSearch.bookList") {
		t.Errorf("degradedFields = %v，缺 ruleSearch.bookList", issue.DegradedFields)
	}
	if !strings.Contains(issue.Reason, "已保留原文") {
		t.Errorf("reason = %q，期望提到已保留原文", issue.Reason)
	}
}
