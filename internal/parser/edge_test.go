package parser

import (
	"strings"
	"testing"

	"norvels/internal/model"
)

// TestDecodeBytesBranches 覆盖旧解码入口的各分支（含 UTF-16 无 BOM、UTF-32 BOM、乱码兜底）。
func TestDecodeBytesBranches(t *testing.T) {
	// UTF-16LE 带 BOM。
	le := append([]byte{0xFF, 0xFE}, encodeUTF16("中文内容 abcd", true)...)
	if enc, text, _ := decodeBytes(le); enc != EncUTF16LE || text == "" {
		t.Errorf("UTF-16LE = %q %q", enc, text)
	}
	// UTF-16BE 带 BOM。
	be := append([]byte{0xFE, 0xFF}, encodeUTF16("中文内容 abcd", false)...)
	if enc, text, _ := decodeBytes(be); enc != EncUTF16BE || text == "" {
		t.Errorf("UTF-16BE = %q %q", enc, text)
	}
	// UTF-32 BOM：按 UTF-16LE 尽力解码并告警。
	u32 := append([]byte{0xFF, 0xFE, 0x00, 0x00}, encodeUTF16("abcd 中文", true)...)
	if enc, _, warns := decodeBytes(u32); enc != EncUTF16LE || len(warns) == 0 {
		t.Errorf("UTF-32 BOM = %q warns=%v", enc, warns)
	}
	// 无 BOM 的 UTF-16：方向靠 NUL 分布判定，断言「识别为 UTF-16 家族」即可。
	noBOM := encodeUTF16("中文内容 abcd efgh", true)
	if enc, _, warns := decodeBytes(noBOM); (enc != EncUTF16LE && enc != EncUTF16BE) || len(warns) == 0 {
		t.Errorf("无 BOM UTF-16 = %q warns=%v", enc, warns)
	}
	// 空输入。
	if enc, text, _ := decodeBytes(nil); enc != EncUTF8 || text != "" {
		t.Errorf("空输入 = %q %q", enc, text)
	}
	// 任意字节都不应 panic，且必须给出一个非空的编码判定。
	for _, junk := range [][]byte{
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		{0x80, 0x81, 0x82, 0x00, 0xFE},
		{0x1B, 0x5B, 0x30, 0x6D},
	} {
		if enc, _, _ := decodeBytes(junk); enc == "" {
			t.Errorf("非法字节 %v 未给出编码判定", junk)
		}
	}
}

// TestDecodeIntoLossy 覆盖「非法字节 → 尽力解码」这条路径。
func TestDecodeIntoLossy(t *testing.T) {
	// 合法 GBK 前缀 + 非法尾字节，迫使 GB18030 解码器走 lossy 分支。
	raw := []byte{0xD6, 0xD0, 0xCE, 0xC4, 0xFF}
	text, ok := decodeGB18030(raw)
	if ok {
		t.Logf("该输入被判定为完全合法，跳过 lossy 断言")
	}
	if text == "" {
		t.Error("lossy 解码不应返回空")
	}
	// Big5 的 lossy 分支同样要有输出。
	if bt, _ := decodeBig5([]byte{0xA4, 0xA4, 0xFF}); bt == "" {
		t.Error("Big5 lossy 不应返回空")
	}
}

// TestSampleRegions 覆盖小文件与大文件两种采样路径。
func TestSampleRegions(t *testing.T) {
	small := sampleRegions(1000)
	if len(small) != 1 || small[0][1] != 1000 {
		t.Errorf("小文件采样 = %v", small)
	}
	big := sampleRegions(4 << 20)
	if len(big) < 3 {
		t.Errorf("大文件采样点 = %d，期望 ≥ 3", len(big))
	}
	for _, r := range big {
		if r[0] < 0 || r[1] > 4<<20 || r[0] >= r[1] {
			t.Errorf("采样区间非法: %v", r)
		}
	}
}

// TestCleanerInlineAndObfuscation 覆盖行内广告剥离与混淆水印判定。
func TestCleanerInlineAndObfuscation(t *testing.T) {
	c := newFullCleaner()
	// stripInline 只跑「本文档真正启用过」的规则；正文里带 http 时 cleaner 才会挂上该规则。
	const withURL = "正文内容 http://example.com/path 结尾"
	c2 := newCleaner(withURL)
	if got := c2.stripInline(withURL); strings.Contains(got, "example.com") {
		t.Errorf("网址未剥离: %q", got)
	}
	if got := c.stripInline("<p>正文</p>"); strings.Contains(got, "<p>") {
		t.Errorf("HTML 标签未剥离: %q", got)
	}
	// 未启用对应规则的 cleaner 不应误删正常文本。
	if got := c.stripInline("正文内容 www.example.com 结尾"); got == "" {
		t.Error("不应把整行删空")
	}
	if !c.isNoiseLine("本作品来自互联网，仅供预览") {
		t.Error("版权行应判为噪声")
	}
	if !c.isNoiseLine("=====") {
		t.Error("分隔线应判为噪声")
	}
	if c.isNoiseLine("这是一段正常正文内容，不应被删。") {
		t.Error("正常正文被误判为噪声")
	}
	if !looksObfuscated(strings.Repeat(";'", 8)) {
		t.Error("混淆水印未被识别")
	}
	if looksObfuscated("正常的一句话") {
		t.Error("正常文本被判为混淆")
	}
}

