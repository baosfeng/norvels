package ruleexec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

const sampleHTML = `
<html><body>
<div class="list">
  <div class="item"><a class="title" href="/book/1">示例书一</a><span class="author">作者甲</span></div>
  <div class="item"><a class="title" href="/book/2">示例书二</a><span class="author">作者乙</span></div>
</div>
<div id="content"><p>第一段。</p><p>第二段。</p></div>
</body></html>`

// newCtx 构造 HTML 求值上下文。
func newCtx(t *testing.T) *Context {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(sampleHTML))
	if err != nil {
		t.Fatalf("解析样例 HTML 失败: %v", err)
	}
	return &Context{HTML: doc, Vars: map[string]string{"baseUrl": "https://example.invalid"}}
}

// eval 编译并求值一条规则。
func eval(t *testing.T, raw string, ctx *Context) string {
	t.Helper()
	r, err := Compile(raw)
	if err != nil {
		t.Fatalf("编译 %q 失败: %v", raw, err)
	}
	v, err := r.Eval(ctx)
	if err != nil {
		t.Fatalf("求值 %q 失败: %v", raw, err)
	}
	return v
}

func TestCSSSelectors(t *testing.T) {
	ctx := newCtx(t)
	cases := []struct{ rule, want string }{
		{"@css:.item .title@text", "示例书一"},
		{".item .title@text", "示例书一"},
		{".item .title@href", "/book/1"},
		{"@css:.item .author@text", "作者甲"},
		{"@css:#content p@text", "第一段。"},
		{"@css:#content@html", "<p>第一段。</p><p>第二段。</p>"},
	}
	for _, tc := range cases {
		if got := eval(t, tc.rule, ctx); got != tc.want {
			t.Errorf("%q = %q，期望 %q", tc.rule, got, tc.want)
		}
	}
}

func TestXPathSelectors(t *testing.T) {
	ctx := newCtx(t)
	cases := []struct{ rule, want string }{
		{"@XPath://div[@class='item']/a/@href", "/book/1"},
		{"@XPath://span[@class='author']", "作者甲"},
		{"@XPath://div[@id='content']/p[2]", "第二段。"},
	}
	for _, tc := range cases {
		if got := eval(t, tc.rule, ctx); got != tc.want {
			t.Errorf("%q = %q，期望 %q", tc.rule, got, tc.want)
		}
	}
}

func TestJSONPath(t *testing.T) {
	var v any
	if err := json.Unmarshal([]byte(`{"data":{"books":[{"name":"示例书一"},{"name":"示例书二"}]}}`), &v); err != nil {
		t.Fatal(err)
	}
	ctx := &Context{JSON: v}
	cases := []struct{ rule, want string }{
		{"@json:$.data.books[0].name", "示例书一"},
		{"@json:$.data.books[*].name", "示例书一"},
		{"@json:$..name", "示例书一"},
		{"@json:$.missing", ""},
	}
	for _, tc := range cases {
		if got := eval(t, tc.rule, ctx); got != tc.want {
			t.Errorf("%q = %q，期望 %q", tc.rule, got, tc.want)
		}
	}
}

func TestRegexReplacement(t *testing.T) {
	ctx := newCtx(t)
	if got := eval(t, `.title@text##示例##样`, ctx); got != "样书一" {
		t.Errorf("替换结果 = %q", got)
	}
	if got := eval(t, `.title@text##书[一二]##`, ctx); got != "示例" {
		t.Errorf("删除结果 = %q", got)
	}
}

