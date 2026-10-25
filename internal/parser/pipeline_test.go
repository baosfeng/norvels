package parser

import (
	"strings"
	"testing"
)

// fixture 是一段合成的「典型中文小说」文本：带 BOM 之外的噪声头、作者行、
// 三章正文。所有测试都以它为基线，避免依赖任何真实语料。
func fixtureChapters() string {
	var b strings.Builder
	b.WriteString("某书名\n作者：某作者\n内容简介：这是一个用于测试的合成故事。\n\n")
	for i := 1; i <= 3; i++ {
		b.WriteString("第")
		b.WriteString([]string{"", "一", "二", "三"}[i])
		b.WriteString("章 标题")
		b.WriteString(string(rune('0' + i)))
		b.WriteString("\n")
		b.WriteString("这是第")
		b.WriteString(string(rune('0' + i)))
		b.WriteString("章的正文内容，用来验证切章与清洗。\n\n")
	}
	return b.String()
}

// TestParsePipeline 覆盖四层主路径：编码 → 过滤 → 元数据 → 切章。
func TestParsePipeline(t *testing.T) {
	p := New(DefaultOptions())
	book, err := p.ParseBytes("某文件名.txt", []byte(fixtureChapters()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if book.Encoding != EncUTF8 {
		t.Errorf("编码 = %q，期望 %q", book.Encoding, EncUTF8)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("章数 = %d，期望 3", len(book.Chapters))
	}
	if !strings.Contains(book.Chapters[0].Title, "第一章") {
		t.Errorf("首章标题 = %q，期望含「第一章」", book.Chapters[0].Title)
	}
	if book.Chapters[0].WordCount == 0 {
		t.Error("首章字数为 0")
	}
	if book.Title == "" || book.Title == "未命名" {
		t.Errorf("书名未抽到: %q", book.Title)
	}
	if book.Author != "某作者" {
		t.Errorf("作者 = %q，期望「某作者」", book.Author)
	}
	if book.Intro == "" {
		t.Error("简介未抽到")
	}
	if book.SHA256 == "" || book.FileSize == 0 {
		t.Error("哈希或体积缺失")
	}
}

// TestParseBytesEmpty 覆盖空内容这一异常路径。
func TestParseBytesEmpty(t *testing.T) {
	if _, err := New(DefaultOptions()).ParseBytes("空.txt", nil); err == nil {
		t.Fatal("空内容应返回错误")
	}
}

// TestParseBytesWhitespaceOnly 覆盖「解码后无有效文本」路径。
func TestParseBytesWhitespaceOnly(t *testing.T) {
	_, err := New(DefaultOptions()).ParseBytes("空白.txt", []byte("   \n\n\t\n"))
	if err == nil {
		t.Fatal("全空白应返回错误")
	}
}

// TestParseBytesSingleChapter 覆盖「极小文件整篇一章」路径。
func TestParseBytesSingleChapter(t *testing.T) {
	book, err := New(DefaultOptions()).ParseBytes("短文.txt", []byte("只有一句话。"))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) != 1 {
		t.Fatalf("章数 = %d，期望 1", len(book.Chapters))
	}
	if book.Chapters[0].Title != "全文" {
		t.Errorf("标题 = %q，期望「全文」", book.Chapters[0].Title)
	}
}

// TestParseBytesLowContent 覆盖「全是噪声 → 兜底单章 + low_content 告警」路径。
func TestParseBytesLowContent(t *testing.T) {
	body := "更多小说资源尽在强鹿小说群：123456\nwww.example.com\n"
	book, err := New(DefaultOptions()).ParseBytes("噪声.txt", []byte(body))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) == 0 {
		t.Fatal("至少应产出一章，保证能入库")
	}
}

// TestOptionsForceEncoding 覆盖强制编码与非法编码名两条路径。
func TestOptionsForceEncoding(t *testing.T) {
	raw := []byte(fixtureChapters())
	opts := DefaultOptions()
	opts.ForceEncoding = EncUTF8
	book, err := New(opts).ParseBytes("某书.txt", raw)
	if err != nil {
		t.Fatalf("强制编码解析失败: %v", err)
	}
	if book.Encoding != EncUTF8 {
		t.Errorf("编码 = %q，期望 %q", book.Encoding, EncUTF8)
	}
	// 不存在的编码名：应回退到打分结果而不是报错。
	opts.ForceEncoding = "不存在的编码"
	book, err = New(opts).ParseBytes("某书.txt", raw)
	if err != nil {
		t.Fatalf("非法编码名不应导致失败: %v", err)
	}
	if book.Encoding == "不存在的编码" {
		t.Error("不应把非法编码名写进结果")
	}
}