// TestScanEmbedded 覆盖「整篇几乎没有换行」时的内嵌标题扫描。
func TestScanEmbedded(t *testing.T) {
	// 触发条件：单行 > 400 字节，且章节标记紧贴行尾（这样标记之后的切片足够短，
	// 才可能通过「标题行不超过 40 字」的判定）。
	var b strings.Builder
	for i := 1; i <= 3; i++ {
		b.WriteString(strings.Repeat("很长的正文段落", 60))
		b.WriteString("第" + string(rune('0'+i)) + "章 标题\n")
	}
	out := scanEmbedded(b.String())
	if len(out) == 0 {
		t.Error("超长行内的章节标记应被扫描到")
	}
	// 短行文件不应触发内嵌扫描。
	if got := scanEmbedded("第一章 标题\n正文\n"); len(got) != 0 {
		t.Errorf("短行不应触发内嵌扫描: %v", got)
	}
}

// TestPostProcessMerging 覆盖空章清理、短章合并与卷裁剪。
func TestPostProcessMerging(t *testing.T) {
	res := splitResult{
		volumes: []model.ParsedVolume{{Idx: 0, Title: "第一卷"}, {Idx: 1, Title: "空卷"}},
		chapters: []model.ParsedChapter{
			{Title: "第一章", Content: "正文一", VolumeIdx: 0},
			{Title: "空章", Content: "", VolumeIdx: 0},
			{Title: "第二章", Content: "短", VolumeIdx: 1},
		},
	}
	postProcess(&res, 100)
	if len(res.chapters) != 1 {
		t.Fatalf("章数 = %d，期望 1（空章删、短章并）", len(res.chapters))
	}
	if !strings.Contains(res.chapters[0].Content, "短") {
		t.Errorf("短章内容未并入: %q", res.chapters[0].Content)
	}

	// 全部为空：应清空卷与章。
	res = splitResult{chapters: []model.ParsedChapter{{Content: ""}}}
	postProcess(&res, 0)
	if res.chapters != nil || res.volumes != nil {
		t.Errorf("全空时应清空: %+v", res)
	}
}

// TestTrimLineRunes 覆盖 trimLine 的全角/不间断空格处理。
func TestTrimLineRunes(t *testing.T) {
	cases := map[string]string{
		"  普通  ":         "普通",
		"\u3000全角\u3000": "全角",
		"\u00a0空格\u00a0": "空格",
		"\t制表\t":         "制表",
	}
	for in, want := range cases {
		if got := trimLine(in); got != want {
			t.Errorf("trimLine(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestIsCJKScripts 覆盖 isCJK 的各文字系统分支。
func TestIsCJKScripts(t *testing.T) {
	cases := map[rune]bool{
		'中': true, 'あ': true, 'ア': true, '한': true, 'A': false, '1': false, '，': false,
	}
	for r, want := range cases {
		if got := isCJK(r); got != want {
			t.Errorf("isCJK(%q) = %v，期望 %v", r, got, want)
		}
	}
	if got := countWords("中文 hello 123"); got != 4 {
		t.Errorf("countWords = %d，期望 4", got)
	}
}

// TestTrimUnclosedBracket 覆盖「未闭合开括号」的截断保护。
func TestTrimUnclosedBracket(t *testing.T) {
	cases := map[string]string{
		"某书名（1-99）【标签": "某书名（1-99）",
		"某书名【标签，标签":    "某书名",
		"某书名（副标题）":     "某书名（副标题）",
		"某书名":          "某书名",
	}
	for in, want := range cases {
		if got := trimUnclosedBracket(in); got != want {
			t.Errorf("trimUnclosedBracket(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestAuthorFromEdges 覆盖两端署名扫描的命中与过滤路径。
func TestAuthorFromEdges(t *testing.T) {
	// 头部短行署名。
	head := "【某书名】作者：某作者\n正文开始\n"
	if got := authorFromEdges(head); got != "某作者" {
		t.Errorf("头部署名 = %q", got)
	}
	// 括号内署名（与标题同行）。
	inline := "某书名【作者：某作者】 第一章\n正文\n第二章\n正文\n"
	if got := authorFromEdges(inline); got != "某作者" {
		t.Errorf("括号署名 = %q", got)
	}
	// 文末署名。
	tail := "正文\n正文\n作者：某作者\n"
	if got := authorFromEdges(tail); got != "某作者" {
		t.Errorf("文末署名 = %q", got)
	}
	// 正文里提到「作者」但句子很长：不应误取。
	noise := "他的推广渠道就一个B站作者号，平时都自称某某。\n"
	if got := authorFromEdges(noise); got != "" {
		t.Errorf("正文误取作者 = %q", got)
	}
	// 空文本。
	if got := authorFromEdges(""); got != "" {
		t.Errorf("空文本 = %q", got)
	}
}

// TestIsPlausibleAuthor 覆盖作者名的可信度过滤。
func TestIsPlausibleAuthor(t *testing.T) {
	good := []string{"某作者", "某作家", "abc"}
	for _, n := range good {
		if !isPlausibleAuthor(n) {
			t.Errorf("%q 应判为可信", n)
		}
	}
	bad := []string{"", "全本", "搜书吧", "123", "含句号的句子。", "带：冒号"}
	for _, n := range bad {
		if isPlausibleAuthor(n) {
			t.Errorf("%q 不应判为可信", n)
		}
	}
}
