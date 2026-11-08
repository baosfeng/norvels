package source

import "testing"

// TestConvertLegadoSelector 覆盖每个语法点：基本选择器、索引、排除、多级 @、
// 后缀、原生前缀放行、## 正则与管道保留，以及无法转换时的降级。
func TestConvertLegadoSelector(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		degrade bool
	}{
		// 基本选择器
		{"tag 基本选择器", "tag.div", "div", false},
		{"class 基本选择器", "class.foo", ".foo", false},
		{"id 基本选择器", "id.bar", "#bar", false},
		// 索引与排除
		{"索引 0 省略", "class.odd.0@tag.a@text", ".odd a@text", false},
		{"索引 N", "tag.div.1@text", "div:nth-child(2)@text", false},
		{"负索引", "tag.div.-1@text", "div:nth-last-child(1)@text", false},
		{"排除首项", "class.grid@tr!0", ".grid tr:not(:nth-child(1))", false},
		{"排除多项", "tag.div!0:2@text", "div:not(:nth-child(1)):not(:nth-child(3))@text", false},
		{"排除负索引", "tag.tr!-1@text", "tr:not(:nth-last-child(1))@text", false},
		// 多级 @ 与后缀
		{"多级 @", "class.info@h3@text", ".info h3@text", false},
		{"id + 索引 + 后缀", "id.intro@tag.p.0@text", "#intro p@text", false},
		{"ownText 后缀", "class.list@tag.li@ownText", ".list li@ownText", false},
		{"属性后缀", "tag.a@href", "a@href", false},
		{"链尾属性", "class.name@tag.a.0@href", ".name a@href", false},
		{"裸 @ 链", "dt@a@href", "dt a@href", false},
		{"裸链 + 链接元素", "id.chapter-list@li@a", "#chapter-list li a", false},
		{"链尾属性名 title", "class.size@a@title", ".size a@title", false},
		{"链尾标签名 img", "class.book-info@img@src", ".book-info img@src", false},
		{"链尾标签名 span", "class.foo@span", ".foo span", false},
		// 组合与分隔
		{"## 正则保留", "class.foo@text##<b>##", ".foo@text##<b>##", false},
		{"管道保留", "tag.a@href | abs()", "a@href | abs()", false},
		{"|| 分支", "tag.a@text||tag.span@text", "a@text||span@text", false},
		{"&& 分支", "tag.a@text&&tag.span@text", "a@text&&span@text", false},
		// 原生前缀原样放行
		{"@css 前缀", "@css:.item .title@text", "@css:.item .title@text", false},
		{"@XPath 前缀（大小写不敏感）", "@XPath://div[@class=\"x\"]/a/@href", "@XPath://div[@class=\"x\"]/a/@href", false},
		{"@json 前缀", "@json:$.data[*].name", "@json:$.data[*].name", false},
		{"@js 前缀", "@js:java.getString()", "@js:java.getString()", false},
		// 模板 / JS / 纯正则：不是选择器简写，原样放行
		{"模板规则放行", "{{@@class.clamp3@text}}·{{@@class.cite-date@text}}", "{{@@class.clamp3@text}}·{{@@class.cite-date@text}}", false},
		{"<js> 规则放行", "class.even.0@tag.a@href<js>var x = 1;</js>", "class.even.0@tag.a@href<js>var x = 1;</js>", false},
		{"纯正则规则放行", ":<a href=\"([^\"]+)\">([^<]+)</a>", ":<a href=\"([^\"]+)\">([^<]+)</a>", false},
		// 标准 CSS 不动（ruleexec 既有语义不受影响）
		{"标准 CSS 选择器", ".item .title@text", ".item .title@text", false},
		{"标准 CSS 属性后缀", ".name@href", ".name@href", false},
		// 无法安全转换 → 原文 + 降级说明
		{"列表倒置 -", "-tag.dd@tag.a@text", "-tag.dd@tag.a@text", true},
		{"children 类型", "children.li@text", "children.li@text", true},
		{"text 选择器类型", "text.关键字@text", "text.关键字@text", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, warn := ConvertLegadoSelector(c.in)
			if got != c.want {
				t.Fatalf("ConvertLegadoSelector(%q) = %q，期望 %q", c.in, got, c.want)
			}
			if (warn != "") != c.degrade {
				t.Fatalf("ConvertLegadoSelector(%q) 降级说明 = %q，期望降级=%v", c.in, warn, c.degrade)
			}
		})
	}
}

