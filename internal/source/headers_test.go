package source

import (
	"reflect"
	"testing"
)

// TestSanitizeHeaders 覆盖 CR/LF 注入、Host/Content-Length 等禁用键与合法头保留。
func TestSanitizeHeaders(t *testing.T) {
	in := map[string]string{
		"User-Agent":        "示例UA",
		"Referer":           "http://ref.example/",
		"Host":              "evil.example",
		"Content-Length":    "999",
		"Transfer-Encoding": "chunked",
		"X-Bad":             "a\r\nInjected: 1",
		"X-Nul":             "a\x00b",
		"Empty":             "  ",
		"Bad Key":           "v",
	}
	clean, rejected := SanitizeHeaders(in)
	wantClean := map[string]string{"User-Agent": "示例UA", "Referer": "http://ref.example/"}
	if !reflect.DeepEqual(clean, wantClean) {
		t.Errorf("clean = %v，期望 %v", clean, wantClean)
	}
	wantRejected := []string{"Bad Key", "Content-Length", "Empty", "Host", "Transfer-Encoding", "X-Bad", "X-Nul"}
	if !reflect.DeepEqual(rejected, wantRejected) {
		t.Errorf("rejected = %v，期望 %v", rejected, wantRejected)
	}
	if in["Host"] != "evil.example" || in["X-Bad"] != "a\r\nInjected: 1" {
		t.Error("SanitizeHeaders 不应修改入参 map")
	}
	if got, rejected := SanitizeHeaders(nil); got != nil || rejected != nil {
		t.Errorf("SanitizeHeaders(nil) = %v/%v，期望 nil/nil", got, rejected)
	}
}

// TestUnsafeHeaderReason 逐类断言拒绝原因（validate 与导入都用它回显）。
func TestUnsafeHeaderReason(t *testing.T) {
	cases := []struct{ name, key, value, wantPart string }{
		{"合法", "Referer", "http://ref.example/", ""},
		{"CR 注入", "X-A", "a\rb", "控制字符"},
		{"LF 注入", "X-A", "a\nb", "控制字符"},
		{"Host", "Host", "evil.example", "Host"},
		{"Content-Length", "Content-Length", "1", "请求长度"},
		{"Transfer-Encoding", "transfer-encoding", "chunked", "传输编码"},
		{"空值", "X-A", "  ", "值为空"},
		{"非法键名", "Bad Key", "v", "token"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := UnsafeHeaderReason(c.key, c.value)
			if c.wantPart == "" {
				if got != "" {
					t.Fatalf("UnsafeHeaderReason(%q) = %q，期望安全", c.key, got)
				}
				return
			}
			if !containsString(got, c.wantPart) {
				t.Fatalf("UnsafeHeaderReason(%q) = %q，期望包含 %q", c.key, got, c.wantPart)
			}
		})
	}
}

// containsString 小工具：避免为一个断言引入 strings（与包内测试风格一致）。
func containsString(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
