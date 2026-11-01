package recommend

import (
	"math/rand"
	"testing"
)

// makeCands 造 n 本互不相同的书（同一作者/分类，避免多样性影响排序断言）。
func makeCands(n int) []Candidate {
	out := make([]Candidate, n)
	for i := range out {
		out[i] = Candidate{BookID: int64(i + 1), HotReads: 5}
	}
	return out
}

func bookIDs(items []Scored) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.BookID
	}
	return out
}

func sameOrder(a, b []Scored) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].BookID != b[i].BookID || a[i].Score != b[i].Score {
			return false
		}
	}
	return true
}

func TestRankEmptyAndBadLimit(t *testing.T) {
	if got := Rank(nil, Signals{}, nil, 10); got != nil {
		t.Fatalf("空候选应返回 nil，得到 %v", got)
	}
	if got := Rank([]Candidate{}, Signals{}, nil, 10); got != nil {
		t.Fatalf("空候选应返回 nil，得到 %v", got)
	}
	if got := Rank(makeCands(3), Signals{}, nil, 0); got != nil {
		t.Fatalf("limit=0 应返回 nil，得到 %v", got)
	}
	if got := Rank(makeCands(3), Signals{}, nil, -5); got != nil {
		t.Fatalf("limit<0 应返回 nil，得到 %v", got)
	}
}

func TestRankKeepsReadButUnfinished(t *testing.T) {
	// 有过 read 事件 ≠ 读完：这类书只降权，仍然可以被推荐。
	cands := []Candidate{
		{BookID: 1, Read: true, HotReads: 100},
		{BookID: 2, HotReads: 1},
		{BookID: 3, Read: true, HotReads: 100},
	}
	got := Rank(cands, Signals{}, rand.New(rand.NewSource(7)), 10)
	if len(got) != 3 {
		t.Fatalf("读过的书不应被过滤，得到 %v", bookIDs(got))
	}
	// 降权而不是过滤：同等条件下「读过」的原始分更低。
	w := DefaultWeights()
	read := rawScore(Candidate{BookID: 9, HotReads: 10, Read: true}, nil, w)
	unread := rawScore(Candidate{BookID: 9, HotReads: 10}, nil, w)
	if read >= unread {
		t.Fatalf("读过的书应被降权: read=%v unread=%v", read, unread)
	}
}

func TestRankFiltersFinishedBooks(t *testing.T) {
	cands := []Candidate{
		{BookID: 1, Finished: true, HotReads: 100},
		{BookID: 2, HotReads: 1},
	}
	got := Rank(cands, Signals{}, rand.New(rand.NewSource(7)), 10)
	if len(got) != 1 || got[0].BookID != 2 {
		t.Fatalf("读完的书应被过滤，得到 %v", bookIDs(got))
	}
}

func TestRankAllFinishedReturnsNil(t *testing.T) {
	cands := []Candidate{{BookID: 1, Finished: true}, {BookID: 2, Finished: true}}
	if got := Rank(cands, Signals{}, rand.New(rand.NewSource(7)), 10); got != nil {
		t.Fatalf("全读完应返回 nil，得到 %v", got)
	}
}

func TestRankKeepsResumeCandidates(t *testing.T) {
	// 续读候选一定带 Read（有进度就有 read 事件），它们靠「Read 不再过滤」存活；
	// 但若已经读完（Finished），续读也不该再推。
	cands := []Candidate{
		{BookID: 1, Read: true, Resume: false, HotReads: 1},   // 读过：留下（降权）
		{BookID: 2, Read: true, Resume: true, HotReads: 1},    // 续读：留下且排首位（+2.0 基础分）
		{BookID: 3, Read: true, Resume: true, Finished: true}, // 已读完：过滤
	}
	got := Rank(cands, Signals{}, rand.New(rand.NewSource(7)), 10)
	if len(got) != 2 {
		t.Fatalf("续读候选应保留、读完的应过滤，得到 %v", bookIDs(got))
	}
	resume := got[0]
	if resume.BookID != 2 {
		t.Fatalf("续读候选应占首位（基础分 2.0），得到 %+v", bookIDs(got))
	}
	if resume.Reason != ReasonResume {
		t.Fatalf("续读候选理由 = %q，期望 %q", resume.Reason, ReasonResume)
	}
}

func TestRankOrdersByInterestAndExplains(t *testing.T) {
	cands := []Candidate{
		{BookID: 1, CategoryL1: "都市"},
		{BookID: 2, CategoryL1: "武侠"},
		{BookID: 3, CategoryL1: "都市"},
	}
	sig := Signals{Interest: map[string]float64{"category_l1:武侠": 2}}
	got := Rank(cands, sig, rand.New(rand.NewSource(3)), 2)
	if len(got) != 2 {
		t.Fatalf("结果条数 = %d，期望 2", len(got))
	}
	if got[0].BookID != 2 {
		t.Fatalf("兴趣书应排第一，得到 %v", bookIDs(got))
	}
	if got[0].Reason != "因为你常看武侠" {
		t.Fatalf("理由 = %q", got[0].Reason)
	}
}

