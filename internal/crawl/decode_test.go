package crawl

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// mustGBK 把 UTF-8 文本编成 GBK 字节（测试夹具）。
func mustGBK(t *testing.T, s string) []byte {
	t.Helper()
	out, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("GBK 编码失败: %v", err)
	}
	return out
}

// TestDecodeBody 覆盖解码优先级与兜底：URL 选项 > forceEncoding > meta > UTF-8。
func TestDecodeBody(t *testing.T) {
	page := "<html><head><meta charset=\"gbk\"></head><body>中文书名</body></html>"
	gbkPage := mustGBK(t, page)
	utf8Page := []byte("<html><body>中文书名</body></html>")

	cases := []struct {
		name     string
		body     []byte
		explicit string
		forced   string
		want     string
		used     string
		warn     bool
	}{
		{"URL 选项 charset", gbkPage, "gbk", "", "中文书名", "gb18030", false},
		{"GB2312 别名", gbkPage, "gb2312", "", "中文书名", "gb18030", false},
		{"forceEncoding 兜底", gbkPage, "", "gbk", "中文书名", "gb18030", false},
		{"meta 探测", gbkPage, "", "", "中文书名", "gb18030", false},
		{"显式编码优先于 meta", utf8Page, "utf-8", "", "中文书名", "utf-8", false},
		{"UTF-8 页面无降级", utf8Page, "", "", "中文书名", "utf-8", false},
		{"未知编码降级", utf8Page, "x-bogus", "", "中文书名", "utf-8", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, used, warn := decodeBody(c.body, c.explicit, c.forced)
			if !strings.Contains(text, c.want) || used != c.used {
				t.Fatalf("decodeBody = (%q, %q)，期望包含 %q / 编码 %q", text, used, c.want, c.used)
			}
			if (warn != "") != c.warn {
				t.Fatalf("warning = %q，期望有降级=%v", warn, c.warn)
			}
		})
	}
}

// TestDecodeBodyInvalidGBK 非法 GBK 字节只降级、不 panic、不报错。
func TestDecodeBodyInvalidGBK(t *testing.T) {
	_, used, warn := decodeBody([]byte{0x81, 0x81, 0x81}, "gbk", "")
	if used != "gb18030" {
		t.Errorf("used = %q，期望 gb18030（解码失败也记录尝试的编码）", used)
	}
	if warn == "" {
		t.Errorf("非法 GBK 字节应给出降级说明")
	}
}
