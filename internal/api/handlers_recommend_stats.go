package api

import (
	"context"
	"net/http"
	"sort"
	"strconv"

	"norvels/internal/recommend"
	"norvels/internal/recstore"
	"norvels/internal/store"
)

// 推荐看板（驾驶舱「推荐与埋点」区块）：GET /api/recommend/stats?days=14。
//
// 口径（两处注释写清，别让后来者以为 daily 也按账号）：
//   - summary / daily 读 book_daily，**全局埋点口径**：热度是所有人共同贡献的，
//     按账号拆反而失去意义，所以不带 ScopeUserID；
//   - interests 读 user_interest，**当前账号口径**：兴趣只对本人有意义。
//
// 常量与召回参数刻意分开（recallHotDays 等）：召回要的是「够不够凑一屏」，
// 看板要的是「人一眼看得懂」——两者以后会各自调，共用常量必然互相绑架。

const (
	defaultRecommendStatsDays = 14
	minRecommendStatsDays     = 1
	maxRecommendStatsDays     = 90
	// statsHotDays / statsHotLimit 是看板的「近 7 天热门」口径（与召回无关）。
	statsHotDays       = 7
	statsHotLimit      = 10
	statsInterestLimit = 10
)

// 兴趣维度的取值与 recstore 的 dim 一致（两包不互相 import，各自钉住字符串，
// 由各自的单测断言同一个字面量）。分类分两级：看板上「武侠」与「江湖」
// 是两行独立数据，不再成对重复。
const (
	interestDimCategoryL1 = "category_l1"
	interestDimCategoryL2 = "category_l2"
	interestDimAuthor     = "author"
	interestDimTag        = "tag"
	interestDimSeries     = "series"
)

// interestStat 是兴趣榜的一行（带给人看的 name）。
//
// name 与 key 的分工：key 是稳定的机器标识（作者 id / 分类名），name 是展示名。
// 作者维度只给 id 的话，看板上会出现「作者 1 / 3 / 2」这种没人看得懂的条目。
type interestStat struct {
	Dim    string  `json:"dim"`
	Key    string  `json:"key"`
	Name   string  `json:"name,omitempty"`
	Weight float64 `json:"weight"`
}

// hotBookStat 是热门榜的一行（热度 + 主库补出的展示字段）。
type hotBookStat struct {
	BookID     int64  `json:"bookId"`
	Title      string `json:"title"`
	AuthorName string `json:"authorName"`
	Reads      int    `json:"reads"`
	Clicks     int    `json:"clicks"`
	Seconds    int    `json:"seconds"`
}

// recommendStatsResponse 是 GET /api/recommend/stats 的响应体。
type recommendStatsResponse struct {
	Summary   recstore.Summary     `json:"summary"`
	Daily     []recstore.DailyStat `json:"daily"`
	Interests []interestStat       `json:"interests"`
	HotBooks  []hotBookStat        `json:"hotBooks"`
	// ReasonMix / CategoryClicks 是「推荐理由被点了几次」「分类被点了几次」，
	// 都从 click 明细聚合（impression 不落明细，拿不到理由分布）。
	ReasonMix      []reasonMixStat          `json:"reasonMix"`
	CategoryClicks []recstore.CategoryClick `json:"categoryClicks"`
}

// handleRecommendStats 处理 GET /api/recommend/stats：看板聚合数据（只读）。
func (s *Server) handleRecommendStats(w http.ResponseWriter, r *http.Request) {
	if !s.requireRec(w) {
		return
	}
	days, err := clampedIntQuery(r, "days", defaultRecommendStatsDays,
		minRecommendStatsDays, maxRecommendStatsDays)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	ctx := r.Context()
	summary, err := s.cfg.Rec.Summary(ctx, days)
	if err != nil {
		s.failRec(w, r, err, "推荐汇总不可用")
		return
	}
	daily, err := s.cfg.Rec.DailyStats(ctx, days)
	if err != nil {
		s.failRec(w, r, err, "推荐按日统计不可用")
		return
	}
	interestRows, err := s.cfg.Rec.InterestTop(ctx, store.ScopeUserID(ctx), statsInterestLimit)
	if err != nil {
		s.failRec(w, r, err, "推荐兴趣榜不可用")
		return
	}
	interests, err := s.interestItems(ctx, interestRows)
	if err != nil {
		s.fail(w, r, err, "推荐兴趣榜不可用")
		return
	}
	hot, err := s.buildHotBooks(ctx)
	if err != nil {
		s.fail(w, r, err, "推荐热门榜不可用")
		return
	}
	reasonMix, err := s.cfg.Rec.ReasonMix(ctx, days)
	if err != nil {
		s.failRec(w, r, err, "推荐理由分布不可用")
		return
	}
	categoryClicks, err := s.cfg.Rec.CategoryClicks(ctx, days)
	if err != nil {
		s.failRec(w, r, err, "推荐分类点击不可用")
		return
	}
	writeJSON(w, http.StatusOK, recommendStatsResponse{
		Summary: summary, Daily: daily, Interests: interests, HotBooks: hot,
		ReasonMix: normalizeReasonMix(reasonMix), CategoryClicks: categoryClicks,
	})
}