func TestRankLimitIsRespected(t *testing.T) {
	cands := makeCands(10)
	got := Rank(cands, Signals{}, rand.New(rand.NewSource(5)), 4)
	if len(got) != 4 {
		t.Fatalf("结果条数 = %d，期望 4", len(got))
	}
	over := Rank(cands, Signals{}, rand.New(rand.NewSource(5)), 99)
	if len(over) != 10 {
		t.Fatalf("limit 超过候选数时条数 = %d，期望 10", len(over))
	}
}

func TestRankDeterministicWithSeed(t *testing.T) {
	cands := makeCands(8)
	first := Rank(cands, Signals{}, rand.New(rand.NewSource(42)), 6)
	second := Rank(cands, Signals{}, rand.New(rand.NewSource(42)), 6)
	if !sameOrder(first, second) {
		t.Fatalf("同 seed 结果应完全一致: %v vs %v", bookIDs(first), bookIDs(second))
	}
	other := Rank(cands, Signals{}, rand.New(rand.NewSource(43)), 6)
	if sameOrder(first, other) {
		t.Fatal("不同 seed 应得到不同的排序（随机扰动没生效？）")
	}
}

func TestRankNilRandIsDeterministic(t *testing.T) {
	cands := makeCands(6)
	first := Rank(cands, Signals{}, nil, 6)
	second := Rank(cands, Signals{}, nil, 6)
	if !sameOrder(first, second) {
		t.Fatalf("rnd=nil 时结果应可复现: %v vs %v", bookIDs(first), bookIDs(second))
	}
}

func TestRankExploresTenPercent(t *testing.T) {
	cands := makeCands(20)
	got := Rank(cands, Signals{}, rand.New(rand.NewSource(11)), 10)
	if len(got) != 10 {
		t.Fatalf("结果条数 = %d，期望 10", len(got))
	}
	random := 0
	for _, it := range got {
		switch it.Reason {
		case ReasonRandom:
			random++
		case ReasonHot:
		default:
			t.Fatalf("意外理由 %q", it.Reason)
		}
	}
	if random != 1 {
		t.Fatalf("探索条数 = %d，期望 1（limit 的 10%%）", random)
	}
}

func TestRankSmallLimitSkipsExploration(t *testing.T) {
	cands := makeCands(20)
	got := Rank(cands, Signals{}, rand.New(rand.NewSource(11)), 4)
	if len(got) != 4 {
		t.Fatalf("结果条数 = %d，期望 4", len(got))
	}
	for _, it := range got {
		if it.Reason != ReasonHot {
			t.Fatalf("小 limit 不应出现探索项，得到理由 %q", it.Reason)
		}
	}
}

func TestExploreCount(t *testing.T) {
	cases := []struct {
		limit, pool int
		ratio       float64
		want        int
	}{
		{10, 20, 0.1, 1},
		{20, 100, 0.1, 2},
		{9, 100, 0.1, 0},
		{100, 3, 0.1, 1}, // 不超过候选数的一半
		{10, 1, 0.1, 0},
		{0, 10, 0.1, 0},
		{10, 20, 0.5, 5}, // 随机 tab：50% 探索
		{10, 4, 0.5, 2},  // 仍然受「不超过一半」约束
	}
	for _, c := range cases {
		if got := exploreCount(c.limit, c.pool, c.ratio); got != c.want {
			t.Errorf("exploreCount(%d,%d,%v) = %d，期望 %d", c.limit, c.pool, c.ratio, got, c.want)
		}
	}
}

func TestExploreRatioOf(t *testing.T) {
	if got := exploreRatioOf(0); got != defaultExploreRatio {
		t.Fatalf("未填应回落 %v，得到 %v", defaultExploreRatio, got)
	}
	if got := exploreRatioOf(-1); got != defaultExploreRatio {
		t.Fatalf("负数应回落默认值，得到 %v", got)
	}
	if got := exploreRatioOf(2); got != defaultExploreRatio {
		t.Fatalf(">1 应回落默认值，得到 %v", got)
	}
	if got := exploreRatioOf(0.5); got != 0.5 {
		t.Fatalf("显式 0.5 应生效，得到 %v", got)
	}
}

func TestRankRandomTabExploresHalf(t *testing.T) {
	cands := makeCands(20)
	got := Rank(cands, Signals{ExploreRatio: 0.5}, rand.New(rand.NewSource(11)), 10)
	random := 0
	for _, it := range got {
		if it.Reason == ReasonRandom {
			random++
		}
	}
	if random != 5 {
		t.Fatalf("50%% 探索时随机条数 = %d，期望 5", random)
	}
}
