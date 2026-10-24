package parser

import (
	"strings"
	"testing"
)

// TestTitleFromFilenameShapes 覆盖真实语料里出现的各类书名包装形态。
// 全部用合成字符串，不含任何真实书名。
func TestTitleFromFilenameShapes(t *testing.T) {
	cases := []struct {
		in     string
		title  string
		author string
	}{
		// 主形态：序号 + 【书名】
		{"623.【某书名】.txt", "某书名", ""},
		{"【某书名】.txt", "某书名", ""},
		{"《某书名》.txt", "某书名", ""},
		// 书名内有副标题括号：保留；尾部编号括号：剥掉。
		{"【某书名（副标题）】（01_99）.txt", "某书名（副标题）", ""},
		{"【某书名（副标题）】.txt", "某书名（副标题）", ""},
		// 括号不配对：取最左开括号到最近闭括号之间的内容。
		{"606.【某书名》（未删节1-870章）作者：某人.txt", "某书名", "某人"},
		// 书名后跟标签块：标签要被剥掉。
		{"【某书名】【更新至2516大结局】【搜书吧】.txt", "某书名", ""},
		{"645.【某书名】重修加料版.txt", "某书名", ""},
		// 块内「书名-作者」形态。
		{"637.【某书名（1-5卷27章）-某作者】.txt", "某书名", "某作者"},
		// 块内标签堆：只有第一段是书名。
		{"【某书名，某标签，某标签，某标签】某字,123.txt", "某书名", ""},
		// 数字开头的真书名：不能当序号剥掉。
		{"2012末日狂欢.txt", "2012末日狂欢", ""},
		// 序号 + 真书名：序号要剥，书名保留。
		{"10.1某书名（1-1990）.txt", "某书名", ""},
		{"129 某书名[搜书吧].txt", "某书名", ""},
	}
	for _, c := range cases {
		title, author, _ := titleFromFilename(c.in)
		if title != c.title {
			t.Errorf("%q 书名 = %q，期望 %q", c.in, title, c.title)
		}
		if author != c.author {
			t.Errorf("%q 作者 = %q，期望 %q", c.in, author, c.author)
		}
	}
}

// TestTitleFromFilenameNoResidue 是缺陷 1 的回归测试：
// 书名里不许残留包装符号或广告词。
func TestTitleFromFilenameNoResidue(t *testing.T) {
	names := []string{
		"623.【某书名】.txt",
		"606.【某书名》（未删节1-870章）作者：某人.txt",
		"【某书名】（01_99）.txt",
		"【某书名】【搜书吧】.txt",
		"640.某书名（1-1990）.txt",
		"124.【某书名】作者：某人[搜书吧].txt",
	}
	bad := []string{"【", "】", "《", "》", "搜书吧", "未删节", "作者：", "(1-", "（1-"}
	for _, name := range names {
		title, _, _ := titleFromFilename(name)
		if title == "" {
			t.Errorf("%q 书名不应为空", name)
			continue
		}
		for _, b := range bad {
			if strings.Contains(title, b) {
				t.Errorf("%q 书名残留 %q: %q", name, b, title)
			}
		}
	}
}

// TestTitleFromFilenameEdge 覆盖空文件名、纯扩展名等边界。
func TestTitleFromFilenameEdge(t *testing.T) {
	if title, author, _ := titleFromFilename("   .txt"); title != "" {
		t.Errorf("空主干书名 = %q", title)
	}
	if title, _, _ := titleFromFilename(""); title != "" {
		t.Errorf("空文件名 = %q", title)
	}
	// 没有扩展名：整串当书名。
	if title, _, _ := titleFromFilename("某书名"); title != "某书名" {
		t.Errorf("无扩展名书名 = %q", title)
	}
}

// TestStripFileDecorKeepsOriginal 覆盖「剥光时回退原串」这条保护路径。
func TestStripFileDecorKeepsOriginal(t *testing.T) {
	// 整串都是装饰字符时不能返回空串。
	if got := stripFileDecor("【】"); got == "" {
		t.Error("剥光后应回退原串，而不是返回空")
	}
	if got := stripFileDecor("  640.  "); got != "" {
		t.Logf("纯序号剥除 = %q", got)
	}
}

