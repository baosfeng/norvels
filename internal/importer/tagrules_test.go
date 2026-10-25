package importer

import (
	"testing"

	"norvels/internal/model"
	"norvels/internal/store"
)

// TestMatchTags 覆盖匹配文本范围、大小写、上限与「同标签只打一次」。
func TestMatchTags(t *testing.T) {
	b := &model.ParsedBook{
		Title:  "Magic 之旅",
		Author: "作者甲",
		Chapters: []model.ParsedChapter{
			{Title: "第一章 精灵", Content: "正文里提到飞船。"},
			{Title: "第二章 无", Content: ""},
			{Title: "第三章 无", Content: ""},
			{Title: "第四章 禁词XYZ", Content: ""},
		},
	}
	rules := []store.TagRule{
		{Tag: "奇幻", Keywords: []string{"魔法", "精灵"}},
		{Tag: "科幻", Keywords: []string{"飞船"}},
		{Tag: "大小写", Keywords: []string{"magic"}},
		{Tag: "作者", Keywords: []string{"作者甲"}},
		{Tag: "第四章标题不参与", Keywords: []string{"禁词xyz"}},
	}
	got := matchTags(rules, b)
	want := []string{"奇幻", "科幻", "大小写", "作者"}
	if len(got) != len(want) {
		t.Fatalf("matchTags = %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matchTags = %v，期望 %v", got, want)
		}
	}
}

// TestMatchTagsLimit 单书最多 10 个标签。
func TestMatchTagsLimit(t *testing.T) {
	b := &model.ParsedBook{Title: "上限之书", Author: "作者乙"}
	rules := make([]store.TagRule, 0, 15)
	for i := 0; i < 15; i++ {
		rules = append(rules, store.TagRule{Tag: "标签" + string(rune('A'+i)), Keywords: []string{"上限之书"}})
	}
	if got := matchTags(rules, b); len(got) != maxAutoTags {
		t.Errorf("命中标签数 = %d，期望上限 %d", len(got), maxAutoTags)
	}
}

// TestMatchTagsNoRules 空规则不打标签。
func TestMatchTagsNoRules(t *testing.T) {
	if got := matchTags(nil, &model.ParsedBook{Title: "x"}); len(got) != 0 {
		t.Errorf("空规则应返回空标签，实际 %v", got)
	}
}
