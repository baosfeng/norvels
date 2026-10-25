package ebook

import (
	"errors"
	"strings"
	"testing"
)

// TestOpenEncrypted 加密 EPUB 必须返回 ErrEncrypted（上传接口据此回 422）。
func TestOpenEncrypted(t *testing.T) {
	enc := []byte(`<?xml version="1.0"?>
<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <EncryptedData/>
</encryption>`)
	data := buildEPUB(t, testOPF,
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
		zipEntry{name: "META-INF/encryption.xml", data: enc},
	)
	err := openErr(t, "示例书简.epub", data)
	if !errors.Is(err, ErrEncrypted) {
		t.Fatalf("错误 = %v，期望包装 ErrEncrypted", err)
	}
}

// TestOpenEmptyEncryption 空的 encryption.xml 不算加密。
func TestOpenEmptyEncryption(t *testing.T) {
	data := buildEPUB(t, testOPF,
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
		zipEntry{name: "META-INF/encryption.xml", data: []byte("  \n ")},
	)
	book := mustOpen(t, "示例书简.epub", data)
	if len(book.Chapters) != 1 {
		t.Fatalf("章节数 = %d，期望 1", len(book.Chapters))
	}
}

// TestOpenBadZip 非 zip / 截断字节必须报错且不 panic。
func TestOpenBadZip(t *testing.T) {
	openErr(t, "坏文件.epub", []byte("这不是一个 zip 文件"))
	full := standardEPUB(t)
	openErr(t, "截断.epub", full[:len(full)/2])
}

// TestOpenTooManyEntries 条目数超过 2000 直接拒绝（不解压任何内容）。
func TestOpenTooManyEntries(t *testing.T) {
	entries := make([]zipEntry, 0, maxEntries+1)
	for i := 0; i <= maxEntries; i++ {
		entries = append(entries, zipEntry{name: "filler/" + itoa(i), data: []byte("x")})
	}
	openErr(t, "炸弹.epub", toZip(t, entries))
}

// TestOpenEntryTooLarge 单条目解压后超过 16 MiB 直接拒绝。
func TestOpenEntryTooLarge(t *testing.T) {
	data := buildEPUB(t, testOPF)
	huge := make([]byte, maxEntryBytes+1)
	data = toZip(t, []zipEntry{
		{name: "META-INF/container.xml", data: huge},
		{name: "OEBPS/content.opf", data: []byte(testOPF)},
	})
	err := openErr(t, "炸弹.epub", data)
	if !strings.Contains(err.Error(), "上限") {
		t.Errorf("错误 = %v，期望提到上限", err)
	}
}

// TestArchiveTotalBudget 解压总量用满后必须拒绝继续解压。
func TestArchiveTotalBudget(t *testing.T) {
	a, err := openArchive(standardEPUB(t))
	if err != nil {
		t.Fatalf("openArchive: %v", err)
	}
	a.total = maxTotalBytes
	if _, _, err := a.read("OEBPS/ch1.xhtml", maxEntryBytes); err == nil {
		t.Fatal("解压总量超限时应报错")
	}
}

// TestOpenNeedsSplit 单 XHTML 的大书需要交给 txt 解析器切章。
func TestOpenNeedsSplit(t *testing.T) {
	longText := strings.Repeat("示例正文", 6000)
	opf := testOPFText(`<dc:title>示例长书</dc:title>`,
		`<item id="c1" href="book.xhtml" media-type="application/xhtml+xml"/>`,
		`<itemref idref="c1"/>`, "")
	chapter := `<html><head><title>整本一页</title></head><body><p>` + longText + `</p></body></html>`
	data := buildEPUB(t, opf, zipEntry{name: "OEBPS/book.xhtml", data: []byte(chapter)})
	book := mustOpen(t, "示例长书.epub", data)
	if len(book.Chapters) != 1 {
		t.Fatalf("章节数 = %d，期望 1", len(book.Chapters))
	}
	if !book.NeedsSplit() {
		t.Fatalf("spine=1 且 %d 字的书应标记为需要切章", len(book.FullText()))
	}
	if len(book.FullText()) < len(longText) {
		t.Errorf("全文长度 %d，短于原文 %d", len(book.FullText()), len(longText))
	}
}

