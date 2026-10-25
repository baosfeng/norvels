package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// 合成 EPUB 分卷样本：书名/卷名/章名全部虚构（隐私红线）。

// volumesOPF 是带 nav 文档的 EPUB3 清单：nav 不进 spine。
const volumesOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>分卷示例书</dc:title></metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="ch2.xhtml" media-type="application/xhtml+xml"/>
    <item id="c3" href="ch3.xhtml" media-type="application/xhtml+xml"/>
    <item id="c4" href="ch4.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/><itemref idref="c3"/><itemref idref="c4"/></spine>
</package>`

// volumesNav 是两卷、每卷两章的两级目录。
const volumesNav = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body><nav epub:type="toc"><ol>
  <li><a href="ch1.xhtml">上卷 合成甲部</a><ol>
    <li><a href="ch1.xhtml">第一章 卷题一</a></li>
    <li><a href="ch2.xhtml">第二章 卷题二</a></li>
  </ol></li>
  <li><a href="ch3.xhtml">下卷 合成乙部</a><ol>
    <li><a href="ch3.xhtml">第三章 卷题三</a></li>
    <li><a href="ch4.xhtml">第四章 卷题四</a></li>
  </ol></li>
</ol></nav></body></html>`

// volumeChapterXHTML 造一章正文（h1 与 TOC 标题不同，验证标题取 TOC）。
func volumeChapterXHTML(h1, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>` + h1 + `</h1><p>` + body + `</p></body></html>`
}

// volumesEPUB 合成一本带两级目录的 EPUB。
func volumesEPUB(t *testing.T) []byte {
	t.Helper()
	return epubBytes(t, []epubEntry{
		{name: "mimetype", data: []byte("application/epub+zip")},
		{name: "META-INF/container.xml", data: []byte(epubContainerXML)},
		{name: "OEBPS/content.opf", data: []byte(volumesOPF)},
		{name: "OEBPS/nav.xhtml", data: []byte(volumesNav)},
		{name: "OEBPS/ch1.xhtml", data: []byte(volumeChapterXHTML("第一章 正文题", "第一章合成正文。"))},
		{name: "OEBPS/ch2.xhtml", data: []byte(volumeChapterXHTML("第二章 正文题", "第二章合成正文。"))},
		{name: "OEBPS/ch3.xhtml", data: []byte(volumeChapterXHTML("第三章 正文题", "第三章合成正文。"))},
		{name: "OEBPS/ch4.xhtml", data: []byte(volumeChapterXHTML("第四章 正文题", "第四章合成正文。"))},
	})
}

// TestParseFileEPUBVolumes 覆盖 EPUB 目录层级 → model 卷与章归属（1 起 → 0 起只转一次）。
func TestParseFileEPUBVolumes(t *testing.T) {
	book, err := parseEPUB(t, "分卷示例.epub", volumesEPUB(t))
	if err != nil {
		t.Fatalf("解析 EPUB: %v", err)
	}
	if len(book.Volumes) != 2 {
		t.Fatalf("卷数 = %d，期望 2（%+v）", len(book.Volumes), book.Volumes)
	}
	if book.Volumes[0].Title != "上卷 合成甲部" || book.Volumes[1].Title != "下卷 合成乙部" {
		t.Errorf("卷标题 = %q / %q", book.Volumes[0].Title, book.Volumes[1].Title)
	}
	wantVol := []int{0, 0, 1, 1}
	for i, c := range book.Chapters {
		if c.VolumeIdx != wantVol[i] {
			t.Errorf("第 %d 章卷下标 = %d，期望 %d（model 口径 0 起）", i+1, c.VolumeIdx, wantVol[i])
		}
	}
	if book.Chapters[0].Title != "第一章 卷题一" {
		t.Errorf("第 1 章标题 = %q，期望取 TOC", book.Chapters[0].Title)
	}
}

// TestRunEPUBVolumesPersisted 覆盖卷落库：volumes 行数与被分卷章节数。
func TestRunEPUBVolumesPersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "分卷示例.epub")
	if err := os.WriteFile(path, volumesEPUB(t), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st := newStore(t)
	res, err := Run(ctx, st, opts(dir), nil)
	if err != nil || len(res.Imported) != 1 {
		t.Fatalf("Run: err=%v imported=%v", err, res.Imported)
	}
	detail, err := st.GetBook(ctx, res.Imported[0])
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if detail.VolumeCount != 2 || len(detail.Volumes) != 2 {
		t.Fatalf("卷数 = %d（详情 %d），期望 2", detail.VolumeCount, len(detail.Volumes))
	}
	for i, node := range detail.Volumes {
		if len(node.Chapters) != 2 {
			t.Errorf("第 %d 卷章数 = %d，期望 2", i+1, len(node.Chapters))
		}
	}
}
