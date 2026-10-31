package ruleexec

import (
	"strings"
	"testing"
)

// TestPipelineAliases 覆盖 24 个管道函数里此前只走主路径的那些。
func TestPipelineAliases(t *testing.T) {
	ctx := newCtx(t)
	cases := []struct{ rule, want string }{
		{`.title@text | replace(/例/, "")`, "示书一"},
		{`.title@text | replace(/示例/, 样)`, "样书一"},
		{`.title@text | split(书) | nth(2)`, "一"},
		{`.title@text | split(书) | nth(5)`, ""},
		{`.title@text | split(书) | nth(0)`, ""},
		{`.title@text | split(书) | first() | default(空)`, "示例"},
		{`.none@text | split(书) | last()`, ""},
		{`.title@text | number()`, "0"},
		{`.title@text | urlencode() | urldecode()`, "示例书一"},
		{`.title@text | b64enc() | b64dec()`, "示例书一"},
		{`.title@text | dedupe()`, "示例书一"},
	}
	for _, tc := range cases {
		if got := eval(t, tc.rule, ctx); got != tc.want {
			t.Errorf("%q = %q，期望 %q", tc.rule, got, tc.want)
		}
	}
}

func TestPipelineURLAndBase64(t *testing.T) {
	ctx := newCtx(t)
	enc := eval(t, `.title@text | urlencode()`, ctx)
	if !strings.Contains(enc, "%") {
		t.Fatalf("urlencode 结果不像 URL 编码: %q", enc)
	}
	dec := eval(t, `.title@text | b64dec()`, ctx)
	if dec != "" {
		t.Fatalf("非法 base64 应返回空，实际 %q", dec)
	}
	encB64 := eval(t, `.title@text | b64enc()`, ctx)
	if encB64 == "" || encB64 == "示例书一" {
		t.Fatalf("b64enc 结果异常: %q", encB64)
	}
}

func TestPipelineJSON(t *testing.T) {
	ctx := newCtx(t)
	ctx.Vars["payload"] = `{"a":{"b":"取值"}}`
	if got := eval(t, `getvar(payload) | json($.a.b)`, ctx); got != "取值" {
		t.Errorf("json(path) 结果 = %q", got)
	}
	ctx.Vars["bad"] = "not json"
	if got := eval(t, `getvar(bad) | json($.a)`, ctx); got != "" {
		t.Errorf("非法 JSON 应返回空，实际 %q", got)
	}
	if _, err := parseJSON("{"); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

func TestPipelineErrors(t *testing.T) {
	ctx := newCtx(t)
	bad := []string{
		`.title@text | concat()`,
		`.title@text | replace(/示例/)`,
	}
	for _, rule := range bad {
		r, err := Compile(rule)
		if err != nil {
			continue // 编译期就拒绝也算通过
		}
		if _, err := r.Eval(ctx); err == nil {
			t.Errorf("%q 应报错", rule)
		}
	}
}

func TestOwnText(t *testing.T) {
	ctx := newCtx(t)
	if got := eval(t, `.item@ownText`, ctx); strings.Contains(got, "示例书一") {
		t.Fatalf("ownText 不应包含子元素文本，实际 %q", got)
	}
}

func TestSyntaxEdges(t *testing.T) {
	ctx := newCtx(t)
	// 正则里的 | 不应被当成管道分隔符
	if got := eval(t, `.title@text##示例|书##X`, ctx); got != "XX一" {
		t.Errorf("正则内 | 处理错误: %q", got)
	}
	// XPath 的 @attr 不被当后缀
	if got := eval(t, `@XPath://span[@class='author']`, ctx); got != "作者甲" {
		t.Errorf("XPath 属性谓词被拆坏: %q", got)
	}
	// 纯管道规则
	ctx.Vars["v"] = "值"
	if got := eval(t, `getvar(v)`, ctx); got != "值" {
		t.Errorf("纯管道规则 = %q", got)
	}
	// 未闭合引号 / 未知函数在编译期报错
	for _, rule := range []string{`.a@text | join("x)`, `.a@text | nosuch()`, ``} {
		if _, err := Compile(rule); err == nil {
			t.Errorf("%q 应编译失败", rule)
		}
	}
}

func TestDegradeMatrix(t *testing.T) {
	requireFieldsStatus(t, "ruleX.", "unusable",
		[]string{"bookList", "chapterList", "content", "searchUrl", "exploreUrl"})
	requireFieldsStatus(t, "ruleBookInfo.", "partial",
		[]string{"name", "author", "intro", "coverUrl", "nextContentUrl", "chapterName"})

	clean, err := CompileFields(map[string]string{"ruleBookInfo.name": ".a@text"})
	if err != nil {
		t.Fatal(err)
	}
	if clean.Status() != "ok" {
		t.Errorf("无降级应为 ok，实际 %s", clean.Status())
	}
	block, field := splitFieldKey("chapterList")
	if block != "" || field != "chapterList" {
		t.Errorf("无点 key 拆解错误: %q / %q", block, field)
	}
}

// requireFieldsStatus 断言一组字段在 @js 规则下得到的整体状态。
func requireFieldsStatus(t *testing.T, prefix, want string, fields []string) {
	t.Helper()
	for _, f := range fields {
		fs, err := CompileFields(map[string]string{prefix + f: "@js:x"})
		if err != nil {
			t.Fatal(err)
		}
		if fs.Status() != want {
			t.Errorf("%s 应为 %s，实际 %s", f, want, fs.Status())
		}
	}
}

func TestJSONPathEdges(t *testing.T) {
	var v any
	if err := parseJSONInto(`{"a":{"b":[{"name":"甲"}]},"n":3,"ok":true}`, &v); err != nil {
		t.Fatal(err)
	}
	ctx := &Context{JSON: v}
	cases := []struct{ rule, want string }{
		{`@json:$..name`, "甲"},
		{`@json:$.a.b[5]`, ""},
		{`@json:$.a[0]`, ""},
		{`@json:$.n`, "3"},
		{`@json:$.ok`, "true"},
		{`@json:$.missing.deep`, ""},
	}
	for _, tc := range cases {
		if got := eval(t, tc.rule, ctx); got != tc.want {
			t.Errorf("%q = %q，期望 %q", tc.rule, got, tc.want)
		}
	}
	if got, _ := jsonValue(&Context{}, "$.a"); got != "" {
		t.Errorf("无 JSON 上下文应返回空，实际 %q", got)
	}
	badPaths := []string{"a.b", "$.a[", "$.a[x]", "$!"}
	for _, p := range badPaths {
		if _, err := parseJSONPath(p); err == nil {
			t.Errorf("路径 %q 应解析失败", p)
		}
	}
}

// parseJSONInto 是测试用的 JSON 解析助手。
func parseJSONInto(s string, out *any) error {
	v, err := parseJSON(s)
	if err != nil {
		return err
	}
	*out = v
	return nil
}
