package recommend

import (
	"math/rand"
	"sort"
)

// defaultSeed 是 rnd 为 nil 时的确定性种子。
//
// 为什么允许 rnd=nil 又给固定种子：单元测试与 Gherkin 场景需要「同一批输入同一批输出」，
// 而生产必须传带时钟种子的 rnd；两者共用一套代码时，nil 不能 panic 也不能变成
// 「每次结果不同」的隐藏随机源。
const defaultSeed = 1

// defaultExploreRatio 是 ε-greedy 的默认探索比例（设计 §3：10%）。
const defaultExploreRatio = 0.1

// exploreRatioOf 取本次请求的探索比例：Signals 里越界/未填时回落默认值。
//
// 「随机发现」tab 会显式传 0.5；把它做成信号而不是常量，是因为同一个排序函数
// 要同时服务「推荐要准」与「随机要野」两种互斥目标。
func exploreRatioOf(ratio float64) float64 {
	if ratio <= 0 || ratio > 1 {
		return defaultExploreRatio
	}
	return ratio
}

// Rank 按设计 §3 打分并返回不超过 limit 条结果。
//
// 顺序：已读过滤 → 逐条打分（加权和 × 扰动 + ε 加分）→ 按分数降序 → 尾部抽 10% 随机发现。
// 扰动保证「换一批」有变化，ε 探索保证变化不是「同一批换顺序」。
func Rank(cands []Candidate, sig Signals, rnd *rand.Rand, limit int) []Scored {
	if limit <= 0 || len(cands) == 0 {
		return nil
	}
	w := sig.Weights.resolve()
	if rnd == nil {
		rnd = rand.New(rand.NewSource(defaultSeed))
	}
	scored := make([]Scored, 0, len(cands))
	for _, c := range cands {
		// 只过滤「读完」：Read（有过 read 事件）仅参与打分降权。
		// 早期版本按 Read 过滤，真实数据里几乎每本书都被 read 覆盖过，
		// 候选池会被越吃越小，最后推不出书。
		if c.Finished {
			continue
		}
		scored = append(scored, scoreOf(c, sig.Interest, w, rnd))
	}
	if len(scored) == 0 {
		return nil
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	return withExploration(scored, limit, rnd, exploreRatioOf(sig.ExploreRatio))
}

// scoreOf 给一本书打分：raw 乘随机扰动，再加 ε 探索加分让「有分但不高」的候选有机会。
func scoreOf(c Candidate, interest map[string]float64, w Weights, rnd *rand.Rand) Scored {
	contrib := contributionsOf(c, interest, w)
	score := contrib.total()*(w.JitterBase+w.JitterSpan*rnd.Float64()) + w.Explore*rnd.Float64()
	r := reasonFor(c, contrib, interest)
	return Scored{Candidate: c, Score: score, Reason: r.Text, ReasonCode: r.Code}
}

// withExploration 在排序结果里强制混入 n 条随机发现。
//
// 抽的是「排不进前 limit−n 的尾部」而不是全体候选：全体随机等于拿好结果换噪声，
// 尾部随机才是「给冷门一次机会」——这正是 ε-greedy 在推荐流里的本意。
func withExploration(scored []Scored, limit int, rnd *rand.Rand, ratio float64) []Scored {
	taken := min(limit, len(scored))
	n := exploreCount(limit, len(scored), ratio)
	out := make([]Scored, 0, taken)
	out = append(out, scored[:taken-n]...)
	if n == 0 {
		return out
	}
	rest := scored[taken-n:]
	for _, i := range rnd.Perm(len(rest))[:n] {
		item := rest[i]
		item.Reason = ReasonRandom // 理由必须跟着「怎么被选出来的」走，不能报成兴趣
		item.ReasonCode = CodeRandom
		out = append(out, item)
	}
	return out
}

// exploreCount 计算探索条数：ratio·limit，且不超过候选数的一半。
//
// 「不超过一半」是给候选很少的场景兜底：冷启动只有 3 本书、limit=20 时，
// 若按 10% 算会把 2 条都标成「随机发现」，理由与事实不符。
func exploreCount(limit, pool int, ratio float64) int {
	n := int(float64(limit) * ratio)
	if half := pool / 2; n > half {
		n = half
	}
	return n
}
