package ebook

import (
	"bytes"
	"strings"
	"testing"
)

func TestOpenMetadata(t *testing.T) {
	book := mustOpen(t, "示例书简.epub", standardEPUB(t))

	cases := []struct{ field, got, want string }{
		{"书名", book.Title, "示例书简"},
		{"作者", book.Author, "示例作者"},
		{"简介", book.Intro, "一段合成的示例简介。"},
		{"语言", book.Language, "zh-CN"},
		{"出版社", book.Publisher, "样例出版社"},
		{"书号", book.ISBN, "9780000000002"},
		{"出版日期", book.PubDate, "2026-01-02"},
		{"系列", book.Series, "示例系列"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q，期望 %q", c.field, c.got, c.want)
		}
	}
}

func TestOpenSeriesAndSubjects(t *testing.T) {
	book := mustOpen(t, "示例书简.epub", standardEPUB(t))
	if book.SeriesIndex != 3 {
		t.Errorf("系列序号 = %d，期望 3", book.SeriesIndex)
	}
	if len(book.Subjects) != 1 || book.Subjects[0] != "示例分类" {
		t.Errorf("分类 = %v，期望去空去重后的 [示例分类]", book.Subjects)
	}
}

func TestOpenCoverAndChapters(t *testing.T) {
	book := mustOpen(t, "示例书简.epub", standardEPUB(t))
	if book.Cover == nil {
		t.Fatal("封面为 nil，期望从 cover-image 取到")
	}
	if book.Cover.MIME != "image/jpeg" {
		t.Errorf("封面 MIME = %q，期望 image/jpeg", book.Cover.MIME)
	}
	if !bytes.Equal(book.Cover.Bytes, testCoverBytes) {
		t.Error("封面字节与清单条目不一致")
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("章节数 = %d，期望 3", len(book.Chapters))
	}
	if book.NeedsSplit() {
		t.Error("三章的小书不该走全文切章")
	}
	if len(book.Warnings) != 0 {
		t.Errorf("标准样本不该有告警: %v", book.Warnings)
	}
}

func TestOpenChapterTitles(t *testing.T) {
	book := mustOpen(t, "示例书简.epub", standardEPUB(t))
	want := []string{"第一章 起始", "第二章 标题", "ch3"}
	if len(book.Chapters) != len(want) {
		t.Fatalf("章节数 = %d，期望 %d", len(book.Chapters), len(want))
	}
	for i, w := range want {
		if got := book.Chapters[i].Title; got != w {
			t.Errorf("第 %d 章标题 = %q，期望 %q", i+1, got, w)
		}
	}
}

func TestOpenChapterContent(t *testing.T) {
	book := mustOpen(t, "示例书简.epub", standardEPUB(t))
	want := "第一段 & 实体解码。\n第二段\n换行后的内容。\n汉字保留本体。"
	if got := book.Chapters[0].Content; got != want {
		t.Errorf("第一章正文 = %q，期望 %q", got, want)
	}
	for _, banned := range []string{"脚本内容", "不该用的文档标题", "color:red"} {
		if strings.Contains(book.Chapters[0].Content, banned) {
			t.Errorf("正文不应包含 %q", banned)
		}
	}
	if got := book.Chapters[2].Content; got != "第三章正文。" {
		t.Errorf("第三章正文 = %q", got)
	}
}

func TestOpenNoCover(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title>`,
		`<item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>`,
		`<itemref idref="c1"/>`, "")
	data := buildEPUB(t, opf, zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)})
	book := mustOpen(t, "示例书简.epub", data)
	if book.Cover != nil {
		t.Errorf("没有封面线索时 Cover 应为 nil，实际 %+v", book.Cover)
	}
}

func TestOpenCoverByMeta(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title><meta name="cover" content="img"/>`,
		`<item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
		 <item id="img" href="cover.png" media-type="image/png"/>`,
		`<itemref idref="c1"/>`, "")
	data := buildEPUB(t, opf,
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
		zipEntry{name: "OEBPS/cover.png", data: []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}},
	)
	book := mustOpen(t, "示例书简.epub", data)
	if book.Cover == nil || book.Cover.MIME != "image/png" {
		t.Fatalf("meta name=cover 未能取到封面: %+v", book.Cover)
	}
}

