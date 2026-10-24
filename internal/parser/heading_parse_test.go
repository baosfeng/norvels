package parser

import (
	"strings"
	"testing"
)

// TestParseHeadingDiUnit 覆盖「第N章」主形态与弱形态的识别。
func TestParseHeadingDiUnit(t *testing.T) {
	cases := []struct {
		line  string
		kind  headKind
		num   int
		title string
	}{
		{"第一章 风起", headChapter, 1, "风起"},
		{"第12章 云涌", headChapter, 12, "云涌"},
		{"第一百零八章 归途", headChapter, 108, "归途"},
		{"　第三章　女歌手", headChapter, 3, "女歌手"},
		{"第5节 小结", headChapter, 5, "小结"},
		{"第2回 重逢", headChapter, 2, "重逢"},
		{"Chapter 7 The End", headChapter, 7, "The End"},
		{"序章", headSpecial, 0, ""},
		{"楔子 缘起", headSpecial, 0, "缘起"},
		{"第一卷 风云", headVolume, 0, ""},
	}
	for _, c := range cases {
		h, ok := parseHeading(c.line)
		if !ok {
			t.Errorf("%q 应识别为标题", c.line)
			continue
		}
		if h.kind != c.kind {
			t.Errorf("%q kind = %d，期望 %d", c.line, h.kind, c.kind)
		}
		if c.num > 0 && h.num != c.num {
			t.Errorf("%q num = %d，期望 %d", c.line, h.num, c.num)
		}
		if c.title != "" && h.title != c.title {
			t.Errorf("%q title = %q，期望 %q", c.line, h.title, c.title)
		}
	}
}

// TestParseHeadingRejects 覆盖「不是标题」的各条拒识规则。
func TestParseHeadingRejects(t *testing.T) {
	bad := []string{
		"",
		"他转过身，看向远方，心里想着很多事情。",
		"这一段话以句号结尾，所以不是标题。",
		strings.Repeat("很长的标题行", 40),
	}
	for _, line := range bad {
		if h, ok := parseHeading(line); ok {
			t.Errorf("%q 不应识别为标题，得到 %+v", line, h)
		}
	}
}

// TestParseHeadingVolumeWithChapter 覆盖「卷名 + 章号同行」这类复合标题。
func TestParseHeadingVolumeWithChapter(t *testing.T) {
	h, ok := parseHeading("第一卷 风云初起 第一章 出发")
	if !ok {
		t.Fatal("复合标题应被识别")
	}
	if h.kind != headChapter {
		t.Errorf("kind = %d，期望章", h.kind)
	}
	if h.volTitle == "" {
		t.Error("卷标题不应为空")
	}
}

// TestParseHeadingZhengwenPrefix 覆盖「正文 第N章」这类带前缀的写法。
func TestParseHeadingZhengwenPrefix(t *testing.T) {
	h, ok := parseHeading("正文 第一章 开始")
	if !ok || h.kind != headChapter {
		t.Fatalf("正文前缀应被剥掉: %+v ok=%v", h, ok)
	}
	if h.num != 1 {
		t.Errorf("num = %d", h.num)
	}
	// 只有「正文卷」：应当成卷标记。
	if h, ok := parseHeading("正文卷"); !ok || h.kind != headVolume {
		t.Errorf("正文卷应识别为卷: %+v ok=%v", h, ok)
	}
}

// TestParseNumeralShapes 覆盖阿拉伯数字 / 全角数字 / 中文数字三条分支。
func TestParseNumeralShapes(t *testing.T) {
	cases := []struct {
		in  string
		num int
		ok  bool
	}{
		{"123", 123, true},
		{"０７", 7, true},
		{"一百零五", 105, true},
		{"两", 2, true},
		{"十", 10, true},
		{"abc", 0, false},
		{"零", 0, false},
	}
	for _, c := range cases {
		num, _, ok := parseNumeral(c.in)
		if ok != c.ok {
			t.Errorf("%q ok = %v，期望 %v", c.in, ok, c.ok)
			continue
		}
		if c.ok && num != c.num {
			t.Errorf("%q num = %d，期望 %d", c.in, num, c.num)
		}
	}
}

// TestParseInlineHeading 覆盖「标题与正文挤在同一行」的解析。
func TestParseInlineHeading(t *testing.T) {
	line := "第三章   女歌手   一位身穿白色短裙的女孩走了进来，她看起来很紧张。"
	h, off, ok := parseInlineHeading(line)
	if !ok {
		t.Fatal("应解析出行内标题")
	}
	if h.num != 3 {
		t.Errorf("num = %d", h.num)
	}
	if off <= 0 || off > len(line) {
		t.Errorf("正文偏移 = %d", off)
	}
	// 没有标记的行：放弃。
	if _, _, ok := parseInlineHeading("这是一段没有标记的正文内容。"); ok {
		t.Error("无标记行不应解析出标题")
	}
}

// TestParsePrefixedHeading 覆盖「书名 第N章 标题」这类带前缀的标题。
func TestParsePrefixedHeading(t *testing.T) {
	h, ok := parsePrefixedHeading("某书名 第一章 出发")
	if !ok || h.kind != headChapter {
		t.Fatalf("带前缀标题应被识别: %+v ok=%v", h, ok)
	}
	if h.volTitle != "某书名" {
		t.Errorf("前缀 = %q", h.volTitle)
	}
	// 前缀中有句末标点：多半是正文，不应识别。
	if _, ok := parsePrefixedHeading("他说：这件事 第一章 我不知道。"); ok {
		t.Error("正文句不应识别为标题")
	}
}