func TestPipelines(t *testing.T) {
	ctx := newCtx(t)
	cases := []struct{ rule, want string }{
		{`.title@text | concat("!")`, "示例书一!"},
		{`.title@text | trim() | default("兜底")`, "示例书一"},
		{`.none@text | default("兜底")`, "兜底"},
		{`.title@text | match(/示例(.)/, 1)`, "书"},
		{`.title@text | sub(0, 2)`, "示例"},
		{`.title@text | sub(-2)`, "书一"},
		{`.title@href | prefix("https://example.invalid") | abs()`, "https://example.invalid/book/1"},
		{`.title@text | split(书) | join(-)`, "示例-一"},
		{`.title@text | split(书) | first()`, "示例"},
		{`.title@text | split(书) | last()`, "一"},
		{`.title@text | setvar(name)`, "示例书一"},
	}
	for _, tc := range cases {
		if got := eval(t, tc.rule, ctx); got != tc.want {
			t.Errorf("%q = %q，期望 %q", tc.rule, got, tc.want)
		}
	}
	if got := eval(t, `getvar(name) | prefix("书:")`, ctx); got != "书:示例书一" {
		t.Errorf("getvar 结果 = %q", got)
	}
}

func TestTemplateVars(t *testing.T) {
	ctx := newCtx(t)
	ctx.Vars["sel"] = ".title"
	if got := eval(t, `@css:{{sel}}@text`, ctx); got != "示例书一" {
		t.Errorf("模板替换结果 = %q", got)
	}
	if got := eval(t, `{{unknown}}`, ctx); got != "" {
		t.Errorf("未知变量应替换为空，实际 %q", got)
	}
}

func TestCombine(t *testing.T) {
	ctx := newCtx(t)
	if got := eval(t, `.missing@text && .title@text`, ctx); got != "示例书一" {
		t.Errorf("&& 结果 = %q", got)
	}
	if got := eval(t, `.missing@text || .author@text`, ctx); got != "作者甲" {
		t.Errorf("|| 结果 = %q", got)
	}
	if got := eval(t, `.missing@text || .also-missing@text`, ctx); got != "" {
		t.Errorf("全空结果应为空，实际 %q", got)
	}
}

func TestSelectAll(t *testing.T) {
	ctx := newCtx(t)
	r, err := Compile(".item .title")
	if err != nil {
		t.Fatal(err)
	}
	items, err := r.SelectAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("期望 2 个条目，实际 %d", len(items))
	}
}

func TestCompileErrors(t *testing.T) {
	bad := []string{"", "@css:.a | nosuchfn()", "@css:.a##[##x"}
	for _, raw := range bad {
		if _, err := Compile(raw); err == nil {
			t.Errorf("%q 应编译失败", raw)
		}
	}
}

func TestJSDegradation(t *testing.T) {
	r, err := Compile("@js:java.ajax(url)")
	if err != nil {
		t.Fatalf("降级规则不该编译失败: %v", err)
	}
	if !r.Degraded {
		t.Fatal("@js 规则应标记降级")
	}
	ctx := newCtx(t)
	if v, err := r.Eval(ctx); err != nil || v != "" {
		t.Fatalf("降级规则应返回空串且无错误，实际 %q / %v", v, err)
	}
	lit, err := Compile("@css:.title@text | concat(@js:)")
	if err != nil {
		t.Fatal(err)
	}
	if lit.Degraded {
		t.Fatal("字符串字面量里的 @js: 不应触发降级")
	}
}

func TestFieldSetDegradation(t *testing.T) {
	fs, err := CompileFields(map[string]string{
		"ruleSearch.bookList": ".item",
		"ruleToc.chapterList": "@js:java.ajax(url)",
		"ruleBookInfo.name":   "@js:java.ajax(url)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fs.Status() != "unusable" {
		t.Fatalf("含 hard 降级应为 unusable，实际 %s", fs.Status())
	}
	if len(fs.Degraded) != 2 {
		t.Fatalf("期望 2 条降级记录，实际 %d", len(fs.Degraded))
	}
	soft, err := CompileFields(map[string]string{"ruleBookInfo.name": "@js:x"})
	if err != nil {
		t.Fatal(err)
	}
	if soft.Status() != "partial" {
		t.Fatalf("只有 soft 降级应为 partial，实际 %s", soft.Status())
	}
}
