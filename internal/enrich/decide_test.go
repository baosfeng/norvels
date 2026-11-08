package enrich

import (
	"strings"
	"testing"
)

// TestDecideAuthorFromPlaceholder 覆盖核心场景：库内是「佚名」，源给了真名。
func TestDecideAuthorFromPlaceholder(t *testing.T) {
	s := Subject{Title: "示例书一", Author: "佚名", CategoryL1: Unclassified}
	hits := []Hit{
		{SourceID: "src-a", Priority: 10, Title: "示例书一", Author: "作者甲"},
		{SourceID: "src-b", Priority: 5, Title: "示例书一", Author: "作者甲"},
	}
	d := Decide(s, hits, DefaultOptions())
	if d.Author == nil || d.Author.Value != "作者甲" || !d.Author.Changed {
		t.Fatalf("应从占位名改成真名，实际 %+v", d.Author)
	}
	if len(d.Author.Sources) != 2 {
		t.Errorf("两个源都支持，Sources 应有 2 项，实际 %v", d.Author.Sources)
	}
}

// TestDecideAuthorKeepsRealName 覆盖「已有真名不覆盖」。
func TestDecideAuthorKeepsRealName(t *testing.T) {
	s := Subject{Title: "示例书一", Author: "已有作者"}
	hits := []Hit{{SourceID: "src-a", Title: "示例书一", Author: "另一个作者"}}
	d := Decide(s, hits, DefaultOptions())
	if d.Author != nil {
		t.Fatalf("已有真实作者时不该改，实际 %+v", d.Author)
	}
	if !containsSkip(d.Skipped, "author:already_present") {
		t.Errorf("应记录 already_present，实际 %v", d.Skipped)
	}
}

// TestDecideLowTitleSim 覆盖相似度过滤：同名不同书不参与决策。
func TestDecideLowTitleSim(t *testing.T) {
	s := Subject{Title: "示例书一", Author: "佚名"}
	hits := []Hit{{SourceID: "src-a", Title: "完全无关的另一本", Author: "作者甲"}}
	d := Decide(s, hits, DefaultOptions())
	if d.Author != nil {
		t.Fatalf("相似度不足的命中不该被采纳，实际 %+v", d.Author)
	}
	if !containsSkip(d.Skipped, "no_hit") {
		t.Errorf("应记录 no_hit，实际 %v", d.Skipped)
	}
}

// TestDecidePlaceholderOnly 覆盖「源给的也是占位名」：不写。
func TestDecidePlaceholderOnly(t *testing.T) {
	s := Subject{Title: "示例书一", Author: "佚名"}
	hits := []Hit{{SourceID: "src-a", Title: "示例书一", Author: "未知"}}
	d := Decide(s, hits, DefaultOptions())
	if d.Author != nil {
		t.Fatalf("源也只给占位名时不该写，实际 %+v", d.Author)
	}
	if !containsSkip(d.Skipped, "author:placeholder_only") {
		t.Errorf("应记录 placeholder_only，实际 %v", d.Skipped)
	}
}

// TestDecideIntroLongest 覆盖简介取最长、且过短不写。
func TestDecideIntroLongest(t *testing.T) {
	s := Subject{Title: "示例书一", Intro: ""}
	long := strings.Repeat("完", 60)
	hits := []Hit{
		{SourceID: "src-a", Priority: 10, Title: "示例书一", Intro: strings.Repeat("短", 40)},
		{SourceID: "src-b", Priority: 1, Title: "示例书一", Intro: long},
	}
	d := Decide(s, hits, DefaultOptions())
	if d.Intro == nil || d.Intro.Value != long {
		t.Fatalf("应取最长的简介，实际 %+v", d.Intro)
	}
	if d.Intro.Sources[0] != "src-b" {
		t.Errorf("Sources 应指向给出最长简介的源，实际 %v", d.Intro.Sources)
	}
}

// TestDecideIntroTooShort 覆盖简介不足最小长度：不写。
func TestDecideIntroTooShort(t *testing.T) {
	s := Subject{Title: "示例书一"}
	hits := []Hit{{SourceID: "src-a", Title: "示例书一", Intro: "太短"}}
	d := Decide(s, hits, DefaultOptions())
	if d.Intro != nil {
		t.Fatalf("过短简介不该写，实际 %+v", d.Intro)
	}
	if !containsSkip(d.Skipped, "intro:too_short") {
		t.Errorf("应记录 too_short，实际 %v", d.Skipped)
	}
}