// TestFindChapterMarker 覆盖章标记定位的正常与未找到路径。
func TestFindChapterMarker(t *testing.T) {
	if idx := findChapterMarker("前缀 第一章 标题"); idx != len("前缀 ") {
		t.Errorf("idx = %d", idx)
	}
	if idx := findChapterMarker("没有任何标记"); idx != -1 {
		t.Errorf("未找到时应返回 -1，实际 %d", idx)
	}
}

// TestHelpersCoverage 覆盖若干纯函数助手。
func TestHelpersCoverage(t *testing.T) {
	if !isPadRune(' ') || isPadRune('x') {
		t.Error("isPadRune 判定错误")
	}
	if !isSpecialTitle("番外 一") || isSpecialTitle("正文") {
		t.Error("isSpecialTitle 判定错误")
	}
	if got := joinTitle("第一章", "标题"); got != "第一章 标题" {
		t.Errorf("joinTitle = %q", got)
	}
	if got := truncateRunes("abcdef", 3); got != "abc" {
		t.Errorf("truncateRunes = %q", got)
	}
	if got := normalizeText("a\r\nb\u200b"); got != "a\nb" {
		t.Errorf("normalizeText = %q", got)
	}
	if n := runeLen("中文abc"); n != 5 {
		t.Errorf("runeLen = %d", n)
	}
	if !hasHan("中文") || hasHan("abc") {
		t.Error("hasHan 判定错误")
	}
	if !isLikelyIntroLine("这是一段足够长的简介文字，可以用作简介。") {
		t.Error("isLikelyIntroLine 判定错误")
	}
	if !isLikelyTitleLine("作者：某人") || isLikelyTitleLine("这是一段普通正文内容。") {
		t.Error("isLikelyTitleLine 判定错误")
	}
	if got := cleanContentViaCleaner("正文\n====\n更多"); strings.Contains(got, "====") {
		t.Errorf("分隔线未清除: %q", got)
	}
}

// cleanContentViaCleaner 用全规则清洗器处理一段文本（覆盖 cleanContent 入口）。
func cleanContentViaCleaner(s string) string { return fullCleaner.cleanContent(s) }

// TestMergeCandidates 覆盖候选合并与去重。
func TestMergeCandidates(t *testing.T) {
	a := []candidate{{start: 0}, {start: 100}}
	// 距离过近的候选（< 8 字节）会被去重，所以 101 与 100 只保留一个。
	b := []candidate{{start: 50}, {start: 200}}
	out := mergeCandidates(a, b)
	if len(out) != 4 {
		t.Fatalf("合并后 = %d，期望 4", len(out))
	}
	// offset 0 的候选不能被去重掉：以「第一章」开头的书必须保住首章标题。
	first := mergeCandidates([]candidate{{start: 0}}, nil)
	if len(first) != 1 || first[0].start != 0 {
		t.Errorf("offset 0 的候选被误删: %v", first)
	}
	for i := 1; i < len(out); i++ {
		if out[i].start < out[i-1].start {
			t.Errorf("未按位置排序: %v", out)
		}
	}
	if got := mergeCandidates(a, nil); len(got) != 2 {
		t.Errorf("合入空列表 = %d", len(got))
	}
	// 位置过近的候选应被去重。
	dup := mergeCandidates([]candidate{{start: 100}}, []candidate{{start: 101}})
	if len(dup) != 1 {
		t.Errorf("过近候选未去重: %v", dup)
	}
}

// TestDecodePublicAPI 覆盖公开的 Decode 入口（含各编码分支）。
func TestDecodePublicAPI(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		enc  string
	}{
		{"utf8", []byte("中文"), EncUTF8},
		{"utf8-bom", append([]byte{0xEF, 0xBB, 0xBF}, []byte("中文")...), EncUTF8BOM},
		{"gb18030", []byte{0xD6, 0xD0, 0xCE, 0xC4}, EncGB18030},
	}
	for _, c := range cases {
		enc, text := Decode(c.raw)
		if enc != c.enc {
			t.Errorf("%s 编码 = %q，期望 %q", c.name, enc, c.enc)
		}
		if text == "" {
			t.Errorf("%s 解码结果为空", c.name)
		}
	}
}

// TestSplitChapterContent 覆盖超长章切分的各条分支。
func TestSplitChapterContent(t *testing.T) {
	short := "短内容"
	if parts := splitChapterContent(short, 100); parts != nil {
		t.Errorf("未超限应返回 nil，实际 %v", parts)
	}
	long := strings.Repeat("这是一段用于测试切分的合成正文。\n", 500)
	parts := splitChapterContent(long, 500)
	if len(parts) < 2 {
		t.Fatalf("超限应切分，实际 %d 段", len(parts))
	}
	// 单行超长：应退化为按字符硬切而不是切不动。
	oneLine := strings.Repeat("很长的单行内容", 200)
	parts = splitChapterContent(oneLine, 500)
	if len(parts) < 2 {
		t.Errorf("超长单行应被硬切，实际 %d 段", len(parts))
	}
	if got := hardSplit("abcdef", 2); len(got) != 3 {
		t.Errorf("hardSplit = %v", got)
	}
}
