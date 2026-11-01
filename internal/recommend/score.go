package recommend

import (
	"math"
	"strconv"
)

// 本文件是设计 §3 的打分公式：raw = 3.0·interest(L1) + 1.5·interest(L2) +
// 2.5·interest(author) + 2.0·log1p(cooccur) + 1.5·log1p(hot_reads) +
// 0.5·log1p(hot_clicks) + 1.0·freshness − 1.0·已读 − 0.5·近期已曝光。
//
// 为什么共读/热度取 log1p 而不是原始次数：热门书的点击可能是冷门的 100 倍，
// 线性相加会让热度一项吃掉全部兴趣信号，推荐流退化成「热榜复制」。
// log1p 把 0→0、1→0.69、100→4.6 压到一个不会压垮其它项的区间。

// contributions 是一本书在各路信号上的加权贡献（可正可负），同时也是理由的依据。
type contributions struct {
	l1        float64
	l2        float64
	author    float64
	cooccur   float64
	hotReads  float64
	hotClicks float64
	fresh     float64
	read      float64
	recent    float64
}

// total 把各分量合成特征加权和（惩罚项按设计为负）。
func (c contributions) total() float64 {
	return c.l1 + c.l2 + c.author + c.cooccur + c.hotReads + c.hotClicks +
		c.fresh - c.read - c.recent
}

// contributionsOf 计算各分量。惩罚项在 total 里减去，这里只存正值。
func contributionsOf(c Candidate, interest map[string]float64, w Weights) contributions {
	return contributions{
		l1:        w.InterestL1 * interestWeight(interest, dimCategory, c.CategoryL1),
		l2:        w.InterestL2 * interestWeight(interest, dimCategory, c.CategoryL2),
		author:    w.InterestAuthor * interestWeight(interest, dimAuthor, authorKey(c.AuthorID)),
		cooccur:   w.Cooccur * math.Log1p(nonNegative(c.CooccurUsers)),
		hotReads:  w.HotReads * math.Log1p(nonNegative(c.HotReads)),
		hotClicks: w.HotClicks * math.Log1p(nonNegative(c.HotClicks)),
		fresh:     w.Freshness * clampUnit(c.Fresh),
		read:      boolWeight(c.Read, w.ReadPenalty),
		recent:    boolWeight(c.RecentShown, w.RecentShownPenalty),
	}
}

// rawScore 是扰动前的原始分，导出给同包之外的测试探查公式，不参与 Rank 的随机化。
func rawScore(c Candidate, interest map[string]float64, w Weights) float64 {
	return contributionsOf(c, interest, w).total()
}

// interestWeight 取某维度的兴趣权重；key 为空表示「这本书没有这个维度」。
func interestWeight(m map[string]float64, dim, key string) float64 {
	if key == "" || len(m) == 0 {
		return 0
	}
	return m[dim+":"+key]
}

// authorKey 把作者 id 转成兴趣 map 的 key 形式；0 表示无作者，不参与作者兴趣。
func authorKey(id int64) string {
	if id <= 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

// nonNegative 把计数夹到非负：负值只可能来自调用方没做完的减法，
// 直接喂给 log1p 会得到 NaN 并污染整条排序结果。
func nonNegative(n int) float64 {
	if n <= 0 {
		return 0
	}
	return float64(n)
}

// clampUnit 把新鲜度夹进 0..1：公式假定它是一个比例，越界说明调用方算错了。
func clampUnit(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

// boolWeight 是布尔惩罚项的取值。
func boolWeight(on bool, w float64) float64 {
	if !on {
		return 0
	}
	return w
}

// reasonOption 是「候选理由」的一个选项：某个信号贡献了多少分、对应哪句话。
type reasonOption struct {
	value  float64
	reason string
}

// reasonFor 挑出贡献最大的那一路信号作为推荐理由。
//
// 用「最大贡献」而不是「命中优先级」：同一条推荐可能同时命中兴趣与热度，
// 理由要能解释用户实际看到的那次排序，而不是永远只报兴趣（排查线上推荐时
// 优先级写法会把真正的驱动信号藏起来）。全为 0 时退化为随机发现。
func reasonFor(c Candidate, sc contributions) string {
	opts := [...]reasonOption{
		{sc.l1 + sc.l2 + sc.author, interestReason(c)},
		{sc.cooccur, ReasonCooccur},
		{sc.hotReads + sc.hotClicks, ReasonHot},
		{sc.fresh, ReasonFresh},
	}
	best := reasonOption{reason: ReasonRandom}
	for _, o := range opts {
		if o.value > best.value {
			best = o
		}
	}
	return best.reason
}

// interestReason 拼兴趣类理由；分类缺失时退化为不带分类的泛化说法。
func interestReason(c Candidate) string {
	for _, cat := range [...]string{c.CategoryL1, c.CategoryL2} {
		if cat != "" {
			return interestPrefix + cat
		}
	}
	return ReasonDefault
}
