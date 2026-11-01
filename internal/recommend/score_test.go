package recommend

import (
	"math"
	"testing"
)

func TestRawScoreFullFormula(t *testing.T) {
	w := DefaultWeights()
	// L1 与 L2 是两个维度：同一个名字落在不同维度是不同的信号。
	interest := map[string]float64{
		"category_l1:武侠": 2,
		"category_l2:传统": 1,
		"author:12":      1,
	}
	c := Candidate{
		BookID: 1, AuthorID: 12, CategoryL1: "武侠", CategoryL2: "传统",
		CooccurUsers: 3, HotReads: 10, HotClicks: 5, Fresh: 0.5,
	}
	want := 3.0*2 + 1.5*1 + 2.5*1 +
		2.0*math.Log1p(3) + 1.5*math.Log1p(10) + 0.5*math.Log1p(5) + 1.0*0.5
	if got := rawScore(c, interest, w); math.Abs(got-want) > 1e-9 {
		t.Fatalf("rawScore = %v，期望 %v", got, want)
	}
}

func TestRawScoreTagSeriesResume(t *testing.T) {
	w := DefaultWeights()
	interest := map[string]float64{"tag:武侠": 1.5, "series:山河": 1.2}
	c := Candidate{Tags: []string{"武侠", "群像"}, Series: "山河", Resume: true}
	// 标签取命中最大值（1.5×2.0），系列 1.2×1.5，续读 2.0。
	want := 1.5*2.0 + 1.2*1.5 + 2.0
	if got := rawScore(c, interest, w); math.Abs(got-want) > 1e-9 {
		t.Fatalf("rawScore = %v，期望 %v", got, want)
	}
}

func TestRawScorePenalties(t *testing.T) {
	w := DefaultWeights()
	base := Candidate{BookID: 1, HotReads: 4}
	penalized := base
	penalized.Read = true
	shown := base
	shown.RecentShown = true
	both := base
	both.Read, both.RecentShown = true, true

	baseScore := rawScore(base, nil, w)
	if got := rawScore(penalized, nil, w); math.Abs(got-(baseScore-1.0)) > 1e-9 {
		t.Fatalf("已读惩罚缺失: %v", got)
	}
	if got := rawScore(shown, nil, w); math.Abs(got-(baseScore-0.5)) > 1e-9 {
		t.Fatalf("曝光惩罚缺失: %v", got)
	}
	if got := rawScore(both, nil, w); math.Abs(got-(baseScore-1.5)) > 1e-9 {
		t.Fatalf("两项惩罚应叠加: %v", got)
	}
}

func TestClampUnit(t *testing.T) {
	cases := map[float64]float64{-1: 0, 0: 0, 0.25: 0.25, 1: 1, 2: 1}
	for in, want := range cases {
		if got := clampUnit(in); got != want {
			t.Errorf("clampUnit(%v) = %v，期望 %v", in, got, want)
		}
	}
}

func TestNonNegative(t *testing.T) {
	cases := map[int]float64{-5: 0, 0: 0, 3: 3}
	for in, want := range cases {
		if got := nonNegative(in); got != want {
			t.Errorf("nonNegative(%d) = %v，期望 %v", in, got, want)
		}
	}
}

func TestInterestWeight(t *testing.T) {
	m := map[string]float64{"category_l1:武侠": 2}
	if got := interestWeight(m, dimCategoryL1, "武侠"); got != 2 {
		t.Fatalf("命中权重 = %v", got)
	}
	// 同名但维度不同（L2）不能命中：否则 L1/L2 又会在打分里混成一份。
	if got := interestWeight(m, dimCategoryL2, "武侠"); got != 0 {
		t.Fatalf("L2 维度不应命中 L1 的权重，得到 %v", got)
	}
	if got := interestWeight(m, dimCategoryL1, ""); got != 0 {
		t.Fatalf("空 key 应取 0，得到 %v", got)
	}
	if got := interestWeight(m, dimCategoryL1, "仙侠"); got != 0 {
		t.Fatalf("未命中应取 0，得到 %v", got)
	}
	if got := interestWeight(nil, dimCategoryL1, "武侠"); got != 0 {
		t.Fatalf("nil map 应取 0，得到 %v", got)
	}
}

func TestAuthorKey(t *testing.T) {
	if got := authorKey(0); got != "" {
		t.Fatalf("无作者应返回空 key，得到 %q", got)
	}
	if got := authorKey(12); got != "12" {
		t.Fatalf("作者 key = %q，期望 %q", got, "12")
	}
}

func TestBoolWeight(t *testing.T) {
	if got := boolWeight(false, 1.5); got != 0 {
		t.Fatalf("false 应为 0，得到 %v", got)
	}
	if got := boolWeight(true, 1.5); got != 1.5 {
		t.Fatalf("true 应取权重，得到 %v", got)
	}
}

func TestReasonFor(t *testing.T) {
	interest := map[string]float64{"tag:武侠": 1.5, "series:山河": 1.2}
	cases := []struct {
		name string
		cand Candidate
		sc   contributions
		want string
	}{
		{"兴趣最强", Candidate{CategoryL1: "武侠"}, contributions{l1: 6}, "因为你常看武侠"},
		{"二级分类兜底", Candidate{CategoryL2: "传统"}, contributions{l2: 6}, "因为你常看传统"},
		{"无分类时泛化", Candidate{}, contributions{author: 5}, ReasonDefault},
		{"标签最强", Candidate{Tags: []string{"武侠", "群像"}}, contributions{tag: 3}, "因为你喜欢「武侠」标签"},
		{"系列最强", Candidate{Series: "山河"}, contributions{series: 3}, "同系列《山河》"},
		{"续读最强", Candidate{}, contributions{resume: 2}, ReasonResume},
		{"共读最强", Candidate{CategoryL1: "武侠"}, contributions{l1: 1, cooccur: 5}, ReasonCooccur},
		{"热度最强", Candidate{CategoryL1: "武侠"}, contributions{l1: 1, hotReads: 2, hotClicks: 3}, ReasonHot},
		{"新鲜最强", Candidate{}, contributions{fresh: 3}, ReasonFresh},
		{"全零", Candidate{}, contributions{}, ReasonRandom},
	}
	for _, c := range cases {
		if got := reasonFor(c.cand, c.sc, interest); got != c.want {
			t.Errorf("%s: reason = %q，期望 %q", c.name, got, c.want)
		}
	}
}

func TestBestTagWeight(t *testing.T) {
	interest := map[string]float64{"tag:武侠": 1.5, "tag:群像": 3}
	if got := bestTagWeight([]string{"武侠", "群像"}, interest); got != 3 {
		t.Fatalf("应取命中标签的最大权重 3，得到 %v", got)
	}
	if got := bestTagWeight([]string{"科幻"}, interest); got != 0 {
		t.Fatalf("未命中应为 0，得到 %v", got)
	}
	if got := bestTagWeight(nil, interest); got != 0 {
		t.Fatalf("无标签应为 0，得到 %v", got)
	}
	if got := bestTag([]string{"武侠", "群像"}, interest); got != "群像" {
		t.Fatalf("最强标签 = %q，期望 群像", got)
	}
}
