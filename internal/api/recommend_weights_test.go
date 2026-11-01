package api

import (
	"math/rand"
	"testing"

	"norvels/internal/recommend"
)

// 本文件锁死 P4-8 的重量级证据：三项配置打开后**排序真的改变**，关闭时与接入前一致。
// 「配置 → 权重」由 rankWeights 断言，「权重 → 排序」由对 Rank 的定种子调用断言：
// 中间没有别的分支，两段接起来就是端到端的接线证明。

// floatPtr 造一个覆盖值（api.Config 用指针区分「没配」与「配成 0」）。
func floatPtr(v float64) *float64 { return &v }

// TestRankWeightsDefaultsWhenUnset 未配置时逐位等于 DefaultWeights，默认行为不变。
func TestRankWeightsDefaultsWhenUnset(t *testing.T) {
	e := newEnv(t)
	if got := e.srv.rankWeights(); got != recommend.DefaultWeights() {
		t.Fatalf("未配置时权重 = %+v，期望默认权重", got)
	}
}

// TestRankWeightsAppliesOverrides 配了就覆盖对应维度，没配的维度保持默认。
func TestRankWeightsAppliesOverrides(t *testing.T) {
	e := newEnvWith(t, Config{
		Version:             "weights",
		DisableAccessLog:    true,
		RecWeightInterestL1: floatPtr(9),
		RecWeightHotReads:   floatPtr(0), // 显式 0 = 关掉热度这一路
	}, testFS())
	got := e.srv.rankWeights()
	want := recommend.DefaultWeights()
	want.InterestL1 = 9
	want.HotReads = 0
	if got != want {
		t.Fatalf("权重覆盖结果 = %+v，期望 %+v", got, want)
	}
	if got.Freshness != recommend.DefaultWeights().Freshness {
		t.Fatalf("未配置的维度不应被改写: Freshness=%v", got.Freshness)
	}
}

// TestRankWeightsChangeRanking 权重真的进了排序：固定种子、同一批候选，
// 只把兴趣权重压到 0，兴趣书的排名就掉到热度书之后。
//
// 候选刻意做成「兴趣书只吃 L1 兴趣、热度书只吃热度」，两条路互不重叠，
// 因此排序翻转只可能来自权重变化，不可能是别的信号碰巧抵消。
func TestRankWeightsChangeRanking(t *testing.T) {
	interestBook := recommend.Candidate{BookID: 1, CategoryL1: "武侠", HotReads: 1}
	hotBook := recommend.Candidate{BookID: 2, CategoryL1: "都市", HotReads: 40}
	cands := []recommend.Candidate{interestBook, hotBook}
	interest := map[string]float64{"category_l1:武侠": 3}

	withDefault := recommend.Rank(cands,
		recommend.Signals{Interest: interest, Weights: recommend.DefaultWeights()},
		rand.New(rand.NewSource(7)), 10)
	noInterest := recommend.Rank(cands,
		recommend.Signals{Interest: interest, Weights: noInterestWeights(t)},
		rand.New(rand.NewSource(7)), 10)
	if len(withDefault) != 2 || len(noInterest) != 2 {
		t.Fatalf("候选都应被排进结果: %d / %d", len(withDefault), len(noInterest))
	}
	if withDefault[0].BookID != 1 {
		t.Fatalf("默认权重下兴趣书应排第一，得到 %d", withDefault[0].BookID)
	}
	if noInterest[0].BookID != 2 {
		t.Fatalf("关掉兴趣权重后热度书应排第一，得到 %d（权重没生效）", noInterest[0].BookID)
	}
}

// noInterestWeights 模拟「把兴趣权重配成 0」：走 api.Config 的同一条覆盖路径。
func noInterestWeights(t *testing.T) recommend.Weights {
	t.Helper()
	e := newEnvWith(t, Config{Version: "weights", DisableAccessLog: true,
		RecWeightInterestL1: floatPtr(0)}, testFS())
	return e.srv.rankWeights()
}
