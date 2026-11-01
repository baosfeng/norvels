package recall

import (
	"time"

	"norvels/internal/recommend"
	"norvels/internal/recstore"
	"norvels/internal/store"
)

// Result 是各路召回的并集：书目（回主库补齐的展示字段）+ 打分信号。
//
// 用 map 去重 + order 保序：同一本书常被多路同时召回（兴趣 + 热度），
// 只留第一次出现的顺序能让「随机探索」与打分的输入稳定可复现。
type Result struct {
	// Books 与 Resume 是装配阶段要读的两份结果：前者拼推荐卡，后者给续读 tab 排序。
	Books  map[int64]store.BookSummary
	Resume map[int64]float64

	order   []int64
	cooccur map[int64]int
	hot     map[int64]recstore.HotBook
	// tags 是候选书的标签（打分与理由都要用，召回结束后一次性补齐）。
	tags map[int64][]string
	// similar 记录相似书召回里「与种子书的关系文案」；selfID 是种子书（不推荐自己）。
	similar map[int64]string
	// similarCodes 是同一批关系文案的稳定 code（看板按它分组，文案改了不影响统计）。
	similarCodes map[int64]string
	selfID       int64
}

// New 预分配容量（一屏召回通常几十到几百本）。
func New() *Result {
	return &Result{
		Books:        make(map[int64]store.BookSummary, 64),
		Resume:       make(map[int64]float64, 16),
		order:        make([]int64, 0, 64),
		cooccur:      make(map[int64]int, 32),
		hot:          make(map[int64]recstore.HotBook, 32),
		tags:         make(map[int64][]string, 64),
		similar:      make(map[int64]string, 32),
		similarCodes: make(map[int64]string, 32),
	}
}

// Add 收下一本书（同一本书只留第一次出现的顺序）。
func (r *Result) Add(bs store.BookSummary) {
	if bs.ID <= 0 {
		return
	}
	if _, ok := r.Books[bs.ID]; ok {
		return
	}
	r.Books[bs.ID] = bs
	r.order = append(r.order, bs.ID)
}

// AddSimilar 收下相似候选并记下关系（文案 + 稳定 code，先到先得）。
//
// 文案给人看、code 给看板分组：两者一起存，才可能「改措辞不改统计口径」。
func (r *Result) AddSimilar(bs store.BookSummary, reason recommend.Reason) {
	if bs.ID == r.selfID {
		return
	}
	if _, ok := r.similar[bs.ID]; !ok {
		r.similar[bs.ID] = reason.Text
		r.similarCodes[bs.ID] = reason.Code
	}
	r.Add(bs)
}

// KeepCooccur 记下某本书的共读人数（同一邻居书多路贡献时取最大值）。
func (r *Result) KeepCooccur(pair recstore.Cooccur) {
	if pair.Users > r.cooccur[pair.OtherID] {
		r.cooccur[pair.OtherID] = pair.Users
	}
}

// Signals 是打分需要的用户侧信号（与召回结果一起进 Candidates）。
type Signals struct {
	// Read：有过 read 事件（只降权）。Finished：主库进度 ≥0.99（唯一会被过滤的读状态）。
	Read     map[int64]bool
	Finished map[int64]bool
	Recent   map[int64]bool
	Now      time.Time
}

// Candidates 把召回结果转成打分输入（顺序与召回顺序一致）。
func (r *Result) Candidates(sig Signals) []recommend.Candidate {
	out := make([]recommend.Candidate, 0, len(r.order))
	for _, id := range r.order {
		bs := r.Books[id]
		out = append(out, recommend.Candidate{
			BookID: bs.ID, AuthorID: bs.AuthorID,
			CategoryL1: bs.CategoryL1, CategoryL2: bs.CategoryL2,
			ImportedAt: bs.ImportedAt, Rating: bs.Rating,
			Read: sig.Read[id], Finished: sig.Finished[id], RecentShown: sig.Recent[id],
			CooccurUsers: r.cooccur[id],
			HotReads:     r.hot[id].Reads, HotClicks: r.hot[id].Clicks,
			Fresh:   recommend.Freshness(bs.ImportedAt, sig.Now),
			Tags:    r.tags[id],
			Series:  bs.Series,
			Resume:  r.Resume[id] > 0,
			Similar: r.similar[id] != "", SimilarReason: r.similar[id],
			SimilarReasonCode: r.similarCodes[id],
		})
	}
	return out
}
