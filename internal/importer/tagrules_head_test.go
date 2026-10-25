package importer

import (
	"strings"
	"testing"

	"norvels/internal/model"
	"norvels/internal/store"
)

// TestMatchTagsHeadLimit 正文只匹配前 5000 字：第 5000 字之后的关键词不命中。
func TestMatchTagsHeadLimit(t *testing.T) {
	b := &model.ParsedBook{
		Title:  "长文之书",
		Author: "作者壬",
		Chapters: []model.ParsedChapter{
			{Title: "第一章", Content: strings.Repeat("甲", 5000)},
			{Title: "第二章", Content: "关键词在五千字后"},
		},
	}
	rules := []store.TagRule{{Tag: "远词", Keywords: []string{"关键词在五千字后"}}}
	if got := matchTags(rules, b); len(got) != 0 {
		t.Errorf("matchTags = %v，期望不命中（关键词在正文前 5000 字之外）", got)
	}
}