// TestFirstBracketBlock 覆盖括号块定位：配对、不配对、无括号三种情况。
func TestFirstBracketBlock(t *testing.T) {
	if inner, ok := firstBracketBlock("前缀【某书名】后缀"); !ok || inner != "某书名" {
		t.Errorf("配对括号 = %q %v", inner, ok)
	}
	// 不配对：【后只有 》：取到最近的闭括号为止。
	if inner, ok := firstBracketBlock("【某书名》其余"); !ok || inner != "某书名" {
		t.Errorf("不配对 = %q %v", inner, ok)
	}
	if _, ok := firstBracketBlock("没有括号"); ok {
		t.Error("无括号时应放弃")
	}
	// 完全没有闭括号：整段当内容。
	if inner, ok := firstBracketBlock("【没有闭合"); !ok || inner != "没有闭合" {
		t.Errorf("无闭括号 = %q %v", inner, ok)
	}
}

// TestStripTitleTagBlocks 覆盖块内标签剥离（保留真副标题）。
func TestStripTitleTagBlocks(t *testing.T) {
	if got := stripTitleTagBlocks("某书名【全本】"); got != "某书名" {
		t.Errorf("标签块未剥除: %q", got)
	}
	if got := stripTitleTagBlocks("某书名（副标题）"); got != "某书名（副标题）" {
		t.Errorf("真副标题被误删: %q", got)
	}
	if got := stripTitleTagBlocks("某书名（1-1990）"); got != "某书名" {
		t.Errorf("编号括号未剥除: %q", got)
	}
}

// TestStripTitleTailTags 覆盖书名尾部编号剥离。
func TestStripTitleTailTags(t *testing.T) {
	cases := map[string]string{
		"某书名（01_99）": "某书名",
		"某书名(1-180)": "某书名",
		"某书名（副标题）":   "某书名（副标题）",
		"某书名":        "某书名",
		"某书名（全本）":    "某书名",
	}
	for in, want := range cases {
		if got := stripTitleTailTags(in); got != want {
			t.Errorf("%q → %q，期望 %q", in, got, want)
		}
	}
}

// TestNearestClose 覆盖「找最近闭括号」的兜底逻辑。
func TestNearestClose(t *testing.T) {
	if got := nearestClose("某书名》后半"); got != "某书名" {
		t.Errorf("nearestClose = %q", got)
	}
	if got := nearestClose("没有闭括号"); got != "没有闭括号" {
		t.Errorf("无闭括号 = %q", got)
	}
}

// TestSplitTitleAuthor 覆盖「书名-作者」切分的成功与各类放弃路径。
func TestSplitTitleAuthor(t *testing.T) {
	if title, author := splitTitleAuthor("某书名-某作者"); title != "某书名" || author != "某作者" {
		t.Errorf("正常切分 = %q / %q", title, author)
	}
	// 没有分隔符：整串是书名。
	if title, author := splitTitleAuthor("某书名"); title != "某书名" || author != "" {
		t.Errorf("无分隔符 = %q / %q", title, author)
	}
	// 右侧是状态词：不当作者。
	if title, author := splitTitleAuthor("某书名-全本"); author != "" {
		t.Errorf("状态词被当作者: %q / %q", title, author)
	}
	// 右侧过长：不当作者。
	if title, author := splitTitleAuthor("某书名-这是一个非常非常长的后缀不可能是作者"); author != "" {
		t.Errorf("过长后缀被当作者: %q / %q", title, author)
	}
	// 右侧含括号：不当作者。
	if title, author := splitTitleAuthor("某书名-（某作者）"); author != "" {
		t.Errorf("含括号后缀被当作者: %q / %q", title, author)
	}
}

// TestUnwrapWholeBrackets 覆盖整串被括号包住的判定。
func TestUnwrapWholeBrackets(t *testing.T) {
	if inner, ok := unwrapWholeBrackets("【某内容】"); !ok || inner != "某内容" {
		t.Errorf("整串括号 = %q %v", inner, ok)
	}
	if _, ok := unwrapWholeBrackets("前缀【某内容】"); ok {
		t.Error("不是整串包住时应放弃")
	}
	if _, ok := unwrapWholeBrackets("【有【嵌套】】"); ok {
		t.Error("嵌套括号应放弃")
	}
}