func TestOpenCoverByGuide(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title>`,
		`<item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
		 <item id="page" href="cover.xhtml" media-type="application/xhtml+xml"/>`,
		`<itemref idref="c1"/>`,
		`<guide><reference type="cover" href="cover.xhtml"/></guide>`)
	page := `<html><body><img src="../images/pic.png"/></body></html>`
	data := buildEPUB(t, opf,
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
		zipEntry{name: "OEBPS/cover.xhtml", data: []byte(page)},
		zipEntry{name: "images/pic.png", data: []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}},
	)
	book := mustOpen(t, "示例书简.epub", data)
	if book.Cover == nil || book.Cover.MIME != "image/png" {
		t.Fatalf("guide cover 页里的图片未能作为封面: %+v", book.Cover)
	}
}

func TestOpenCoverByFilename(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title>`,
		`<item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>`,
		`<itemref idref="c1"/>`, "")
	data := buildEPUB(t, opf,
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
		zipEntry{name: "OEBPS/cover.webp", data: []byte("RIFF0000WEBPVP8 ")},
	)
	book := mustOpen(t, "示例书简.epub", data)
	if book.Cover == nil || book.Cover.MIME != "image/webp" {
		t.Fatalf("文件名 cover.* 兜底失败: %+v", book.Cover)
	}
}

func TestOpenCoverTooLarge(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title>`,
		`<item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>`,
		`<itemref idref="c1"/>`, "")
	big := make([]byte, maxCoverBytes+1)
	data := buildEPUB(t, opf,
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
		zipEntry{name: "OEBPS/cover.jpg", data: big},
	)
	book := mustOpen(t, "示例书简.epub", data)
	if book.Cover != nil {
		t.Errorf("超过 2 MiB 的封面应被丢弃")
	}
	if len(book.Warnings) == 0 {
		t.Error("丢弃封面时应记 warning")
	}
}

func TestOpenCoverUnsupportedMIME(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title>`,
		`<item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
		 <item id="img" href="cover.tiff" media-type="image/tiff" properties="cover-image"/>`,
		`<itemref idref="c1"/>`, "")
	data := buildEPUB(t, opf,
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
		zipEntry{name: "OEBPS/cover.tiff", data: []byte{'I', 'I', 0x2A, 0x00}},
	)
	book := mustOpen(t, "示例书简.epub", data)
	if book.Cover != nil {
		t.Errorf("白名单外的封面格式应被丢弃: %+v", book.Cover)
	}
	if len(book.Warnings) == 0 {
		t.Error("丢弃不支持的封面格式时应记 warning")
	}
}

func TestOpenCreatorOnlyFirst(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title><dc:creator>示例作者</dc:creator><dc:creator>另一位示例作者</dc:creator>`,
		`<item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>`,
		`<itemref idref="c1"/>`, "")
	data := buildEPUB(t, opf, zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)})
	book := mustOpen(t, "示例书简.epub", data)
	if book.Author != "示例作者" {
		t.Errorf("作者 = %q，期望只保留第一个", book.Author)
	}
	if len(book.Warnings) == 0 {
		t.Error("多作者时应记 warning")
	}
}

func TestOpenTitleFromFilename(t *testing.T) {
	opf := testOPFText("",
		`<item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>`,
		`<itemref idref="c1"/>`, "")
	data := buildEPUB(t, opf, zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)})
	book := mustOpen(t, "示例书简 - 示例作者.epub", data)
	if book.Title != "示例书简" || book.Author != "示例作者" {
		t.Errorf("文件名兜底 = %q / %q", book.Title, book.Author)
	}
}

func TestPickISBN(t *testing.T) {
	cases := []struct {
		name string
		ids  []opfIdentifier
		want string
	}{
		{"opf:scheme", []opfIdentifier{{Scheme: "ISBN", Value: "0 000000 000000"}}, "0000000000000"},
		{"urn 前缀", []opfIdentifier{{Value: "urn:isbn:9780000000002"}}, "9780000000002"},
		{"裸 13 位", []opfIdentifier{{Value: "示例 9780000000002"}}, "9780000000002"},
		{"非书号忽略", []opfIdentifier{{Value: "urn:uuid:1234"}}, ""},
	}
	for _, c := range cases {
		if got := pickISBN(c.ids); got != c.want {
			t.Errorf("%s: pickISBN = %q，期望 %q", c.name, got, c.want)
		}
	}
}
