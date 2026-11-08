package enrich

import "testing"

// TestClassifyByTitle 覆盖书名命中的主路径。
func TestClassifyByTitle(t *testing.T) {
	l1, l2, ok := Classify("修仙从灵根开始", "")
	if !ok || l1 != "仙侠" || l2 == "" {
		t.Fatalf("书名含「修仙」应归仙侠/修真，实际 %q/%q ok=%v", l1, l2, ok)
	}
}

// TestClassifyByIntro 覆盖书名不命中、简介命中。
func TestClassifyByIntro(t *testing.T) {
	l1, _, ok := Classify("某本没线索的书", "这是一个校园里兵王回归的故事，……")
	if !ok || l1 != "都市" {
		t.Fatalf("简介含「兵王」应归都市，实际 %q ok=%v", l1, ok)
	}
}

// TestClassifyMiss 覆盖词表未命中：必须 ok=false，不能瞎猜。
func TestClassifyMiss(t *testing.T) {
	if l1, l2, ok := Classify("随记", "一些与题材无关的说明文字"); ok {
		t.Fatalf("未命中不该给分类，实际 %q/%q", l1, l2)
	}
}

// TestSplitJoinCategory 覆盖分类串的拆合往返。
func TestSplitJoinCategory(t *testing.T) {
	if l1, l2 := SplitCategory("玄幻/东方玄幻"); l1 != "玄幻" || l2 != "东方玄幻" {
		t.Fatalf("拆分错误: %q/%q", l1, l2)
	}
	if l1, l2 := SplitCategory("玄幻"); l1 != "玄幻" || l2 != "" {
		t.Fatalf("无分隔符时应只给 L1: %q/%q", l1, l2)
	}
	for _, v := range []string{"玄幻/东方玄幻", "玄幻", ""} {
		l1, l2 := SplitCategory(v)
		if JoinCategory(l1, l2) != v {
			t.Errorf("拆合往返不一致: %q → %q", v, JoinCategory(l1, l2))
		}
	}
}

// TestHeadRunes 覆盖按 rune 截断，不能切坏汉字。
func TestHeadRunes(t *testing.T) {
	if got := headRunes("中文示例", 2); got != "中文" {
		t.Errorf("headRunes = %q", got)
	}
	if got := headRunes("短", 10); got != "短" {
		t.Errorf("不足长度应原样返回，实际 %q", got)
	}
}
