package store

import (
	"strings"
	"testing"
)

// 本文件锁死 reviewdiff.go 的纯函数口径：空白归一、公共前后缀、句子切分、上下文拼接、
// 以及审核列表与理由 JSON 的边界归一化。

// TestSquashAndSameText 只差排版的两版正文视为「实质相同」。
func TestSquashAndSameText(t *testing.T) {
	cases := []struct {
		a, b string
		same bool
	}{
		{"正文", "正文", true},
		{"正 文", "正文", true},
		{"正\n文", "正文", true},
		{"正　文", "正文", true}, // 全角空格
		{"正文甲", "正文乙", false},
	}
	for _, tc := range cases {
		if got := sameChapterText(tc.a, tc.b); got != tc.same {
			t.Errorf("sameChapterText(%q,%q) = %v，期望 %v", tc.a, tc.b, got, tc.same)
		}
	}
	if got := squashSpaces("甲 乙\t丙\n丁\r戊　己"); got != "甲乙丙丁戊己" {
		t.Errorf("squashSpaces = %q，期望去掉全部空白", got)
	}
}

// TestCommonAffixes 公共前后缀不重叠：前后缀相加不超过较短一侧的长度。
func TestCommonAffixes(t *testing.T) {
	a := []rune("甲乙丙丁")
	b := []rune("甲乙丙戊")
	prefix := commonPrefixRunes(a, b)
	if prefix != 3 {
		t.Fatalf("公共前缀 = %d，期望 3", prefix)
	}
	if got := commonSuffixRunes(a, b, prefix); got != 0 {
		t.Errorf("公共后缀 = %d，期望 0（与前缀重叠的部分不算）", got)
	}
	same := []rune("甲乙")
	if got := commonPrefixRunes(same, same); got != 2 {
		t.Errorf("完全相同的前缀 = %d，期望 2", got)
	}
	if got := commonSuffixRunes(same, same, 2); got != 0 {
		t.Errorf("完全相同的后缀 = %d，期望 0（前缀已吃满）", got)
	}
	if got := commonSuffixRunes([]rune("甲乙丙"), []rune("丙"), 0); got != 1 {
		t.Errorf("单字后缀 = %d，期望 1", got)
	}
}

// TestSentenceHelpers 前一句 / 后一句的切分与无标点兜底。
func TestSentenceHelpers(t *testing.T) {
	if got := lastSentence("第一句。第二句"); got != "第二句" {
		t.Errorf("lastSentence = %q，期望「第二句」", got)
	}
	// 差异正好落在句号之后：仍然要给「前一句」，不能返回空串。
	if got := lastSentence("前一句。"); got != "前一句" {
		t.Errorf("以句号结尾的 lastSentence = %q，期望「前一句」", got)
	}
	if got := lastSentence(""); got != "" {
		t.Errorf("空串 lastSentence = %q，期望空", got)
	}
	if got := firstSentence("第一句。第二句"); got != "第一句。" {
		t.Errorf("firstSentence = %q，期望「第一句。」", got)
	}
	if got := firstSentence(""); got != "" {
		t.Errorf("空串 firstSentence = %q，期望空", got)
	}
	// 没有句末标点时按 200 rune 截断（尾部 / 头部各取一侧）。
	long := strings.Repeat("甲", 300)
	if got := []rune(lastSentence(long)); len(got) != 200 {
		t.Errorf("无标点 lastSentence 长度 = %d，期望 200", len(got))
	}
	if got := []rune(firstSentence(long)); len(got) != 200 {
		t.Errorf("无标点 firstSentence 长度 = %d，期望 200", len(got))
	}
	if got := tailRunes([]rune("甲乙"), 0); got != "甲乙" {
		t.Errorf("tailRunes(n=0) = %q，期望原样返回", got)
	}
	if got := headRunes([]rune("甲乙"), 5); got != "甲乙" {
		t.Errorf("headRunes(n>len) = %q，期望原样返回", got)
	}
}

// TestDiffContext 上下文拼接的四种组合（都有 / 只有前 / 只有后 / 都没有）。
func TestDiffContext(t *testing.T) {
	both := diffContext([]rune("前一句。"), []rune("后一句。"))
	if !strings.Contains(both, "前一句") || !strings.Contains(both, "后一句") {
		t.Errorf("both = %q，期望前后各一句", both)
	}
	if got := diffContext(nil, []rune("后一句。")); got != "后一句。" {
		t.Errorf("只有后一句 = %q", got)
	}
	if got := diffContext([]rune("前一句。"), nil); got != "前一句" {
		t.Errorf("只有前一句 = %q", got)
	}
	if got := diffContext(nil, nil); got != "" {
		t.Errorf("都没有 = %q，期望空串", got)
	}
}

// TestReviewLimitAndFilter 条数归一化与状态筛选。
func TestReviewLimitAndFilter(t *testing.T) {
	if got := normReviewLimit(0); got != defaultReviewLimit {
		t.Errorf("默认条数 = %d，期望 %d", got, defaultReviewLimit)
	}
	if got := normReviewLimit(maxReviewLimit + 100); got != maxReviewLimit {
		t.Errorf("超限条数 = %d，期望夹到 %d", got, maxReviewLimit)
	}
	if got := normReviewLimit(7); got != 7 {
		t.Errorf("正常条数 = %d，期望 7", got)
	}
	if where, args := reviewStatusFilter(""); where != nil || args != nil {
		t.Errorf("空状态 = %v/%v，期望不筛", where, args)
	}
	if where, _ := reviewStatusFilter(ReviewStatusAll); where != nil {
		t.Errorf("all = %v，期望不筛", where)
	}
	if where, args := reviewStatusFilter(ReviewStatusReady); len(where) != 1 || len(args) != 1 {
		t.Errorf("ready = %v/%v，期望一条条件", where, args)
	}
}

// TestReviewReasonJSON 理由 JSON 的编解码与坏数据兜底。
func TestReviewReasonJSON(t *testing.T) {
	raw, err := encodeReasons(nil)
	if err != nil || raw != "[]" {
		t.Errorf("nil 编码 = %q, err=%v，期望 []", raw, err)
	}
	if got := decodeReasons(""); len(got) != 0 {
		t.Errorf("空串解码 = %v，期望空切片", got)
	}
	if got := decodeReasons("{坏}"); len(got) != 0 {
		t.Errorf("坏 JSON 解码 = %v，期望空切片（不让一条脏行废掉整页）", got)
	}
	if got := decodeReasons("null"); len(got) != 0 {
		t.Errorf("null 解码 = %v，期望空切片", got)
	}
	if got := decodeReasons(`["a","b"]`); len(got) != 2 {
		t.Errorf("正常解码 = %v，期望两条", got)
	}
}

// TestReviewIDAndRatioHelpers 去重 id 与比率夹取。
func TestReviewIDAndRatioHelpers(t *testing.T) {
	ids := uniquePositiveIDs([]int64{3, 0, -1, 3, 5})
	if len(ids) != 2 || ids[0] != 3 || ids[1] != 5 {
		t.Errorf("uniquePositiveIDs = %v，期望 [3 5]", ids)
	}
	if got := clampRatio(-0.5); got != 0 {
		t.Errorf("负比率 = %v，期望 0", got)
	}
	if got := clampRatio(1.5); got != 1 {
		t.Errorf("超限比率 = %v，期望 1", got)
	}
	if got := clampRatio(0.25); got != 0.25 {
		t.Errorf("正常比率 = %v，期望 0.25", got)
	}
}
