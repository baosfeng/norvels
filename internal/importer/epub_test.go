package importer

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"

	"norvels/internal/ebook"
	"norvels/internal/model"
	"norvels/internal/parser"
)

// 合成书目：书名/作者/系列/分类全部虚构——隐私红线要求测试里不出现真实书名。
const (
	epubContainerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`

	epubStandardOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>示例书简</dc:title>
    <dc:creator>示例作者</dc:creator>
    <dc:description>&lt;p&gt;一段合成的示例简介。&lt;/p&gt;</dc:description>
    <dc:language>zh-CN</dc:language>
    <dc:publisher>样例出版社</dc:publisher>
    <dc:identifier>urn:isbn:9780000000002</dc:identifier>
    <dc:date>2026-01-02</dc:date>
    <dc:subject>示例分类</dc:subject>
    <meta name="calibre:series" content="示例系列"/>
    <meta name="calibre:series_index" content="2"/>
  </metadata>
  <manifest>
    <item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="ch2.xhtml" media-type="application/xhtml+xml"/>
    <item id="cover" href="cover.jpg" media-type="image/jpeg" properties="cover-image"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/></spine>
</package>`

	epubCh1XHTML = `<html><head><title>第一章 起始</title></head>
<body><h1>第一章 起始</h1><p>第一章正文 &amp; 实体解码。</p><script>var x="跳过";</script></body></html>`

	epubCh2XHTML = `<html><head><title>第二章 收束</title></head>
<body><p>第二章正文。</p></body></html>`
)

// epubCoverBytes 是最小 JPEG 头尾：内容不重要，MIME 由清单声明。
var epubCoverBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0xFF, 0xD9}

// epubEntry 是合成 EPUB 的一个条目。
type epubEntry struct {
	name string
	data []byte
}

// epubBytes 在内存里合成 zip（测试内造包，不依赖外部样本）。
func epubBytes(t *testing.T, entries []epubEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("创建 zip 条目 %s: %v", e.name, err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatalf("写入 zip 条目 %s: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip: %v", err)
	}
	return buf.Bytes()
}

// standardEPUBBytes 是「两章 + 封面 + 完整元数据」的标准样本。
func standardEPUBBytes(t *testing.T) []byte {
	t.Helper()
	return epubBytes(t, []epubEntry{
		{name: "mimetype", data: []byte("application/epub+zip")},
		{name: "META-INF/container.xml", data: []byte(epubContainerXML)},
		{name: "OEBPS/content.opf", data: []byte(epubStandardOPF)},
		{name: "OEBPS/ch1.xhtml", data: []byte(epubCh1XHTML)},
		{name: "OEBPS/ch2.xhtml", data: []byte(epubCh2XHTML)},
		{name: "OEBPS/cover.jpg", data: epubCoverBytes},
	})
}

// parseEPUB 用默认解析器跑一次分发入口。
func parseEPUB(t *testing.T, name string, data []byte) (*model.ParsedBook, error) {
	t.Helper()
	return parseFile(parser.New(parser.DefaultOptions()), name, data)
}

// TestParseFileEpubMetadata 验证 EPUB 元数据搬进了 ParsedBook。
func TestParseFileEpubMetadata(t *testing.T) {
	book, err := parseEPUB(t, "示例书简.epub", standardEPUBBytes(t))
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	cases := []struct{ field, got, want string }{
		{"书名", book.Title, "示例书简"},
		{"作者", book.Author, "示例作者"},
		{"简介", book.Intro, "一段合成的示例简介。"},
		{"语言", book.Language, "zh-CN"},
		{"出版社", book.Publisher, "样例出版社"},
		{"书号", book.ISBN, "9780000000002"},
		{"出版日期", book.PubDate, "2026-01-02"},
		{"系列", book.Series, "示例系列"},
		{"来源格式", book.SourceFormat, FormatEPUB},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q，期望 %q", c.field, c.got, c.want)
		}
	}
	if book.SeriesIndex != 2 {
		t.Errorf("系列序号 = %d，期望 2", book.SeriesIndex)
	}
	if len(book.Subjects) != 1 || book.Subjects[0] != "示例分类" {
		t.Errorf("分类 = %v，期望 [示例分类]", book.Subjects)
	}
}

// TestParseFileEpubCover 验证封面字节与 MIME 原样搬运。
func TestParseFileEpubCover(t *testing.T) {
	book, err := parseEPUB(t, "示例书简.epub", standardEPUBBytes(t))
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if book.Cover == nil {
		t.Fatal("封面为 nil，期望从 cover-image 取到")
	}
	if book.Cover.MIME != "image/jpeg" || !bytes.Equal(book.Cover.Bytes, epubCoverBytes) {
		t.Errorf("封面 = %+v，期望原样搬运 jpeg 字节", book.Cover)
	}
}

