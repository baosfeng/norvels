package enrich

import "testing"

// TestNormalizeTitle 覆盖书名归一：书名号/标点/空白/全角/大小写都被抹平。
func TestNormalizeTitle(t *testing.T) {
	same := []string{"示例书一", "《示例书一》", "示例 书一", "【示例书一】", "示例书一."}
	for _, s := range same {
		if got := NormalizeTitle(s); got != "示例书一" {
			t.Errorf("NormalizeTitle(%q) = %q，期望 示例书一", s, got)
		}
	}
	if got := NormalizeTitle("ＳＨＯＷ"); got != "show" {
		t.Errorf("全角大写应归一为半角小写，实际 %q", got)
	}
}

// TestTitleSimBoundary 覆盖相似度的三个端点：完全相同、空串、完全无关。
func TestTitleSimBoundary(t *testing.T) {
	if got := TitleSim("示例书一", "《示例书一》"); got != 1 {
		t.Errorf("归一后相同应得 1，实际 %v", got)
	}
	if got := TitleSim("", "示例书一"); got != 0 {
		t.Errorf("空标题应得 0，实际 %v", got)
	}
	low := TitleSim("示例书一", "完全不同的另一本书")
	if low > 0.3 {
		t.Errorf("完全无关的书名相似度应很低，实际 %v", low)
	}
}

// TestTitleSimClose 覆盖「差一个字」的中段相似度。
func TestTitleSimClose(t *testing.T) {
	sim := TitleSim("示例书一", "示例书二")
	if sim < 0.6 || sim >= 1 {
		t.Errorf("差一个字的相似度应在 (0.6, 1) 内，实际 %v", sim)
	}
}

// TestEditDistance 覆盖编辑距离的边界。
func TestEditDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
	}
	for _, c := range cases {
		if got := editDistance(c.a, c.b); got != c.want {
			t.Errorf("editDistance(%q,%q) = %d，期望 %d", c.a, c.b, got, c.want)
		}
	}
}
