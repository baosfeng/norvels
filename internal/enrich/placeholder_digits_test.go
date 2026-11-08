package enrich

import "testing"

// TestDecideAuthorAllDigitsSkipped 覆盖生产 bug：书源把书籍 id 当作者抽了出来
// （纯数字「574981」），多源一致也照样是占位——不产生 author 变更。
func TestDecideAuthorAllDigitsSkipped(t *testing.T) {
	s := Subject{Title: "三字书", Author: "佚名"}
	hits := []Hit{
		{SourceID: "src-a", Priority: 10, Title: "三字书", Author: "574981"},
		{SourceID: "src-b", Priority: 5, Title: "三字书", Author: "574981"},
	}
	d := Decide(s, hits, DefaultOptions())
	if d.Author != nil {
		t.Fatalf("纯数字作者名是占位，不该写库，实际 %+v", d.Author)
	}
	if !containsSkip(d.Skipped, "author:placeholder_only") {
		t.Errorf("应记为占位，实际 %v", d.Skipped)
	}
}

// TestDecideAuthorNameWithDigitsKept 覆盖「含数字的真实笔名必须保留」：
// 判据只能是「全是数字」，不能是「含数字」。
func TestDecideAuthorNameWithDigitsKept(t *testing.T) {
	for _, name := range []string{"秋风123", "三月麻竹", "在上0ca3c4f8170"} {
		s := Subject{Title: "三字书", Author: "佚名"}
		hits := []Hit{
			{SourceID: "src-a", Priority: 10, Title: "三字书", Author: name},
			{SourceID: "src-b", Priority: 5, Title: "三字书", Author: name},
		}
		d := Decide(s, hits, DefaultOptions())
		if d.Author == nil || d.Author.Value != name || !d.Author.Changed {
			t.Fatalf("%q 是真笔名，应正常写入，实际 %+v", name, d.Author)
		}
	}
}

// TestDecideAuthorMixedDigitsKept 覆盖「不是全数字」的形态：123abc 有字母，照常写入。
func TestDecideAuthorMixedDigitsKept(t *testing.T) {
	s := Subject{Title: "三字书", Author: "佚名"}
	hits := []Hit{
		{SourceID: "src-a", Priority: 10, Title: "三字书", Author: "123abc"},
		{SourceID: "src-b", Priority: 5, Title: "三字书", Author: "123abc"},
	}
	d := Decide(s, hits, DefaultOptions())
	if d.Author == nil || d.Author.Value != "123abc" {
		t.Fatalf("123abc 不是全数字，应正常写入，实际 %+v", d.Author)
	}
}

// TestIsPlaceholderAuthorAllDigits 固定「纯数字」判据的边界。
//
// 全角数字（５７４９８１）不算占位：判据刻意只认 ASCII 数字 0-9——书源 id
// 抽出的是 ASCII 串，全角形态只能来自人为输入，把「哪些 Unicode 算数字」也写成
// 规则会让这条判定从「一个字符表 0-9」扩成一张表。若生产出现全角形态，再单独评估。
func TestIsPlaceholderAuthorAllDigits(t *testing.T) {
	yes := []string{"574981", "0", " 574981 "}
	for _, n := range yes {
		if !IsPlaceholderAuthor(n) {
			t.Errorf("%q 归一后全是 ASCII 数字，应判定为占位", n)
		}
	}
	no := []string{"秋风123", "123abc", "abc123", "５７４９８１", "　１２３　", "一二三", "在上0ca3c4f8170"}
	for _, n := range no {
		if IsPlaceholderAuthor(n) {
			t.Errorf("%q 不是全 ASCII 数字，不应判定为占位", n)
		}
	}
}

// TestDecideAllDigitHitCountsInConfidence 记录一个已知的保守行为：
// 纯数字命中不参与投票，但仍计入总命中数，于是「1 个数字 + 1 个真名」的
// 置信度是 0.5，低于默认 0.6 不写。这是既有投票口径（本次不改投票逻辑），
// 结果是保守的「不写」，不会写错。
func TestDecideAllDigitHitCountsInConfidence(t *testing.T) {
	s := Subject{Title: "三字书", Author: "佚名"}
	hits := []Hit{
		{SourceID: "src-a", Priority: 10, Title: "三字书", Author: "574981"},
		{SourceID: "src-b", Priority: 5, Title: "三字书", Author: "作者甲"},
	}
	d := Decide(s, hits, DefaultOptions())
	if d.Author != nil {
		t.Fatalf("置信度被稀释到 0.5，按既有口径不写，实际 %+v", d.Author)
	}
}
