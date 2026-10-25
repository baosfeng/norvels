package parser

import (
	"strings"
	"testing"
)

// ---------- 第 1 条：孤立闭括号 ----------

// TestTrimOrphanClosers 覆盖孤立闭括号的删除（与 trimUnclosedBracket 对称）。
func TestTrimOrphanClosers(t *testing.T) {
	cases := map[string]string{
		"某书名》（x）》": "某书名（x）",
		"【某书名】":    "【某书名】",
		"某书名》":     "某书名",
		// 孤立的开括号【不归本函数管（那是 trimUnclosedBracket 的职责），所以保留；
		// 孤立的闭括号》被丢弃，(x) 成对保留。
		"【某书名》（x）": "【某书名（x）",
		"「某书名」":    "「某书名」",
		"某书名〉":     "某书名",
		"没有括号":     "没有括号",
	}
	for in, want := range cases {
		if got := trimOrphanClosers(in); got != want {
			t.Errorf("trimOrphanClosers(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestTitleBracketsIncludeCJKQuote 覆盖新增的 「」〈〉 配对。
func TestTitleBracketsIncludeCJKQuote(t *testing.T) {
	for _, pair := range []string{"【】", "[]", "［］", "『』", "《》", "「」", "〈〉"} {
		o, c := []rune(pair)[0], []rune(pair)[1]
		got, ok := closerOf(o)
		if !ok || got != c {
			t.Errorf("closerOf(%q) = %q/%v，期望 %q", o, got, ok, c)
		}
		if !isOpener(o) {
			t.Errorf("%q 应被识别为开括号", o)
		}
	}
}

// TestLongestBracketFree 覆盖「取最长无括号片段」。
func TestLongestBracketFree(t *testing.T) {
	cases := map[string]string{
		"【某书名》（x）": "某书名",
		"某书名》":     "某书名",
		"（副）某书名":   "某书名",
		"没有括号":     "没有括号",
	}
	for in, want := range cases {
		if got := longestBracketFree(in); got != want {
			t.Errorf("longestBracketFree(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestEnforceCleanTitle 覆盖入库前不变量校验的兜底与告警。
func TestEnforceCleanTitle(t *testing.T) {
	d := &Doc{}
	if got := enforceCleanTitle(d, "干净书名"); got != "干净书名" {
		t.Errorf("干净书名被改动: %q", got)
	}
	if len(d.Warnings) != 0 {
		t.Error("干净书名不应产生告警")
	}
	// 带孤立括号：应取最长无括号片段并告警。
	got := enforceCleanTitle(d, "【某书名》（x）")
	if hasBracket(got) {
		t.Errorf("兜底后仍含括号: %q", got)
	}
	if len(d.Warnings) == 0 {
		t.Error("应记录告警")
	}
	// 完全无可用片段：保留原值但也告警（不静默吞掉）。
	d2 := &Doc{}
	got = enforceCleanTitle(d2, "》（")
	if len(d2.Warnings) == 0 {
		t.Error("无可用户片段时也应告警")
	}
	_ = got
}

// TestTitleFromFilenameOrphanCloser 是缺陷回归：书名里不许残留孤立闭括号。
func TestTitleFromFilenameOrphanCloser(t *testing.T) {
	names := []string{
		"【某书名》（1-99）》.txt",
		"某书名》.txt",
		"【某书名】）.txt",
		"某书名（上）》.txt",
		"「某书名」.txt",
		"〈某书名〉.txt",
	}
	for _, name := range names {
		title, _, _ := titleFromFilename(name)
		if hasBracket(title) {
			// 允许保留成对的括号（如「某书名（上）」），但不允许孤立括号。
			if trimOrphanClosers(title) != title {
				t.Errorf("%q 书名含孤立括号: %q", name, title)
			}
		}
	}
}

// ---------- 第 2 条：章节单元否定预查 ----------

// TestChapterUnitNegativeLookahead 是验收 ④ 的反例测试：
// 这些句子都不能被当成章节标题。
func TestChapterUnitNegativeLookahead(t *testing.T) {
	negatives := []string{
		"第二节课开始了。",
		"他三回头看了一下。",
		"前言不搭后语。",
		"这是第三集合的地方。",
		"第二部队已经开拔。",
		"第四篇章写得很散。",
		"正文开始了，故事由此展开。",
		"第二回合他赢了。",
		"第三节气得他直跺脚。",
		"这属于第二部门管辖。",
	}
	for _, line := range negatives {
		if h, ok := parseHeading(line); ok {
			t.Errorf("%q 被误判为标题: kind=%d marker=%q", line, h.kind, h.marker)
		}
	}
}

// TestChapterUnitPositiveStillWorks 确认预查没有把合法标题一起挡掉。
func TestChapterUnitPositiveStillWorks(t *testing.T) {
	positives := []string{
		"第一章 风起",
		"第2节 小结",
		"第三回 重逢",
		"第4集 新的开始",
		"序章",
		"前言",
		"番外 一",
	}
	for _, line := range positives {
		if _, ok := parseHeading(line); !ok {
			t.Errorf("%q 应被识别为标题", line)
		}
	}
}

// TestIsChapterUnitAt 直接覆盖否定预查表。
func TestIsChapterUnitAt(t *testing.T) {
	rejects := []string{"节课", "集合", "部队", "篇章", "回合", "话说"}
	for _, s := range rejects {
		if isChapterUnitAt(s, 0) {
			t.Errorf("%q 的单位字应被否定预查拒绝", s)
		}
	}
	accepts := []string{"章 标题", "节 小节", "回 归途", "话 说", "章"}
	for _, s := range accepts {
		if !isChapterUnitAt(s, 0) {
			t.Errorf("%q 应被识别为合法章单位", s)
		}
	}
}

// TestIsMarkerBoundary 覆盖裸标记词的边界判定。
func TestIsMarkerBoundary(t *testing.T) {
	good := []string{"", " 标题", "：说明", "第一章", "（上）", "12"}
	for _, s := range good {
		if !isMarkerBoundary(s) {
			t.Errorf("%q 应判为边界", s)
		}
	}
	bad := []string{"不搭后语", "开始了", "写得很散"}
	for _, s := range bad {
		if isMarkerBoundary(s) {
			t.Errorf("%q 不应判为边界", s)
		}
	}
}

// ---------- 第 3 条：count_sanity / 局部再切 ----------

// TestCountSanity 覆盖章数合理性的三档（偏少 / 正常 / 偏多）。
func TestCountSanity(t *testing.T) {
	// 15 章 / 4.5 万字：lo≈5.6 hi≈30 → 落在区间内，满分。
	if got := countSanity(15, 45000); got != 1.0 {
		t.Errorf("15 章 / 4.5 万字 = %v，期望 1.0", got)
	}
	// 1 章 / 4.5 万字：远低于 lo，分数应很低。
	if got := countSanity(1, 45000); got >= 0.5 {
		t.Errorf("1 章 / 4.5 万字 = %v，期望 < 0.5", got)
	}
	// 章数过多：应衰减。
	if got := countSanity(10000, 45000); got >= 1.0 {
		t.Errorf("10000 章 / 4.5 万字 = %v，期望衰减", got)
	}
	// 非法输入。
	if got := countSanity(0, 0); got != 0 {
		t.Errorf("零输入 = %v，期望 0", got)
	}
}

// TestFindOutliers 覆盖离群章判定。
func TestFindOutliers(t *testing.T) {
	// 15 个正常章 + 1 个巨块。
	chapters := make([]string, 0, 16)
	for i := 0; i < 15; i++ {
		chapters = append(chapters, strings.Repeat("正常长度的正文。", 100))
	}
	chapters = append(chapters, strings.Repeat("巨型块的正文。", 20000))
	idx := findOutliers(chapters)
	if len(idx) != 1 || idx[0] != 15 {
		t.Errorf("离群章 = %v，期望 [15]", idx)
	}
	// 长度均匀：没有离群章。
	even := []string{strings.Repeat("正文", 100), strings.Repeat("正文", 100), strings.Repeat("正文", 100)}
	if got := findOutliers(even); len(got) != 0 {
		t.Errorf("均匀章节不应有离群: %v", got)
	}
	if got := findOutliers(nil); got != nil {
		t.Errorf("空输入 = %v", got)
	}
}

// TestMedianOf 覆盖中位数计算。
func TestMedianOf(t *testing.T) {
	if got := medianOf([]int{3, 1, 2}); got != 2 {
		t.Errorf("中位数 = %d，期望 2", got)
	}
	if got := medianOf([]int{4, 1, 3, 2}); got != 2 {
		t.Errorf("偶数中位数 = %d，期望 2", got)
	}
	if got := medianOf(nil); got != 0 {
		t.Errorf("空输入 = %d", got)
	}
}

// TestRefineSplitKeepsNonOutliers 是验收 ⑤ 的核心测试：
// 15 个正常章 + 1 个藏了 20 个标题的巨块 → 巨块被切开，其余 15 章一个字节不动。
func TestRefineSplitKeepsNonOutliers(t *testing.T) {
	var b strings.Builder
	b.WriteString("某书名\n\n")
	before := make([]string, 0, 15)
	for i := 1; i <= 15; i++ {
		title := "第" + itoaCN(i) + "章 正常标题"
		body := "这是正常长度的正文段落，用于验证非离群章不会被改动。\n"
		b.WriteString(title + "\n" + body)
		before = append(before, title)
	}
	// 巨型章：内部藏着 20 个真章节标题。
	b.WriteString("第16章 巨型块\n")
	for k := 1; k <= 20; k++ {
		b.WriteString("第" + itoaCN(100+k) + "章 子标题" + itoaCN(k) + "\n")
		for j := 0; j < 60; j++ {
			b.WriteString("这是巨型块内部的正文内容，应该被局部再切分开处理。\n")
		}
	}
	book, err := New(DefaultOptions()).ParseBytes("某书.txt", []byte(b.String()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) <= 16 {
		t.Fatalf("巨块未被再切，章数 = %d", len(book.Chapters))
	}
	// 前 15 章标题必须原样保留。
	for i := 0; i < 15 && i < len(book.Chapters); i++ {
		if book.Chapters[i].Title != before[i] {
			t.Errorf("第 %d 章标题被改动: %q → %q", i+1, before[i], book.Chapters[i].Title)
		}
	}
	// 不应再有超长章。
	for _, c := range book.Chapters {
		if runeLen(c.Content) > maxOutlierChars {
			t.Errorf("仍有超长章: %q = %d 字符", c.Title, runeLen(c.Content))
		}
	}
}

// TestStructureScore 覆盖结构分：真标题密度高，正文引用密度低。
func TestStructureScore(t *testing.T) {
	realCands := make([]candidate, 0, 20)
	for i := 0; i < 20; i++ {
		realCands = append(realCands, candidate{h: heading{kind: headChapter, hasNum: true, strong: true, num: i + 1}})
	}
	if got := structureScore(realCands, 30000); got < outlierAcceptScore {
		t.Errorf("真标题结构分 = %v，期望 ≥ %v", got, outlierAcceptScore)
	}
	// 只有 2 个候选、内容却是 5 万字：密度极低，分数应偏低。
	sparse := []candidate{
		{h: heading{kind: headChapter, hasNum: true, strong: true}},
		{h: heading{kind: headChapter, hasNum: true, strong: true}},
	}
	if got := structureScore(sparse, 50000); got >= outlierAcceptScore {
		t.Errorf("稀疏候选结构分 = %v，期望 < %v", got, outlierAcceptScore)
	}
	if got := structureScore(nil, 1000); got != 0 {
		t.Errorf("空候选 = %v", got)
	}
}

// itoaCN 把小整数转成中文数字（供合成 fixture 用，覆盖到 120）。
func itoaCN(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n)
}

// itoa 是测试用的十进制转换。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
