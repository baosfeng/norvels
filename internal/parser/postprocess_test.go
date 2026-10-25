package parser

import (
	"strings"
	"testing"

	"norvels/internal/model"
)

// TestIsEmptyChapter 覆盖「空壳章节」的几种判定：空串、纯空白、纯装饰符。
func TestIsEmptyChapter(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"空串", "", true},
		{"换行", "\n\n", true},
		{"空格制表", " \t \u3000 ", true},
		// 真实语料里空壳章节的典型残留：星号、全角等号、破折号、波浪线。
		{"星号装饰", "★ ★ ★ ★ ★", true},
		{"全角等号", "＝＝＝＝＝＝", true},
		{"破折号行", strings.Repeat("-", 40), true},
		{"波浪线", "~~~~", true},
		{"标点", "……", true},
		{"正常正文", "这是一段正常的中文正文内容。", false},
		{"单字", "好", false},
	}
	for _, c := range cases {
		got := isEmptyChapter(model.ParsedChapter{Content: c.content})
		if got != c.want {
			t.Errorf("%s: isEmptyChapter(%q) = %v，期望 %v", c.name, c.content, got, c.want)
		}
	}
}

// TestPostProcessDropsEmptyShells 是回归测试：装饰符空壳章节必须被清理，
// 且其中间章节的标题要并到上一章（不丢章节名）。
func TestPostProcessDropsEmptyShells(t *testing.T) {
	res := splitResult{chapters: []model.ParsedChapter{
		{Title: "第一章 甲", Content: "正文一"},
		{Title: "第二章 乙（续2）", Content: "★★★"},
		{Title: "第三章 丙", Content: "正文三"},
		{Title: "第四章 丁（续3）", Content: "\u3000\u3000"},
	}}
	postProcess(&res, 0)
	if len(res.chapters) != 2 {
		t.Fatalf("章数 = %d，期望 2（两个空壳都被清理）", len(res.chapters))
	}
	if res.chapters[0].Title != "第一章 甲" {
		t.Errorf("首章标题被改: %q", res.chapters[0].Title)
	}
	joined := res.chapters[0].Content + "\n" + res.chapters[1].Content
	if !strings.Contains(joined, "续") {
		t.Errorf("空壳章节的标题未保留: %q", joined)
	}
	if res.chapters[1].Title != "第三章 丙" {
		t.Errorf("第三章标题被改: %q", res.chapters[1].Title)
	}
	for _, c := range res.chapters {
		if isEmptyChapter(c) {
			t.Errorf("仍残留空壳章节: %q", c.Title)
		}
	}
}

// TestPostProcessLeadsWithEmptyShell 覆盖首章就是空壳的情况（没有上一章可并）。
func TestPostProcessLeadsWithEmptyShell(t *testing.T) {
	res := splitResult{chapters: []model.ParsedChapter{
		{Title: "第一章 空", Content: "★★★"},
		{Title: "第二章 甲", Content: "正文二"},
	}}
	postProcess(&res, 0)
	if len(res.chapters) != 1 {
		t.Fatalf("章数 = %d，期望 1", len(res.chapters))
	}
	if res.chapters[0].Title != "第二章 甲" {
		t.Errorf("首章 = %q", res.chapters[0].Title)
	}
}

// TestPostProcessAllEmptyFallsBack 覆盖「全书都是空壳」时的兜底。
func TestPostProcessAllEmptyFallsBack(t *testing.T) {
	d := &Doc{Name: "空壳.txt", Text: "★★★\n＝＝＝", opts: DefaultOptions()}
	d.Chapters = []model.ParsedChapter{
		{Title: "第一章", Content: "★★★"},
		{Title: "第二章", Content: "＝＝＝"},
	}
	postProcessDoc(d)
	if len(d.Chapters) == 0 {
		t.Fatal("全书空壳时应退化为整篇一章，保证能入库")
	}
}