// TestDecideIntroKeepsExisting 覆盖「已有简介够长时不覆盖」：不动用户手填的内容。
func TestDecideIntroKeepsExisting(t *testing.T) {
	existing := strings.Repeat("旧", 100)
	s := Subject{Title: "示例书一", Intro: existing}
	hits := []Hit{{SourceID: "src-a", Title: "示例书一", Intro: strings.Repeat("新", 110)}}
	d := Decide(s, hits, DefaultOptions())
	if d.Intro != nil {
		t.Fatalf("已有 100 字的简介，110 字的新值不该覆盖，实际 %+v", d.Intro)
	}
}

// TestDecideCategoryLocalWins 覆盖「本地词表优先于书源」。
func TestDecideCategoryLocalWins(t *testing.T) {
	s := Subject{Title: "修仙从灵根开始", CategoryL1: Unclassified}
	hits := []Hit{{SourceID: "src-a", Title: "修仙从灵根开始", Kind: "都市/异能"}}
	d := Decide(s, hits, DefaultOptions())
	if d.Category == nil || !strings.HasPrefix(d.Category.Value, "仙侠") {
		t.Fatalf("本地词表应优先，实际 %+v", d.Category)
	}
	if d.Category.Sources[0] != "local:keyword" {
		t.Errorf("来源应标 local:keyword，实际 %v", d.Category.Sources)
	}
}

// TestDecideCategoryFromSource 覆盖词表未命中时用书源 kind。
func TestDecideCategoryFromSource(t *testing.T) {
	s := Subject{Title: "随记", CategoryL1: Unclassified}
	hits := []Hit{
		{SourceID: "src-a", Priority: 10, Title: "随记", Kind: "历史/架空"},
		{SourceID: "src-b", Priority: 1, Title: "随记", Kind: "历史/架空"},
	}
	d := Decide(s, hits, DefaultOptions())
	if d.Category == nil || d.Category.Value != "历史/架空" {
		t.Fatalf("应采纳书源分类，实际 %+v", d.Category)
	}
}

// TestDecideCategoryKeepsExisting 覆盖「已有非未分类时不覆盖」。
func TestDecideCategoryKeepsExisting(t *testing.T) {
	s := Subject{Title: "修仙从灵根开始", CategoryL1: "科幻"}
	d := Decide(s, []Hit{{SourceID: "src-a", Title: "修仙从灵根开始"}}, DefaultOptions())
	if d.Category != nil {
		t.Fatalf("已有分类不该被覆盖，实际 %+v", d.Category)
	}
}

// TestVoteTieRejected 覆盖平票时的保守处置：两个源各说各的（置信度 0.5）
// 低于默认阈值 0.6，一律不采纳——错名比不补更难收拾。
func TestVoteTieRejected(t *testing.T) {
	hits := []Hit{
		{SourceID: "src-a", Priority: 1, Title: "示例书一", Author: "甲作者"},
		{SourceID: "src-b", Priority: 1, Title: "示例书一", Author: "乙作者"},
	}
	if _, ok := vote(hits, func(h Hit) string { return h.Author }, DefaultOptions()); ok {
		t.Fatal("1/1 平票（置信度 0.5）不该采纳")
	}
}

// TestVoteTieBreak 覆盖放宽阈值后的平票选择：取字典序小者，结果可复现。
func TestVoteTieBreak(t *testing.T) {
	hits := []Hit{
		{SourceID: "src-a", Priority: 1, Title: "示例书一", Author: "甲作者"},
		{SourceID: "src-b", Priority: 1, Title: "示例书一", Author: "乙作者"},
	}
	opt := DefaultOptions()
	opt.MinConfidence = 0.4
	v, ok := vote(hits, func(h Hit) string { return h.Author }, opt)
	if !ok {
		t.Fatal("放宽阈值后平票应给出结论")
	}
	again, _ := vote(hits, func(h Hit) string { return h.Author }, opt)
	if v.Value != again.Value {
		t.Errorf("同样输入应给同样结论：%q vs %q", v.Value, again.Value)
	}
}

// TestVoteLowConfidence 覆盖置信度不足：不采纳。
func TestVoteLowConfidence(t *testing.T) {
	hits := []Hit{
		{SourceID: "src-a", Title: "示例书一", Author: "甲作者"},
		{SourceID: "src-b", Title: "示例书一", Author: "乙作者"},
		{SourceID: "src-c", Title: "示例书一", Author: "丙作者"},
	}
	if _, ok := vote(hits, func(h Hit) string { return h.Author }, DefaultOptions()); ok {
		t.Fatal("三个源各说各的（置信度 1/3）不该采纳")
	}
}

// containsSkip 判断跳过原因里有没有某个值。
func containsSkip(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
