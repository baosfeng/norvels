package parser

import (
	"strings"
	"testing"

	"norvels/internal/model"
)

// TestKeepPreambleAsChapter 覆盖「过长前置正文补章」的触发与不触发。
func TestKeepPreambleAsChapter(t *testing.T) {
	d := &Doc{Name: "x.txt", opts: DefaultOptions()}
	d.Chapters = []model.ParsedChapter{{Title: "第一章", Content: "正文一"}}
	d.keepPreambleAsChapter(strings.Repeat("前置正文内容。", 500))
	if len(d.Chapters) != 2 {
		t.Fatalf("章数 = %d，期望 2", len(d.Chapters))
	}
	if d.Chapters[0].Title != "正文" {
		t.Errorf("首章标题 = %q，期望「正文」", d.Chapters[0].Title)
	}
	if d.Chapters[0].Idx != 0 || d.Chapters[1].Idx != 1 {
		t.Error("插入后章序号未重排")
	}
	if len(d.Warnings) == 0 {
		t.Error("应记录告警")
	}
	d2 := &Doc{Name: "x.txt", opts: DefaultOptions()}
	d2.Chapters = []model.ParsedChapter{{Title: "第一章", Content: "正文一"}}
	d2.keepPreambleAsChapter("短前置")
	if len(d2.Chapters) != 1 {
		t.Errorf("短前置不应补章，实际 %d 章", len(d2.Chapters))
	}
}

// TestBuildFromCandidatesVolumeBodyKept 是回归测试：
// 卷标记之后、首个章标记之前的正文必须并进那一章，不能丢。
func TestBuildFromCandidatesVolumeBodyKept(t *testing.T) {
	text := "第一卷 风云\n这是卷后的正文，必须被保留下来。\n第一章 出发\n第一章正文\n"
	cands := scanCandidates(text)
	if len(cands) < 2 {
		t.Fatalf("候选数 = %d，期望 ≥ 2", len(cands))
	}
	res := buildFromCandidates(text, cands, newCleaner(text))
	if !strings.Contains(joinAll(res.chapters), "必须被保留") {
		t.Errorf("卷后正文丢失，产出 = %q", joinAll(res.chapters))
	}
}

// TestBuildFromCandidatesVolumeTailKept 覆盖「卷后无章标记」的收尾路径。
func TestBuildFromCandidatesVolumeTailKept(t *testing.T) {
	text := "第一卷 风云\n只有正文没有章节标记的一大段内容。\n"
	cands := scanCandidates(text)
	if len(cands) == 0 {
		t.Skip("该 fixture 未产出候选")
	}
	res := buildFromCandidates(text, cands, newCleaner(text))
	if !strings.Contains(joinAll(res.chapters), "只有正文没有章节标记") {
		t.Errorf("卷尾正文丢失，产出 = %q", joinAll(res.chapters))
	}
}

// TestSplitOutlierContentRejectsProse 覆盖「不采纳」路径：
// 纯段落流里的零星候选不该被当成章节结构。
func TestSplitOutlierContentRejectsProse(t *testing.T) {
	ch := model.ParsedChapter{Title: "巨型章", Content: strings.Repeat("这是一段很长的正文，没有任何章节标记。\n", 3000)}
	if parts, ok := splitOutlierContent(ch, newCleaner(ch.Content)); ok {
		t.Errorf("纯段落流不应被再切，实际切出 %d 段", len(parts))
	}
}

