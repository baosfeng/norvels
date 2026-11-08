package enrich

import "testing"

// TestDecideCategoryWithoutHits 覆盖「没有书源命中也能离线补分类」：
// 生产上 10 万本未分类的书绝大多数在任何书源都搜不到，词表必须独立工作。
func TestDecideCategoryWithoutHits(t *testing.T) {
	s := Subject{Title: "修仙从灵根开始", Author: "佚名", CategoryL1: Unclassified}
	d := Decide(s, nil, DefaultOptions())
	if d.Category == nil || d.Category.Value == "" {
		t.Fatalf("无命中时也应能靠本地词表补分类，实际 %+v", d.Category)
	}
	if d.Category.Sources[0] != "local:keyword" {
		t.Errorf("来源应标 local:keyword，实际 %v", d.Category.Sources)
	}
	if d.Author != nil || d.Intro != nil {
		t.Errorf("没有命中时作者与简介不该被改: %+v / %+v", d.Author, d.Intro)
	}
	if !containsSkip(d.Skipped, "no_hit") {
		t.Errorf("应记录 no_hit，实际 %v", d.Skipped)
	}
}

// TestDecideCategoryNoHitAtAll 覆盖「词表也没命中」：什么都不改，原因可解释。
func TestDecideCategoryNoHitAtAll(t *testing.T) {
	s := Subject{Title: "随记", CategoryL1: Unclassified}
	d := Decide(s, nil, DefaultOptions())
	if d.Category != nil {
		t.Fatalf("词表未命中不该给分类，实际 %+v", d.Category)
	}
	if !containsSkip(d.Skipped, "category:no_hit") {
		t.Errorf("应记录 category:no_hit，实际 %v", d.Skipped)
	}
}
