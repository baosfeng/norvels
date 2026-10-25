package ebook

import (
	"strings"
	"testing"
)

// 合成目录样本：书名/卷名/章名全部虚构（隐私红线：测试里不出现真实书名）。

// tocChapterXHTML 合成一章的 XHTML：h1 与 TOC 标题不同，用来验证「标题取 TOC」。
func tocChapterXHTML(h1, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>文档标题一律相同</title></head>
<body><h1>` + h1 + `</h1><p>` + body + `</p></body></html>`
}

// hasWarning 判断告警里是否含某段文字。
func hasWarning(warnings []string, want string) bool {
	for _, w := range warnings {
		if strings.Contains(w, want) {
			return true
		}
	}
	return false
}

// navXHTML 是 EPUB3 的两级目录：两个卷、每卷两章；分组条目 href 与首章同一个文件。
const navXHTML = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>目录</title></head>
<body>
<nav epub:type="toc" id="toc"><h1>目录</h1>
<ol>
  <li><a href="ch1.xhtml">上卷 甲部</a>
    <ol>
      <li><a href="ch1.xhtml">第一章 目录题</a></li>
      <li><a href="ch2.xhtml">第二章 目录题</a></li>
    </ol>
  </li>
  <li><a href="ch3.xhtml">下卷 乙部</a>
    <ol>
      <li><a href="ch3.xhtml">第三章 目录题</a></li>
      <li><a href="ch4.xhtml">第四章 目录题</a></li>
    </ol>
  </li>
</ol></nav>
</body></html>`

// navOPF 是带 nav 文档的 EPUB3 清单：nav 不进 spine。
const navOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>目录样本</dc:title></metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="ch2.xhtml" media-type="application/xhtml+xml"/>
    <item id="c3" href="ch3.xhtml" media-type="application/xhtml+xml"/>
    <item id="c4" href="ch4.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="c1"/><itemref idref="c2"/><itemref idref="c3"/><itemref idref="c4"/>
  </spine>
</package>`

// tocNCX 是 EPUB2 的两级目录样本：结构与 navXHTML 等价。
const tocNCX = `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="n1"><navLabel><text>上卷 甲部</text></navLabel><content src="ch1.xhtml"/>
      <navPoint id="n2"><navLabel><text>第一章 NCX 题</text></navLabel><content src="ch1.xhtml"/></navPoint>
      <navPoint id="n3"><navLabel><text>第二章 NCX 题</text></navLabel><content src="ch2.xhtml"/></navPoint>
    </navPoint>
    <navPoint id="n4"><navLabel><text>下卷 乙部</text></navLabel><content src="ch3.xhtml"/>
      <navPoint id="n5"><navLabel><text>第三章 NCX 题</text></navLabel><content src="ch3.xhtml"/></navPoint>
      <navPoint id="n6"><navLabel><text>第四章 NCX 题</text></navLabel><content src="ch4.xhtml"/></navPoint>
    </navPoint>
  </navMap>
</ncx>`

// ncxOPF 是带 toc.ncx 的 EPUB2 清单。spine 的 toc 属性在此解析器里不参与定位。
const ncxOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>目录样本</dc:title></metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="ch2.xhtml" media-type="application/xhtml+xml"/>
    <item id="c3" href="ch3.xhtml" media-type="application/xhtml+xml"/>
    <item id="c4" href="ch4.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx">
    <itemref idref="c1"/><itemref idref="c2"/><itemref idref="c3"/><itemref idref="c4"/>
  </spine>
</package>`

// tocWant 是目录样本对一章的期望（标题 + 卷下标）。
type tocWant struct {
	title string
	vol   int
}

// assertVolumeTitles 校验卷数与卷标题。
func assertVolumeTitles(t *testing.T, book *Book, want ...string) {
	t.Helper()
	if len(book.Volumes) != len(want) {
		t.Fatalf("卷数 = %d，期望 %d（%v）", len(book.Volumes), len(want), book.Volumes)
	}
	for i, w := range want {
		if book.Volumes[i].Title != w {
			t.Errorf("第 %d 卷标题 = %q，期望 %q", i+1, book.Volumes[i].Title, w)
		}
	}
}

// assertTOCChapters 校验每章标题与卷归属（目录样本的章标题都取自 TOC 第二层）。
func assertTOCChapters(t *testing.T, chapters []Chapter, want []tocWant) {
	t.Helper()
	if len(chapters) != len(want) {
		t.Fatalf("章数 = %d，期望 %d", len(chapters), len(want))
	}
	for i, w := range want {
		c := chapters[i]
		if c.Title != w.title || c.VolumeIdx != w.vol {
			t.Errorf("第 %d 章 = %q（卷 %d），期望 %q（卷 %d）", i+1, c.Title, c.VolumeIdx, w.title, w.vol)
		}
		if c.Level != 2 {
			t.Errorf("第 %d 章 Level = %d，期望 2", i+1, c.Level)
		}
	}
}

// fourChapters 是四章正文，每章 h1 与 TOC 标题都不相同。
func fourChapters() []zipEntry {
	return []zipEntry{
		{name: "OEBPS/ch1.xhtml", data: []byte(tocChapterXHTML("第一章 正文题", "第一章正文。"))},
		{name: "OEBPS/ch2.xhtml", data: []byte(tocChapterXHTML("第二章 正文题", "第二章正文。"))},
		{name: "OEBPS/ch3.xhtml", data: []byte(tocChapterXHTML("第三章 正文题", "第三章正文。"))},
		{name: "OEBPS/ch4.xhtml", data: []byte(tocChapterXHTML("第四章 正文题", "第四章正文。"))},
	}
}

// TestTOCFromNavXHTML 覆盖 EPUB3 nav.xhtml 的两级目录 → 两个卷。
func TestTOCFromNavXHTML(t *testing.T) {
	entries := append([]zipEntry{{name: "OEBPS/nav.xhtml", data: []byte(navXHTML)}}, fourChapters()...)
	book := mustOpen(t, "目录样本.epub", buildEPUB(t, navOPF, entries...))
	assertVolumeTitles(t, book, "上卷 甲部", "下卷 乙部")
	assertTOCChapters(t, book.Chapters, []tocWant{
		{"第一章 目录题", 1}, {"第二章 目录题", 1}, {"第三章 目录题", 2}, {"第四章 目录题", 2},
	})
	if len(book.Volumes[0].Chapters) != 2 || len(book.Volumes[1].Chapters) != 2 {
		t.Errorf("各卷章数 = %d / %d，期望 2 / 2", len(book.Volumes[0].Chapters), len(book.Volumes[1].Chapters))
	}
	if strings.Contains(book.Chapters[0].Content, "正文题") {
		t.Errorf("正文不该残留 h1：%q", book.Chapters[0].Content)
	}
}

// TestTOCFromNCX 覆盖 EPUB2 toc.ncx 兜底路径。
func TestTOCFromNCX(t *testing.T) {
	entries := append([]zipEntry{{name: "OEBPS/toc.ncx", data: []byte(tocNCX)}}, fourChapters()...)
	book := mustOpen(t, "目录样本.epub", buildEPUB(t, ncxOPF, entries...))
	if len(book.Volumes) != 2 {
		t.Fatalf("卷数 = %d，期望 2", len(book.Volumes))
	}
	want := []string{"第一章 NCX 题", "第二章 NCX 题", "第三章 NCX 题", "第四章 NCX 题"}
	for i, w := range want {
		if book.Chapters[i].Title != w {
			t.Errorf("第 %d 章标题 = %q，期望 %q", i+1, book.Chapters[i].Title, w)
		}
	}
	if book.Chapters[3].VolumeIdx != 2 || book.Chapters[3].Level != 2 {
		t.Errorf("第 4 章卷/层 = %d/%d，期望 2/2", book.Chapters[3].VolumeIdx, book.Chapters[3].Level)
	}
}

// TestNoTOCFallsBackFlat 覆盖无 TOC：平铺，标题仍取 XHTML 的 h1。
func TestNoTOCFallsBackFlat(t *testing.T) {
	book := mustOpen(t, "标准样本.epub", standardEPUB(t))
	if len(book.Volumes) != 0 {
		t.Errorf("无 TOC 时卷数 = %d，期望 0", len(book.Volumes))
	}
	for i, c := range book.Chapters {
		if c.VolumeIdx != 0 || c.Level != 0 {
			t.Errorf("第 %d 章卷/层 = %d/%d，期望 0/0", i+1, c.VolumeIdx, c.Level)
		}
	}
	if book.Chapters[0].Title != "第一章 起始" {
		t.Errorf("第 1 章标题 = %q，期望取 XHTML h1", book.Chapters[0].Title)
	}
	if len(book.Warnings) != 0 {
		t.Errorf("无 TOC 不该产生告警: %v", book.Warnings)
	}
}

// TestTOCSpineConflict 覆盖 TOC 与 spine 冲突：章序与章数以 spine 为准，
// TOC 只提供标题；TOC 里有而 spine 没有的条目被忽略。
func TestTOCSpineConflict(t *testing.T) {
	conflictNav := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body><nav epub:type="toc"><ol>
  <li><a href="ch3.xhtml">第三章 目录题</a></li>
  <li><a href="ch2.xhtml">第二章 目录题</a></li>
  <li><a href="ghost.xhtml">幽灵章</a></li>
  <li><a href="ch1.xhtml">第一章 目录题</a></li>
</ol></nav></body></html>`
	entries := append([]zipEntry{{name: "OEBPS/nav.xhtml", data: []byte(conflictNav)}}, fourChapters()[:3]...)
	book := mustOpen(t, "目录样本.epub", buildEPUB(t, navOPF, entries...))
	if len(book.Chapters) != 3 {
		t.Fatalf("章数 = %d，期望 3（以 spine 为准，幽灵条目不新增章节）", len(book.Chapters))
	}
	wantTitles := []string{"第一章 目录题", "第二章 目录题", "第三章 目录题"}
	wantBodies := []string{"第一章正文。", "第二章正文。", "第三章正文。"}
	for i := range book.Chapters {
		if book.Chapters[i].Title != wantTitles[i] {
			t.Errorf("第 %d 章标题 = %q，期望 %q", i+1, book.Chapters[i].Title, wantTitles[i])
		}
		if !strings.Contains(book.Chapters[i].Content, wantBodies[i]) {
			t.Errorf("第 %d 章正文 = %q，期望按 spine 顺序含 %q", i+1, book.Chapters[i].Content, wantBodies[i])
		}
	}
}

// TestTOCDeepLevelsClamped 覆盖层级深度 >3：只取前 3 层，不 panic。
func TestTOCDeepLevelsClamped(t *testing.T) {
	deepNav := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body><nav epub:type="toc"><ol>
  <li><a href="ch1.xhtml">卷一</a><ol>
    <li><a href="ch1.xhtml">第一部</a><ol>
      <li><a href="ch2.xhtml">第一章</a><ol>
        <li><a href="ch3.xhtml">第一节</a></li>
      </ol></li>
    </ol></li>
  </ol></li>
</ol></nav></body></html>`
	entries := append([]zipEntry{{name: "OEBPS/nav.xhtml", data: []byte(deepNav)}}, fourChapters()[:3]...)
	book := mustOpen(t, "目录样本.epub", buildEPUB(t, navOPF, entries...))
	if len(book.Volumes) != 1 {
		t.Fatalf("卷数 = %d，期望 1", len(book.Volumes))
	}
	if book.Chapters[0].VolumeIdx != 1 {
		t.Errorf("第 1 章卷 = %d，期望 1", book.Chapters[0].VolumeIdx)
	}
	// 前两章有 TOC 对应项，层级必须落在 1~3；第 3 章只被第 4 层条目引用，
	// 该条目被丢弃，所以标题回落 XHTML h1、Level 保持 0。
	for i := 0; i < 2; i++ {
		if c := book.Chapters[i]; c.Level < 1 || c.Level > maxTOCLevel {
			t.Errorf("第 %d 章 Level = %d，期望落在 1~%d", i+1, c.Level, maxTOCLevel)
		}
	}
	if book.Chapters[2].Title != "第三章 正文题" || book.Chapters[2].Level != 0 {
		t.Errorf("第 3 章标题/层 = %q/%d，期望丢弃第 4 层后回落 XHTML h1", book.Chapters[2].Title, book.Chapters[2].Level)
	}
}

// TestTOCMalformedFallsBack 覆盖目录损坏：只记 warning，章节照常按 spine 抽。
func TestTOCMalformedFallsBack(t *testing.T) {
	entries := append([]zipEntry{{name: "OEBPS/toc.ncx", data: []byte("<ncx><navMap>")}}, fourChapters()...)
	book := mustOpen(t, "目录样本.epub", buildEPUB(t, ncxOPF, entries...))
	if len(book.Chapters) != 4 {
		t.Fatalf("章数 = %d，期望 4", len(book.Chapters))
	}
	if len(book.Volumes) != 0 {
		t.Errorf("目录损坏时不该分卷，实际 %d 卷", len(book.Volumes))
	}
	if !hasWarning(book.Warnings, "解析目录") {
		t.Errorf("应记录目录解析告警，实际 %v", book.Warnings)
	}
}
