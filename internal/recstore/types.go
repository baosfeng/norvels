// Package recstore 是推荐系统的独立数据层。
//
// 为什么单独一个库（norvels-rec.db）而不是塞进主库：推荐数据全是「可重建的辅助数据」——
// 埋点明细、兴趣权重、共读计数都能从事件重放出来，坏了删库重来即可。独立成库后，
// 推荐侧的写放大、VACUUM、损坏都不会牵连 books / chapters 这些不可重建的主数据，
// 主库的迁移体系也不必为一张可能整表重建的统计表停服迁移（设计 §1）。
//
// 依赖边界：本包只依赖标准库 + modernc sqlite，不 import 其它 internal 包。
// 推荐库与主库之间没有外键、没有跨库事务：books / authors 的 id 在本库里只是
// 「弱引用」，候选书的标题、封面色等展示字段由 api 层回主库补齐。
package recstore

import "errors"

// 事件类型（与前端埋点 SDK 的 kind 一一对应，设计 §4）。
//
// 只有 impression 走「聚合不落明细」：曝光是所有 kind 里量最大的（一次滚动屏就是
// 几十条），逐条落 events 会把库推到 GB 级而查询根本用不到明细；点击、阅读这类
// 信号量级小一个数量级，留明细才能回溯「这个兴趣分是怎么来的」。
const (
	KindImpression = "impression"
	KindClick      = "click"
	KindRead       = "read"
	KindView       = "view"
	KindSearch     = "search"
	KindShelf      = "shelf"
	KindRating     = "rating"
)

// MaxEventBatch 是单次上报的条数上限（设计 §4：≤100 条/次）。
//
// 上限放在数据层而不是只由 api 校验：推荐库的写放大靠「一个请求一个事务」兜底，
// 允许一次塞一万条等于让单个请求长时间持有写锁，前端一个异常重试就能把服务卡住。
const MaxEventBatch = 100

// ErrInvalidArgument 表示调用方参数不合法（未登录/未知 kind/超长字段等）。
//
// 用哨兵错误而不是各写各的 fmt.Errorf：api 层要据此返回 400，不能把「客户端传错」
// 和「库写失败」混成同一个 500。
var ErrInvalidArgument = errors.New("recstore: 参数非法")

// Event 是一条埋点事件。
//
// TS 为空表示「服务端当前时间」：前端离线上报补发时可能不带 ts，用服务端时间
// 至少保证事件顺序单调，不会因为客户端时钟乱序把衰减算成负值。
type Event struct {
	Kind       string `json:"kind"`
	BookID     int64  `json:"bookId,omitempty"`
	AuthorID   int64  `json:"authorId,omitempty"`
	CategoryL1 string `json:"categoryL1,omitempty"`
	CategoryL2 string `json:"categoryL2,omitempty"`
	Value      int    `json:"value,omitempty"` // read = 停留秒数，rating = 星级
	Extra      string `json:"extra,omitempty"`
	TS         string `json:"ts,omitempty"`
}

// Cooccur 是一条共读记录：读过 BookID 的人也读过 OtherID，累计 Users 次。
//
// 调用方（api 的共读召回路）按 OtherID 聚合取 Users 最大值填进
// recommend.Candidate.CooccurUsers——同一本邻居书可能由多本已读书同时贡献。
type Cooccur struct {
	BookID  int64 `json:"bookId"`
	OtherID int64 `json:"otherId"`
	Users   int   `json:"users"`
}

// HotBook 是近 N 天的一本书热度（按 book_daily 汇总，设计 §2）。
type HotBook struct {
	BookID  int64 `json:"bookId"`
	Reads   int   `json:"reads"`
	Clicks  int   `json:"clicks"`
	Seconds int   `json:"seconds"`
}

// Stats 是推荐库的诊断计数（/api/health 与运维排查用，不参与推荐）。
type Stats struct {
	Events      int `json:"events"`
	Books       int `json:"books"`
	Users       int `json:"users"`
	Interests   int `json:"interests"`
	Cooccur     int `json:"cooccur"`
	RecentItems int `json:"recentItems"`
}

// 查询与归一化口径的常量。
const (
	// decayDays 是兴趣权重的指数衰减口径（设计 §2：w = w·exp(-Δt/14d) + delta）。
	// 14 天取自「一次阅读偏好大致能延续两周」的经验值，不做自适应。
	decayDays = 14.0

	// minDecayedWeight 是读回兴趣时的截断阈值：衰减到 1e-6 以下的分量对排序
	// 已无影响（3.0 的权重也被乘成 3e-6），留在 map 里只会让调用方多遍历。
	minDecayedWeight = 1e-6

	// maxFieldLen / maxExtraLen 是字段上限，超长直接判 400 而不是静默截断：
	// 截断后的事件仍然落库，会让「分类名」在库里出现两个不同的半截值。
	maxFieldLen = 64
	maxExtraLen = 512

	// maxReadSeconds 是单条 read 事件的停留秒数上限（一天）。
	// 客户端切后台没上报会导致超长停留，夹住它避免兴趣分被单条事件带飞。
	maxReadSeconds = 86400

	defaultCooccurLimit = 50
	defaultHotDays      = 7
	maxHotDays          = 365
	maxQueryLimit       = 500
)
