package source

import (
	"reflect"
	"strings"
	"testing"
)

// TestParseURLOptions 覆盖无选项块、charset、多键、畸形 JSON 与普通逗号。
func TestParseURLOptions(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantURL string
		charset string
		ignored []string
		warning bool
	}{
		{"无选项块", "https://example.invalid/search?q={{key}}",
			"https://example.invalid/search?q={{key}}", "", nil, false},
		{"charset", `https://example.invalid/search?q={{key}},{"charset":"gbk"}`,
			"https://example.invalid/search?q={{key}}", "gbk", nil, false},
		{"charset 大小写与空格", `https://example.invalid/search?q=x,{"Charset" : "GBK"}`,
			"https://example.invalid/search?q=x", "gbk", nil, false},
		{"多键 + 未实现项", `https://example.invalid/s?q=x,{"webView":true,"charset":"gb18030","headers":{}}`,
			"https://example.invalid/s?q=x", "gb18030", []string{"headers", "webView"}, false},
		{"空 charset 记忽略", `https://example.invalid/s?q=x,{"charset":""}`,
			"https://example.invalid/s?q=x", "", []string{"charset"}, false},
		{"畸形 JSON", `https://example.invalid/s?q=x,{"charset":gbk}`,
			`https://example.invalid/s?q=x,{"charset":gbk}`, "", nil, true},
		{"URL 自带逗号", "https://example.invalid/a,b?q=x", "https://example.invalid/a,b?q=x", "", nil, false},
		{"空串", "  ", "", "", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotURL, opts := ParseURLOptions(c.raw)
			if gotURL != c.wantURL {
				t.Fatalf("URL = %q，期望 %q", gotURL, c.wantURL)
			}
			if opts.Charset != c.charset {
				t.Errorf("charset = %q，期望 %q", opts.Charset, c.charset)
			}
			if !reflect.DeepEqual(opts.Ignored, c.ignored) {
				t.Errorf("ignored = %v，期望 %v", opts.Ignored, c.ignored)
			}
			if (opts.Warning != "") != c.warning {
				t.Errorf("warning = %q，期望有降级=%v", opts.Warning, c.warning)
			}
		})
	}
}

// TestParseURLOptionsKeepsQuery 选项块剥离后查询串必须完整保留。
func TestParseURLOptionsKeepsQuery(t *testing.T) {
	raw := `https://example.invalid/search?q={{key}}&page={{page}},{"charset":"gbk"}`
	got, _ := ParseURLOptions(raw)
	if !strings.Contains(got, "q={{key}}&page={{page}}") {
		t.Fatalf("查询串被破坏：%q", got)
	}
}