// TestConvertLegadoSelectorRealShapes 是从生产书源里摘出来的 9 种规则形态
// （站点名、书名与站点专有 class 名已换成通用名，只保留选择器写法）。
func TestConvertLegadoSelectorRealShapes(t *testing.T) {
	cases := []struct{ in, want string }{
		{"class.grid@tag.tr!0", ".grid tr:not(:nth-child(1))"},
		{"class.grid@tag.tbody@tag.tr!0", ".grid tbody tr:not(:nth-child(1))"},
		{"class.last@tag.a.0@href", ".last a@href"},
		{"dt@a@href", "dt a@href"},
		{"id.chapter-list@li@a", "#chapter-list li a"},
		{"class.odd.1@text", ".odd:nth-child(2)@text"},
		{"class.info@tag.span.1@text##作者：", ".info span:nth-child(2)@text##作者："},
		{"class.tj@tag.label.3@font@text", ".tj label:nth-child(4) font@text"},
		{"div.condition span!-1:0@text##\\s", "div.condition span:not(:nth-last-child(1)):not(:nth-child(1))@text##\\s"},
	}
	for _, c := range cases {
		got, warn := ConvertLegadoSelector(c.in)
		if warn != "" {
			t.Fatalf("ConvertLegadoSelector(%q) 意外降级：%s", c.in, warn)
		}
		if got != c.want {
			t.Errorf("ConvertLegadoSelector(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestConvertLegadoSelectorIdempotent 转换结果再转换一次必须不变（导入与加载
// 各转换一次时不能互相破坏）。
func TestConvertLegadoSelectorIdempotent(t *testing.T) {
	inputs := []string{
		".odd a@text",
		"div:nth-child(2)@text",
		".grid tr:not(:nth-child(1))",
		".info h3@text",
		".name a@href",
		"dt a@href",
		"#chapter-list li a",
		".foo@text##<b>##",
		"a@href | abs()",
		"@css:.item .title@text",
	}
	for _, in := range inputs {
		got, warn := ConvertLegadoSelector(in)
		if warn != "" || got != in {
			t.Errorf("二次转换改变了规则：%q → %q（降级 %q）", in, got, warn)
		}
	}
}

// TestConvertLegadoRules 批量转换：降级字段保持原文并单独报出来。
func TestConvertLegadoRules(t *testing.T) {
	in := map[string]string{
		"name":     "tag.td.0@tag.a@text",
		"bookList": "-tag.dd@tag.a@text",
	}
	got, degraded := ConvertLegadoRules(in)
	if got["name"] != "td a@text" {
		t.Errorf("name = %q，期望 td a@text", got["name"])
	}
	if got["bookList"] != in["bookList"] {
		t.Errorf("降级字段应保持原文，实际 %q", got["bookList"])
	}
	if len(degraded) != 1 || degraded["bookList"] == "" {
		t.Errorf("degraded = %v，期望只报 bookList", degraded)
	}
	if in["name"] != "tag.td.0@tag.a@text" {
		t.Errorf("批量转换不应修改入参 map：%v", in)
	}
	if got, degraded := ConvertLegadoRules(nil); got != nil || degraded != nil {
		t.Errorf("ConvertLegadoRules(nil) = %v/%v，期望 nil/nil", got, degraded)
	}
}

// TestConvertLegadoSelectorEmpty 空规则原样返回、不报降级（空规则由 ruleexec 报错）。
func TestConvertLegadoSelectorEmpty(t *testing.T) {
	for _, in := range []string{"", "   "} {
		if got, warn := ConvertLegadoSelector(in); got != in || warn != "" {
			t.Errorf("ConvertLegadoSelector(%q) = %q/%q，期望原样/空", in, got, warn)
		}
	}
}