// TestParseFileEpubChapters 验证章节标题、序号与字数转换。
func TestParseFileEpubChapters(t *testing.T) {
	book, err := parseEPUB(t, "示例书简.epub", standardEPUBBytes(t))
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(book.Chapters) != 2 {
		t.Fatalf("章节数 = %d，期望 2", len(book.Chapters))
	}
	first := book.Chapters[0]
	if first.Title != "第一章 起始" || book.Chapters[1].Title != "第二章 收束" {
		t.Errorf("章节标题 = %q / %q", first.Title, book.Chapters[1].Title)
	}
	if strings.Contains(first.Content, "跳过") {
		t.Errorf("脚本内容不应进正文: %q", first.Content)
	}
	if first.Idx != 0 || first.VolumeIdx != -1 {
		t.Errorf("章节序号/卷下标 = %d/%d，期望 0/-1", first.Idx, first.VolumeIdx)
	}
	if book.WordCount <= 0 || book.WordCount != first.WordCount+book.Chapters[1].WordCount {
		t.Errorf("全文字数 = %d，期望等于各章之和", book.WordCount)
	}
}

// TestParseFileEpubNoCover 没有封面线索时 Cover 保持 nil。
func TestParseFileEpubNoCover(t *testing.T) {
	opf := strings.Replace(epubStandardOPF, `<item id="cover" href="cover.jpg" media-type="image/jpeg" properties="cover-image"/>`, "", 1)
	data := epubBytes(t, []epubEntry{
		{name: "META-INF/container.xml", data: []byte(epubContainerXML)},
		{name: "OEBPS/content.opf", data: []byte(opf)},
		{name: "OEBPS/ch1.xhtml", data: []byte(epubCh1XHTML)},
		{name: "OEBPS/ch2.xhtml", data: []byte(epubCh2XHTML)},
	})
	book, err := parseEPUB(t, "示例书简.epub", data)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if book.Cover != nil {
		t.Errorf("Cover 应为 nil，实际 %+v", book.Cover)
	}
}

// TestParseFileEpubEncrypted 加密 EPUB 的错误要能被 errors.Is 识别（上传接口据此回 422）。
func TestParseFileEpubEncrypted(t *testing.T) {
	data := epubBytes(t, []epubEntry{
		{name: "META-INF/container.xml", data: []byte(epubContainerXML)},
		{name: "META-INF/encryption.xml", data: []byte(`<encryption><EncryptedData/></encryption>`)},
		{name: "OEBPS/content.opf", data: []byte(epubStandardOPF)},
		{name: "OEBPS/ch1.xhtml", data: []byte(epubCh1XHTML)},
	})
	if _, err := parseEPUB(t, "加密示例.epub", data); !errors.Is(err, ebook.ErrEncrypted) {
		t.Fatalf("err = %v，期望包装 ebook.ErrEncrypted", err)
	}
}

// TestParseFileEpubBadZip 坏 zip 报错而不是 panic，也不产出半本书。
func TestParseFileEpubBadZip(t *testing.T) {
	book, err := parseEPUB(t, "坏文件.epub", []byte("这不是 zip"))
	if err == nil || book != nil {
		t.Fatalf("book=%v err=%v，期望解析失败", book, err)
	}
}

// TestParseFileEpubNeedsSplit spine 只有一个文档的大书交给 txt 解析器切章。
func TestParseFileEpubNeedsSplit(t *testing.T) {
	para := "<p>" + strings.Repeat("示例段落", 175) + "</p>"
	chapter := `<html><head><title>整本一页</title></head><body>` + strings.Repeat(para, 30) + `</body></html>`
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>示例长书</dc:title><dc:creator>示例作者</dc:creator></metadata>
  <manifest><item id="c1" href="book.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="c1"/></spine>
</package>`
	data := epubBytes(t, []epubEntry{
		{name: "META-INF/container.xml", data: []byte(epubContainerXML)},
		{name: "OEBPS/content.opf", data: []byte(opf)},
		{name: "OEBPS/book.xhtml", data: []byte(chapter)},
	})
	book, err := parseEPUB(t, "示例长书.epub", data)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(book.Chapters) < 2 {
		t.Fatalf("章节数 = %d，期望整本一页被重新切章", len(book.Chapters))
	}
	if !hasWarning(book.Warnings, "切章策略") {
		t.Errorf("告警 = %v，期望提示已按 txt 切章策略重新分章", book.Warnings)
	}
}

// TestParseFileTxtUnchanged txt 仍走原有 parser（回归：分发不能改坏 txt 路径）。
func TestParseFileTxtUnchanged(t *testing.T) {
	book, err := parseEPUB(t, "示例小说.txt", []byte(sampleText("单本小说", "作者甲")))
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	// 书名由文件名推断（parser 的既有口径），作者来自正文头部。
	if book.Title != "示例小说" || book.Author != "作者甲" {
		t.Errorf("书名/作者 = %q / %q", book.Title, book.Author)
	}
	if len(book.Chapters) != 3 || book.SourceFormat != "" {
		t.Errorf("章节数/来源格式 = %d / %q", len(book.Chapters), book.SourceFormat)
	}
}

// hasWarning 判断告警里是否出现过某个片段。
func hasWarning(warnings []string, want string) bool {
	for _, w := range warnings {
		if strings.Contains(w, want) {
			return true
		}
	}
	return false
}
