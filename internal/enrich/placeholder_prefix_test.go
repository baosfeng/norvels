package enrich

import "testing"

// TestNormalizeAuthorLabelPrefix 覆盖作者标签前缀的「剥」与「不剥」。
//
// 同一张表里同时放两类用例，让边界一眼可见：
//   - 要剥：标签词（作者 / 著者）+ 冒号（全角：/ 半角:）才剥；
//   - 不剥：以「作者」二字开头、但不带冒号的笔名（书源与生产 authors 表里都有一批
//     这种真名），以及「作者」出现在名字中部的名字——冒号是唯一判别特征，
//     剥错就把真名改坏。此处按仓库隐私红线用虚构笔名表达同一形态。
func TestNormalizeAuthorLabelPrefix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"作者：张三", "张三"},
		{"作者:张三", "张三"},
		{"著者：李四", "李四"},
		{"著者:李四", "李四"},
		{"作者： 张三 ", "张三"},
		{"作者：\u3000张三", "张三"},
		{"作者：王五著", "王五"},
		{"作者：桃小肉", "桃小肉"},
		// 真笔名形态：以「作者」开头但没有冒号，必须原样保留
		{"作者不吃辣", "作者不吃辣"},
		{"作者小七", "作者小七"},
		{"作者取名难", "作者取名难"},
		// 「作者」不在最前面，不是标签
		{"写作者小李", "写作者小李"},
		{"小李作者：甲", "小李作者：甲"},
	}
	for _, c := range cases {
		if got := NormalizeAuthor(c.in); got != c.want {
			t.Errorf("NormalizeAuthor(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestAuthorLabelPrefixEmptyIsPlaceholder 保证「只有标签没有名字」剥完是空串，
// 并被占位判定接住（书源只给一个「作者：」时不能把它写进库）。
func TestAuthorLabelPrefixEmptyIsPlaceholder(t *testing.T) {
	for _, in := range []string{"作者：", "作者:", "著者：", "著者:", "作者： ", "作者：\u3000"} {
		if got := NormalizeAuthor(in); got != "" {
			t.Errorf("NormalizeAuthor(%q) = %q，期望空串", in, got)
		}
		if !IsPlaceholderAuthor(in) {
			t.Errorf("IsPlaceholderAuthor(%q) 应为 true（剥完为空即占位）", in)
		}
	}
}
