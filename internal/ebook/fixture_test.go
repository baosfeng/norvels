package ebook

import (
	"archive/zip"
	"bytes"
	"testing"
)

// 合成书目：书名/作者/系列/分类全部虚构——隐私红线要求测试里不出现真实书名。
const (
	testContainerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	testOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>示例书简</dc:title>
    <dc:creator>示例作者</dc:creator>
    <dc:description>&lt;p&gt;一段合成的示例简介。&lt;/p&gt;</dc:description>
    <dc:language>zh-CN</dc:language>
    <dc:publisher>样例出版社</dc:publisher>
    <dc:identifier id="uid">urn:isbn:9780000000002</dc:identifier>
    <dc:date>2026-01-02</dc:date>
    <dc:subject>示例分类</dc:subject>
    <dc:subject>示例分类</dc:subject>
    <dc:subject>  </dc:subject>
    <meta name="calibre:series" content="示例系列"/>
    <meta name="calibre:series_index" content="3"/>
  </metadata>
  <manifest>
    <item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="ch2.xhtml" media-type="application/xhtml+xml"/>
    <item id="c3" href="ch3.xhtml" media-type="application/xhtml+xml"/>
    <item id="cover" href="cover.jpg" media-type="image/jpeg" properties="cover-image"/>
  </manifest>
  <spine>
    <itemref idref="c1"/>
    <itemref idref="c2"/>
    <itemref idref="c3"/>
  </spine>
</package>`

	// ch1 同时有 h1 与 <title>：验证标题优先取 h1；正文覆盖实体解码、脚本跳过、
	// br 断段、ruby 保留本体。
	testCh1XHTML = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>不该用的文档标题</title><style>p{color:red}</style></head>
<body>
  <h1>第一章 起始</h1>
  <p>第一段 &amp; 实体解码。</p>
  <script>var hidden = "脚本内容";</script>
  <p>第二段<br/>换行后的内容。</p>
  <p><ruby>汉<rt>hàn</rt></ruby>字保留本体。</p>
</body>
</html>`

	// ch2 没有 h1/h2/h3：标题回落 <title>。
	testCh2XHTML = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>第二章 标题</title></head>
<body><p>第二章正文。</p></body>
</html>`

	// ch3 既无标题元素也无 <title>：标题回落文件名。
	testCh3XHTML = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head></head>
<body><p>第三章正文。</p></body>
</html>`
)

// testCoverBytes 是最小 JPEG 头尾：内容不重要，MIME 由清单声明。
var testCoverBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0xFF, 0xD9}

// zipEntry 是合成 EPUB 的一个条目。
type zipEntry struct {
	name string
	data []byte
}

// toZip 在内存里合成 zip，不落盘。
func toZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("合成 zip 条目 %s: %v", e.name, err)
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

// testOPFText 拼一份最小 OPF：元数据/清单/spine/guide 由调用方给片段。
func testOPFText(meta, manifest, spine, guide string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">` + meta + `</metadata>
  <manifest>` + manifest + `</manifest>
  <spine>` + spine + `</spine>` + guide + `
</package>`
}

// buildEPUB 用给定 OPF 与附加条目合成 EPUB（container.xml 指向 OEBPS/content.opf）。
func buildEPUB(t *testing.T, opf string, extra ...zipEntry) []byte {
	t.Helper()
	entries := []zipEntry{
		{name: "mimetype", data: []byte("application/epub+zip")},
		{name: "META-INF/container.xml", data: []byte(testContainerXML)},
		{name: "OEBPS/content.opf", data: []byte(opf)},
	}
	return toZip(t, append(entries, extra...))
}

// standardEPUB 是覆盖「三章 + 封面」的标准样本。
func standardEPUB(t *testing.T) []byte {
	t.Helper()
	return buildEPUB(t, testOPF,
		zipEntry{name: "OEBPS/ch1.xhtml", data: []byte(testCh1XHTML)},
		zipEntry{name: "OEBPS/ch2.xhtml", data: []byte(testCh2XHTML)},
		zipEntry{name: "OEBPS/ch3.xhtml", data: []byte(testCh3XHTML)},
		zipEntry{name: "OEBPS/cover.jpg", data: testCoverBytes},
	)
}

// mustOpen 解析成功或直接失败。
func mustOpen(t *testing.T, name string, data []byte) *Book {
	t.Helper()
	book, err := Open(name, data)
	if err != nil {
		t.Fatalf("Open(%q) 失败: %v", name, err)
	}
	return book
}

// openErr 断言解析失败并返回错误。
func openErr(t *testing.T, name string, data []byte) error {
	t.Helper()
	book, err := Open(name, data)
	if err == nil {
		t.Fatalf("Open(%q) 期望失败，实际成功（%d 章）", name, len(book.Chapters))
	}
	return err
}