// interestItems 给兴趣行补展示名：作者维度换成作者名，分类维度名就是分类名。
//
// 作者查不到名字时留空（json omitempty）：前端统一按 name → key 回退，
// 这样「作者已被删除」不会显示成空字符串，也不会退化成看不懂的数字 id。
func (s *Server) interestItems(ctx context.Context, rows []recstore.InterestRow) ([]interestStat, error) {
	names, err := s.authorNamesFor(ctx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]interestStat, 0, len(rows))
	for _, row := range rows {
		item := interestStat{Dim: row.Dim, Key: row.Key, Weight: row.Weight}
		// 除作者外，key 本身就是展示名（分类名 / 标签名 / 系列名都给人看）。
		if row.Dim != interestDimAuthor {
			item.Name = row.Key
			out = append(out, item)
			continue
		}
		if id, perr := strconv.ParseInt(row.Key, 10, 64); perr == nil {
			item.Name = names[id]
		}
		out = append(out, item)
	}
	return out, nil
}

// authorNamesFor 取出兴趣行里作者维度的 id 并批量查名字（分类行不查库）。
func (s *Server) authorNamesFor(ctx context.Context, rows []recstore.InterestRow) (map[int64]string, error) {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Dim != interestDimAuthor {
			continue
		}
		if id, err := strconv.ParseInt(row.Key, 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil // 全是分类行：不必为主库多查一次
	}
	return s.st.AuthorNamesByIDs(ctx, ids)
}

// buildHotBooks 取近 7 天热度前 10，并回主库补标题/作者；缺书直接跳过。
//
// 跳过而不是占位：埋点里可能有已被删除的书（事件不会级联删），
// 看板上一行「未知书名」除了占位没有任何信息量，还不如让榜单短一行。
func (s *Server) buildHotBooks(ctx context.Context) ([]hotBookStat, error) {
	hot, err := s.cfg.Rec.HotBooks(ctx, statsHotDays, statsHotLimit)
	if err != nil {
		return nil, err
	}
	if len(hot) == 0 {
		return []hotBookStat{}, nil
	}
	ids := make([]int64, 0, len(hot))
	for _, h := range hot {
		ids = append(ids, h.BookID)
	}
	books, err := s.st.BooksByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]store.BookSummary, len(books))
	for _, bs := range books {
		byID[bs.ID] = bs
	}
	rows := make([]hotBookStat, 0, len(hot))
	for _, h := range hot {
		bs, ok := byID[h.BookID]
		if !ok {
			continue
		}
		rows = append(rows, hotBookStat{
			BookID: h.BookID, Title: bs.Title, AuthorName: bs.AuthorName,
			Reads: h.Reads, Clicks: h.Clicks, Seconds: h.Seconds,
		})
	}
	return rows, nil
}

// reasonMixStat 是理由分布的一行：按稳定 code 分组，文案单独给。
type reasonMixStat struct {
	// Reason 是分组键（稳定 code）；历史行里的中文文案会被归一化成 code。
	Reason string `json:"reason"`
	// Label 是当前版本的展示文案（改措辞不会拆行，只会改显示）。
	Label  string `json:"label"`
	Clicks int    `json:"clicks"`
}

// normalizeReasonMix 把「历史文案」与「新 code」归一到同一组。
//
// 改这一版之前 extra 里存的是文案本身：不归一化的话，改一个字就会把同一类理由
// 拆成两行，旧行从此冻结。认不出的值按原样当一组（宁可多一行，也不要吞数据）。
func normalizeReasonMix(raw []recstore.ReasonClick) []reasonMixStat {
	out := make([]reasonMixStat, 0, len(raw))
	index := make(map[string]int, len(raw))
	for _, r := range raw {
		code := recommend.CodeFromText(r.Reason)
		if code == "" {
			code = r.Reason
		}
		if i, ok := index[code]; ok {
			out[i].Clicks += r.Clicks
			continue
		}
		index[code] = len(out)
		out = append(out, reasonMixStat{Reason: code, Label: recommend.TextOf(code), Clicks: r.Clicks})
	}
	sortReasonMix(out)
	return trimReasonMix(out)
}

// sortReasonMix 按点击数降序、同分按 code 升序（与 SQL 的排序口径一致，合并后要重排）。
func sortReasonMix(items []reasonMixStat) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Clicks != items[j].Clicks {
			return items[i].Clicks > items[j].Clicks
		}
		return items[i].Reason < items[j].Reason
	})
}

// trimReasonMix 合并后可能超过 topMixLimit（多条同义文案合成一行时不会超，
// 但归一化本身不保证不减），统一裁到一屏看得完的条数。
func trimReasonMix(items []reasonMixStat) []reasonMixStat {
	if len(items) > 10 {
		return items[:10]
	}
	return items
}
