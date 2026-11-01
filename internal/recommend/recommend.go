// Package recommend 是推荐流的纯函数内核：打分、随机扰动、ε 探索、多样性重排。
//
// 为什么做成无副作用纯函数：召回要读主库（候选书）和推荐库（兴趣/共读/热度），
// 而排序只该依赖「候选 + 信号」这两份输入。把打分和重排抽成纯函数后，
// 「同一份信号给出同一批结果」可以直接用固定 seed 断言，不必起库、不必起 HTTP。
//
// 依赖边界：只依赖标准库。
package recommend

import (
	"math"
	"time"
)

// Candidate 是一本召回候选书。
//
// Fresh 由调用方用 Freshness 从 ImportedAt 折算好再传进来：排序本身不该
// 关心「现在几点」，否则同一份候选在不同时刻打分不同，测试无法复现。
type Candidate struct {
	BookID       int64
	AuthorID     int64
	CategoryL1   string
	CategoryL2   string
	ImportedAt   time.Time // 仅作展示/调试用途，排序只消费 Fresh
	Rating       int       // 用户评分（0 = 未评分）；当前不参与公式，留给后续多目标排序
	Read         bool      // 已读 → 直接过滤，不推荐
	RecentShown  bool      // 近期已曝光 → 扣分（不是不过滤：重复推同一批是常态）
	CooccurUsers int       // 共读人数（同读这本书的人有多少）
	HotReads     int       // 近 N 天阅读次数
	HotClicks    int       // 近 N 天点击次数
	Fresh        float64   // 新鲜度 0..1，见 Freshness
}

// Weights 是打分权重，默认值来自设计 §3；调用方可按需整体替换（例如冷启动只留热度）。
//
// 为什么权重是可配置结构体而不是常量：线上调权重是最常做的运营动作，
// 改常量要重新发版；而把它建模成一份输入后，调权只影响一次请求。
type Weights struct {
	InterestL1         float64 // 一级分类兴趣
	InterestL2         float64 // 二级分类兴趣
	InterestAuthor     float64 // 作者兴趣
	Cooccur            float64 // 共读强度（log 压缩）
	HotReads           float64 // 近期阅读热度（log 压缩）
	HotClicks          float64 // 近期点击热度（log 压缩）
	Freshness          float64 // 新鲜度
	ReadPenalty        float64 // 已读惩罚（Rank 已过滤已读，保留它是为了公式完整可解释）
	RecentShownPenalty float64 // 近期已曝光惩罚
	JitterBase         float64 // 随机扰动基数（0.8）
	JitterSpan         float64 // 随机扰动幅度（0.4）
	Explore            float64 // ε 探索加分
}

// DefaultWeights 返回设计 §3 冻结的默认权重。
func DefaultWeights() Weights {
	return Weights{
		InterestL1:         3.0,
		InterestL2:         1.5,
		InterestAuthor:     2.5,
		Cooccur:            2.0,
		HotReads:           1.5,
		HotClicks:          0.5,
		Freshness:          1.0,
		ReadPenalty:        1.0,
		RecentShownPenalty: 0.5,
		JitterBase:         0.8,
		JitterSpan:         0.4,
		Explore:            0.6,
	}
}

// resolve 把零值 Weights 换成默认权重。
//
// 只有「整份为零」才替换：逐字段补默认值会让「显式关掉某一路信号」变成不可能，
// 而冷启动正是靠关掉兴趣/共读只留热度来实现的。
func (w Weights) resolve() Weights {
	if w == (Weights{}) {
		return DefaultWeights()
	}
	return w
}

// Signals 是一次推荐请求的外部信号：用户兴趣权重 + 打分权重。
type Signals struct {
	// Interest 的 key 与 recstore.Interests 同口径："category:武侠" / "author:12"。
	Interest map[string]float64
	Weights  Weights
}

// Scored 是一条打完分的结果：分数 + 给用户看的推荐理由。
type Scored struct {
	Candidate
	Score  float64
	Reason string
}

// 推荐理由文案（设计 §3 冻结）。
//
// 文案是后端直出的：前端只负责展示，不在客户端拼中文——多端（PC/H5）
// 各拼一次就会出现两套措辞。
const (
	ReasonCooccur = "读过这本书的人也在看"
	ReasonHot     = "最近很多人读"
	ReasonFresh   = "最近入库"
	ReasonRandom  = "随机发现"
	ReasonDefault = "猜你喜欢"
)

// interestPrefix 是兴趣类理由的前缀，后面接分类名，例如「因为你常看武侠」。
const interestPrefix = "因为你常看"

// 维度前缀，必须与 recstore 的 dim 常量一致（两包不互相 import，
// 一致性的保证是各自的单测里都断言了同一个字符串）。
const (
	dimCategory = "category"
	dimAuthor   = "author"
)

// freshHalfLifeDays 是新鲜度衰减的时间常数：14 天前的书新鲜度约 0.37。
//
// 与兴趣权重同一量级（14 天）：一本两周没被翻过的书，「最近入库」这条理由
// 就基本不该再成立了。
const freshHalfLifeDays = 14.0

// Freshness 把入库时间折算成 0..1 的新鲜度（14 天指数衰减）。
//
// 未来时间（客户端/导入器时钟偏差）按 1 处理：宁可当新书，也别算出 >1 的
// 分值把新鲜度项变成「越早越新」。
func Freshness(importedAt, now time.Time) float64 {
	if importedAt.IsZero() {
		return 0
	}
	elapsed := now.Sub(importedAt)
	if elapsed <= 0 {
		return 1
	}
	return math.Exp(-elapsed.Hours() / (24 * freshHalfLifeDays))
}
