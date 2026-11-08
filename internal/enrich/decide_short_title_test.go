package enrich

import "testing"

// TestDecideAuthorSkippedForTwoRuneTitle 覆盖生产错配的核心样本：
// 两字泛化书名 + 多源一致的同名命中，相似度恒为 1.00、投票也一致，
// 但同名不代表同一本，因此不产生 author 变更，只留可解释的跳过原因。
func TestDecideAuthorSkippedForTwoRuneTitle(t *testing.T) {
	s := Subject{Title: "快穿", Author: "佚名"}
	hits := []Hit{
		{SourceID: "src-a", Priority: 10, Title: "快穿", Author: "作者甲"},
		{SourceID: "src-b", Priority: 5, Title: "快穿", Author: "作者甲"},
	}
	d := Decide(s, hits, DefaultOptions())
	if d.Author != nil {
		t.Fatalf("两字书名不该补在线作者，实际 %+v", d.Author)
	}
	if !containsSkip(d.Skipped, "author:title_too_short") {
		t.Errorf("应记录 author:title_too_short，实际 %v", d.Skipped)
	}
}

// TestDecideAuthorThreeRuneTitleAllowed 覆盖阈值边界：归一后 3 字仍走原逻辑（照常变更）。
func TestDecideAuthorThreeRuneTitleAllowed(t *testing.T) {
	s := Subject{Title: "三字书", Author: "佚名"}
	hits := []Hit{
		{SourceID: "src-a", Priority: 10, Title: "三字书", Author: "作者甲"},
		{SourceID: "src-b", Priority: 5, Title: "三字书", Author: "作者甲"},
	}
	d := Decide(s, hits, DefaultOptions())
	if d.Author == nil || d.Author.Value != "作者甲" || !d.Author.Changed {
		t.Fatalf("3 字书名应照原逻辑产生变更，实际 %+v", d.Author)
	}
	if containsSkip(d.Skipped, "author:title_too_short") {
		t.Errorf("3 字书名不该记过短，实际 %v", d.Skipped)
	}
}

// TestDecideAuthorShortAfterNormalize 覆盖「判的是归一后长度」：
// 《快 穿》/【快穿】/全角空格的原始 rune 数都 > 2，归一后都是 2 字，仍然跳过。
func TestDecideAuthorShortAfterNormalize(t *testing.T) {
	for _, title := range []string{"《快 穿》", "【快穿】", "快　穿"} {
		s := Subject{Title: title, Author: "佚名"}
		hits := []Hit{
			{SourceID: "src-a", Priority: 10, Title: title, Author: "作者甲"},
			{SourceID: "src-b", Priority: 5, Title: title, Author: "作者甲"},
		}
		d := Decide(s, hits, DefaultOptions())
		if d.Author != nil {
			t.Fatalf("%q 归一后仍是 2 字，不该补在线作者，实际 %+v", title, d.Author)
		}
		if !containsSkip(d.Skipped, "author:title_too_short") {
			t.Errorf("%q 应记录 author:title_too_short，实际 %v", title, d.Skipped)
		}
	}
}

// TestDecideShortTitleStillClassifies 覆盖「只影响 author」：
// 同一本书作者被跳过时，分类仍按本地词表正常产生。
func TestDecideShortTitleStillClassifies(t *testing.T) {
	s := Subject{Title: "末世", Author: "佚名", CategoryL1: Unclassified}
	hits := []Hit{{SourceID: "src-a", Priority: 10, Title: "末世", Author: "作者甲"}}
	d := Decide(s, hits, DefaultOptions())
	if d.Author != nil {
		t.Fatalf("两字书名不该补在线作者，实际 %+v", d.Author)
	}
	if d.Category == nil || d.Category.Value != "科幻/末世" || !d.Category.Changed {
		t.Fatalf("作者被跳过不影响分类，实际 %+v", d.Category)
	}
}

// TestTooShortForOnlineAuthorThreshold 覆盖阈值来自 Options：调大后 3 字书名也算过短，
// 削掉后 2 字书名放行——判定不写死在函数里。
func TestTooShortForOnlineAuthorThreshold(t *testing.T) {
	opt := DefaultOptions()
	opt.ShortTitleMaxRunes = 3
	hits := []Hit{
		{SourceID: "src-a", Priority: 10, Title: "三字书", Author: "作者甲"},
		{SourceID: "src-b", Priority: 5, Title: "三字书", Author: "作者甲"},
	}
	d := Decide(Subject{Title: "三字书", Author: "佚名"}, hits, opt)
	if d.Author != nil || !containsSkip(d.Skipped, "author:title_too_short") {
		t.Fatalf("阈值调到 3 后 3 字书名应跳过，实际 %+v / %v", d.Author, d.Skipped)
	}
	opt.ShortTitleMaxRunes = 1
	d = Decide(Subject{Title: "快穿", Author: "佚名"}, []Hit{
		{SourceID: "src-a", Priority: 10, Title: "快穿", Author: "作者甲"},
		{SourceID: "src-b", Priority: 5, Title: "快穿", Author: "作者甲"},
	}, opt)
	if d.Author == nil || d.Author.Value != "作者甲" {
		t.Fatalf("阈值调到 1 后 2 字书名应放行，实际 %+v", d.Author)
	}
}

// TestTooShortForOnlineAuthor 覆盖判定本身：归一后的 rune 长度是唯一判据。
func TestTooShortForOnlineAuthor(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{"", true},
		{"快穿", true},
		{"《快 穿》", true},
		{"【快穿】", true},
		{"三字书", false},
		{"示例书一", false},
	}
	for _, c := range cases {
		if got := TooShortForOnlineAuthor(c.title, DefaultOptions().ShortTitleMaxRunes); got != c.want {
			t.Errorf("TooShortForOnlineAuthor(%q) = %v，期望 %v", c.title, got, c.want)
		}
	}
}