// TestRefineSplitNoOutlier 覆盖「没有离群章就什么都不做」。
func TestRefineSplitNoOutlier(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 5; i++ {
		b.WriteString("第" + itoa(i) + "章 标题\n")
		b.WriteString(strings.Repeat("正常长度正文。\n", 20))
	}
	b.WriteString("尾声之后的收束段落，保证末章也有正文。\n")
	book, err := New(DefaultOptions()).ParseBytes("某书.txt", []byte(b.String()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) != 5 {
		t.Errorf("章数 = %d，期望 5（不变）", len(book.Chapters))
	}
}

// TestCountSanityGateBlocksWholeFallback 覆盖「count_sanity 达标时禁止整体回退」。
func TestCountSanityGateBlocksWholeFallback(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 15; i++ {
		b.WriteString("第" + itoa(i) + "章 标题\n")
		for j := 0; j < 60; j++ {
			b.WriteString("这是一段长度正常的合成正文，用于验证不会被整体回退。\n")
		}
	}
	b.WriteString("末尾的收束段落，保证最后一章也有正文。\n")
	book, err := New(DefaultOptions()).ParseBytes("某书.txt", []byte(b.String()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) != 15 {
		t.Errorf("章数 = %d，期望 15（候选结构被保留）", len(book.Chapters))
	}
	for _, c := range book.Chapters {
		if !strings.Contains(c.Title, "章") {
			t.Errorf("标题丢失: %q", c.Title)
		}
	}
}

// joinAll 把若干章的正文拼起来，便于断言内容是否保留。
func joinAll(chs []model.ParsedChapter) string {
	parts := make([]string, 0, len(chs))
	for _, c := range chs {
		parts = append(parts, c.Content)
	}
	return strings.Join(parts, "\n")
}

// TestReSplitOutliersMergedPath 覆盖「离群章里没有可用结构」时不改动的分支。
func TestReSplitOutliersMergedPath(t *testing.T) {
	d := &Doc{Name: "x.txt", opts: DefaultOptions()}
	// 一个超长章但内部没有任何章节标记。
	d.Chapters = []model.ParsedChapter{
		{Title: "第一章", Content: strings.Repeat("普通正文。", 50)},
		{Title: "第二章", Content: strings.Repeat("没有任何标记的长正文。", 20000)},
	}
	before := len(d.Chapters)
	if n := d.reSplitOutliers(nil); n != 0 {
		t.Errorf("无可切结构时应返回 0，实际 %d", n)
	}
	if len(d.Chapters) != before {
		t.Errorf("章数被改动: %d → %d", before, len(d.Chapters))
	}
}

// TestReSplitOutliersEmpty 覆盖空章节列表。
func TestReSplitOutliersEmpty(t *testing.T) {
	d := &Doc{Name: "x.txt", opts: DefaultOptions()}
	if n := d.reSplitOutliers(nil); n != 0 {
		t.Errorf("空列表应返回 0，实际 %d", n)
	}
}

// TestIsVolumeUnitAt 覆盖卷单位否定预查。
func TestIsVolumeUnitAt(t *testing.T) {
	rejects := []string{"部分", "部队", "集合", "篇章"}
	for _, s := range rejects {
		if isVolumeUnitAt(s, 0) {
			t.Errorf("%q 应被否定预查拒绝", s)
		}
	}
	accepts := []string{"卷 标题", "部 上", "集 一", "篇 序", "卷"}
	for _, s := range accepts {
		if !isVolumeUnitAt(s, 0) {
			t.Errorf("%q 应被识别为合法卷单位", s)
		}
	}
}

// TestDecodeUTF16Surrogates 覆盖 UTF-16 的代理项处理（含孤立代理）。
func TestDecodeUTF16Surrogates(t *testing.T) {
	// 成对代理 → 一个补充平面字符。
	paired := []byte{0x3D, 0xD8, 0x0B, 0xDC} // U+1F40B
	if got := decodeUTF16(paired, true); got.bad != 0 || got.text == "" {
		t.Errorf("成对代理解码 = %q bad=%d", got.text, got.bad)
	}
	// 孤立高代理 + 孤立低代理：应记为 bad。
	lonely := []byte{0x3D, 0xD8, 0x41, 0x00}
	if got := decodeUTF16(lonely, true); got.bad == 0 {
		t.Error("孤立代理应计入 bad")
	}
	// 奇数尾字节：应记 bad。
	if got := decodeUTF16([]byte{0x41, 0x00, 0x42}, true); got.bad == 0 {
		t.Error("奇数尾字节应计入 bad")
	}
}

// TestUTF16Warn 覆盖 UTF-16 告警的两条分支。
func TestUTF16Warn(t *testing.T) {
	if w := utf16Warn([]byte{0x41, 0x00}, true); len(w) != 0 {
		t.Errorf("正常字节不应告警: %v", w)
	}
	if w := utf16Warn([]byte{0x3D, 0xD8, 0x41, 0x00}, true); len(w) == 0 {
		t.Error("孤立代理应告警（LE）")
	}
	if w := utf16Warn([]byte{0xD8, 0x3D, 0x00, 0x41}, false); len(w) == 0 {
		t.Error("孤立代理应告警（BE）")
	}
}