// TestOpenNoChapters spine 为空时明确报错，避免入库一本空书。
func TestOpenNoChapters(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title>`, "", "", "")
	openErr(t, "示例书简.epub", buildEPUB(t, opf))
}

// TestOpenMissingContainer container.xml 缺失时退化为第一个 .opf。
func TestOpenMissingContainer(t *testing.T) {
	data := toZip(t, []zipEntry{
		{name: "OEBPS/content.opf", data: []byte(testOPF)},
		{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
		{name: "OEBPS/ch2.xhtml", data: []byte(testCh2XHTML)},
		{name: "OEBPS/ch3.xhtml", data: []byte(testCh3XHTML)},
	})
	book := mustOpen(t, "示例书简.epub", data)
	if book.Title != "示例书简" || len(book.Chapters) != 3 {
		t.Fatalf("退化路径解析失败: title=%q chapters=%d", book.Title, len(book.Chapters))
	}
}

// TestOpenSkipsBrokenSpineRef spine 引用了缺失或非 XHTML 的项时跳过并告警。
func TestOpenSkipsBrokenSpineRef(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title>`,
		`<item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
		 <item id="img" href="pic.jpg" media-type="image/jpeg"/>`,
		`<itemref idref="c1"/><itemref idref="missing"/><itemref idref="img"/>`, "")
	data := buildEPUB(t, opf,
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
		zipEntry{name: "OEBPS/pic.jpg", data: testCoverBytes},
	)
	book := mustOpen(t, "示例书简.epub", data)
	if len(book.Chapters) != 1 {
		t.Fatalf("章节数 = %d，期望只保留 1 章", len(book.Chapters))
	}
	if len(book.Warnings) < 2 {
		t.Errorf("告警 = %v，期望覆盖缺失项与非 XHTML 项", book.Warnings)
	}
}

// TestOpenSkipsEmptyChapter 只有图片的封面页不产生空章节。
func TestOpenSkipsEmptyChapter(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title>`,
		`<item id="page" href="cover.xhtml" media-type="application/xhtml+xml"/>
		 <item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>`,
		`<itemref idref="page"/><itemref idref="c1"/>`, "")
	data := buildEPUB(t, opf,
		zipEntry{name: "OEBPS/cover.xhtml", data: []byte(`<html><body><img src="pic.jpg"/></body></html>`)},
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
	)
	book := mustOpen(t, "示例书简.epub", data)
	if len(book.Chapters) != 1 || book.Chapters[0].Title != "第一章 起始" {
		t.Fatalf("章节 = %+v，期望只有第一章", book.Chapters)
	}
}

// TestOpenSkipsNonLinearSpine linear="no" 的目录/注释页不算章节。
func TestOpenSkipsNonLinearSpine(t *testing.T) {
	opf := testOPFText(`<dc:title>示例书简</dc:title>`,
		`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml"/>
		 <item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>`,
		`<itemref idref="nav" linear="no"/><itemref idref="c1"/>`, "")
	data := buildEPUB(t, opf,
		zipEntry{name: "OEBPS/nav.xhtml", data: []byte(`<html><body><h1>目录</h1><p>第一章</p></body></html>`)},
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
	)
	book := mustOpen(t, "示例书简.epub", data)
	if len(book.Chapters) != 1 || book.Chapters[0].Title != "第一章 起始" {
		t.Fatalf("章节 = %+v，期望跳过目录页", book.Chapters)
	}
}

// itoa 是测试内的小整数转字符串（避免为造条目名引入 strconv 依赖噪音）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
