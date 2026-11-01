// Package recall 是多路召回：按 tab 从主库与推荐库取候选书，产出打分的输入。
//
// 为什么单独成包：召回要同时读两个库（主库候选 + 推荐库信号），但它并不需要
// api.Server 的任何其它东西。把「读库」收敛到 Port / RecPort 两个接口后，
// 召回能脱离 HTTP 独立测试，Server 的装配变化也不再牵动召回逻辑。
//
// 分层位置：api（HTTP + 打分 + 装配响应）→ recall（本包，取候选）→
// recommend（纯函数打分）。本包不 import internal/api（否则成环）。
package recall

import (
	"context"

	"norvels/internal/recstore"
	"norvels/internal/store"
)

// Port 是召回需要的主库只读查询（由 api 侧适配到 *store.Store）。
//
// 只列召回真正用到的五个方法：接口越窄，新调用越难悄悄绕过这一层。
type Port interface {
	ListBooks(ctx context.Context, q store.BookQuery) (store.Page[store.BookSummary], error)
	BooksByIDs(ctx context.Context, ids []int64) ([]store.BookSummary, error)
	BookTagsByIDs(ctx context.Context, ids []int64) (map[int64][]string, error)
	ListReadingProgress(ctx context.Context, limit int) ([]store.ReadingProgress, error)
	RandomBookSummaries(ctx context.Context, n int) ([]store.BookSummary, error)
}

// RecPort 是召回需要的推荐库信号（由 api 侧适配到 *recstore.Store）。
type RecPort interface {
	Cooccur(ctx context.Context, userID int64, limit int) ([]recstore.Cooccur, error)
	CooccurForBook(ctx context.Context, bookID int64, limit int) ([]recstore.Cooccur, error)
	HotBooks(ctx context.Context, days, limit int) ([]recstore.HotBook, error)
}

// 推荐 tab。字面量与 api 侧解析 HTTP 参数得到的值一致（api 负责合法性校验，
// 本包只按值分派）；一致性由本包单测钉住字面量。
const (
	TabForYou = "for_you"
	TabResume = "resume"
	TabRandom = "random"
)

// 兴趣维度前缀。与 recstore 的 dim 常量、recommend 的维度常量同口径：
// 三包互不 import，一致性由各自的单测钉住同一组字面量。
const (
	dimCategoryL1 = "category_l1"
	dimCategoryL2 = "category_l2"
	dimAuthor     = "author"
	dimTag        = "tag"
	dimSeries     = "series"
)

// 各路召回的配额（设计 §3）。
const (
	recallPerCategory   = 40 // 每个兴趣分类取 40 本（设计 §3）
	recallTopCategories = 3  // 只取用户 top3 分类：兴趣分布长尾，多取只是重复同一批书
	recallTopAuthors    = 2
	recallPerAuthor     = 20
	recallCooccurLimit  = 60
	recallHotDays       = 7
	recallHotLimit      = 40
	recallFreshLimit    = 20
	recallRandomLimit   = 20
	recallTopTags       = 2
	recallPerTag        = 20
	recallTopSeries     = 2
	recallPerSeries     = 20
	recallResumeLimit   = 20
)