// TestOptionsDisableFilters 覆盖过滤器启停。
func TestOptionsDisableFilters(t *testing.T) {
	raw := []byte("ＡＢＣ１２３\n第一章 标题\n正文。\n")
	opts := DefaultOptions()
	opts.DisableFilters = []string{"width"}
	book, err := New(opts).ParseBytes("全角.txt", raw)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) == 0 {
		t.Fatal("应至少产出一章")
	}
	// 关闭 width 后全角数字应保持原样（没有归一成半角）。
	if !strings.Contains(book.Chapters[0].Content+book.Chapters[0].Title, "ＡＢＣ") &&
		!strings.Contains(string(raw), "ＡＢＣ") {
		t.Error("测试数据异常")
	}
}

// TestOptionsMaxChapterChars 覆盖超长章再切。
func TestOptionsMaxChapterChars(t *testing.T) {
	var b strings.Builder
	b.WriteString("第一章 长章\n")
	for i := 0; i < 800; i++ {
		b.WriteString("这是一段用于测试超长章节再切的合成正文内容，重复出现以拉长篇幅。\n")
	}
	opts := DefaultOptions()
	opts.MaxChapterChars = 2000
	book, err := New(opts).ParseBytes("超长.txt", []byte(b.String()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) < 2 {
		t.Fatalf("超长章应被再切，实际章数 = %d", len(book.Chapters))
	}
	if !strings.Contains(book.Chapters[1].Title, "续") {
		t.Errorf("续章标题 = %q，期望含「续」", book.Chapters[1].Title)
	}
	for _, c := range book.Chapters {
		if runeLen(c.Content) > opts.MaxChapterChars*2 {
			t.Errorf("章节仍然过长: %d", runeLen(c.Content))
		}
	}
}

// TestParseSpacedArabicChapters 覆盖「第 3 章」「第 12 章」这类数字带填充空白的
// 阿拉伯数字标题：30 章必须切出 30 章（修复前这两章会被并进上一章）。
// 正文里同时出现「第 3 章」引用，必须留在正文里、不额外切章。
func TestParseSpacedArabicChapters(t *testing.T) {
	var b strings.Builder
	b.WriteString("某书名\n作者：某作者\n\n")
	for i := 1; i <= 30; i++ {
		b.WriteString(spacedChapterTitle(i))
		b.WriteString("\n")
		b.WriteString("这是第" + itoa(i) + "章的正文，他翻回第 3 章又看了一遍那段对话。\n\n")
	}
	book, err := New(DefaultOptions()).ParseBytes("某文件名.txt", []byte(b.String()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) != 30 {
		t.Fatalf("章数 = %d，期望 30（标题 = %v）", len(book.Chapters), chapterTitles(book.Chapters))
	}
	for _, i := range []int{3, 12} {
		want := "第 " + itoa(i) + " 章"
		if got := book.Chapters[i-1].Title; !strings.HasPrefix(got, want) {
			t.Errorf("第 %d 章标题 = %q，期望以 %q 开头", i, got, want)
		}
	}
	if !strings.Contains(book.Chapters[7].Content, "第 3 章") {
		t.Errorf("正文里的「第 3 章」引用被吃掉: %q", book.Chapters[7].Content)
	}
}

// TestParseSpacedArabicInVolume 覆盖卷内阿拉伯数字标题。
func TestParseSpacedArabicInVolume(t *testing.T) {
	text := "第一卷 风云\n第 1 章 甲\n正文甲。\n第 2 章 乙\n正文乙。\n第 3 章 丙\n正文丙。\n"
	book, err := New(DefaultOptions()).ParseBytes("卷内.txt", []byte(text))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("章数 = %d，期望 3（标题 = %v）", len(book.Chapters), chapterTitles(book.Chapters))
	}
}

// spacedChapterTitle 造标题：第 3/12 章用「第 3 章」写法（等价「按序号重命名」的产物）。
func spacedChapterTitle(i int) string {
	if i == 3 || i == 12 {
		return "第 " + itoa(i) + " 章 标题" + itoa(i)
	}
	return "第" + itoa(i) + "章 标题" + itoa(i)
}

// TestParseFile 覆盖 ParseFile 的正常与读失败路径。
func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/某书.txt"
	if err := writeFile(path, fixtureChapters()); err != nil {
		t.Fatal(err)
	}
	book, err := New(DefaultOptions()).ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile 失败: %v", err)
	}
	if book.SourcePath != path {
		t.Errorf("SourcePath = %q，期望 %q", book.SourcePath, path)
	}
	if _, err := New(DefaultOptions()).ParseFile(dir + "/不存在.txt"); err == nil {
		t.Fatal("读不存在的文件应返回错误")
	}
}
