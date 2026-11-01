package api

import (
	"context"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"norvels/internal/recommend"
	"norvels/internal/store"
)

// 推荐流（设计 §3/§4）：GET /api/recommend?limit=1–50（默认 20）。
//
// 端点只读、无副作用：召回读主库 + 推荐库，打分在 recommend 包里（纯函数），
// 这里只负责「凑候选 → 排序 → 裁剪成响应」这三件装配工作。

const (
	defaultRecommendLimit = 20
	minRecommendLimit     = 1
	maxRecommendLimit     = 50
	// diversifyAuthorCap / diversifyCategoryCap 与设计 §3 冻结值一致（同作者 ≤2、同大分类 ≤4）。
	diversifyAuthorCap   = 2
	diversifyCategoryCap = 4
	// rankPoolFactor 是「多排几倍再让多样性过滤」的系数。
	//
	// 重排会按作者/分类配额丢掉一部分候选，若只排 limit 条，丢完可能凑不满一屏；
	// 排 3 倍再裁，既能撑满也不至于把整库都打一遍分。
	rankPoolFactor = 3
)

// recommendItem 是推荐卡（前端不查详情即可渲染）。
type recommendItem struct {
	BookID     int64   `json:"bookId"`
	Title      string  `json:"title"`
	AuthorName string  `json:"authorName"`
	CategoryL1 string  `json:"categoryL1"`
	CategoryL2 string  `json:"categoryL2"`
	HasCover   bool    `json:"hasCover"`
	Reason     string  `json:"reason"`
	Score      float64 `json:"score"`
}

// recommendResponse 是 GET /api/recommend 的响应体。
type recommendResponse struct {
	Items       []recommendItem `json:"items"`
	GeneratedAt time.Time       `json:"generatedAt"`
}

// handleRecommend 处理 GET /api/recommend：多路召回 → 打分 → 多样性重排。
func (s *Server) handleRecommend(w http.ResponseWriter, r *http.Request) {
	if !s.requireRec(w) {
		return
	}
	limit, err := clampedIntQuery(r, "limit", defaultRecommendLimit, minRecommendLimit, maxRecommendLimit)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	ctx := r.Context()
	userID := store.ScopeUserID(ctx)
	interest, err := s.cfg.Rec.Interests(ctx, userID)
	if err != nil {
		s.failRec(w, r, err, "推荐信号不可用")
		return
	}
	recall, err := s.recallCandidates(ctx, userID, interest)
	if err != nil {
		s.fail(w, r, err, "推荐候选不可用")
		return
	}
	items, err := s.buildRecommendations(ctx, userID, interest, recall, limit)
	if err != nil {
		s.failRec(w, r, err, "推荐排序不可用")
		return
	}
	writeJSON(w, http.StatusOK, recommendResponse{Items: items, GeneratedAt: time.Now().UTC()})
}

// buildRecommendations 读「已读 / 近期曝光」信号，打分后做多样性重排并装配响应。
//
// 已读与曝光在这里才读：召回阶段已经排过一轮，但两个集合只影响打分与过滤，
// 一次读出即可，不必让每路召回各查一遍。
func (s *Server) buildRecommendations(ctx context.Context, userID int64,
	interest map[string]float64, recall *recallResult, limit int) ([]recommendItem, error) {
	now := time.Now().UTC()
	read, err := s.cfg.Rec.ReadBooks(ctx, userID)
	if err != nil {
		return nil, err
	}
	recent, err := s.cfg.Rec.RecentImpressions(ctx, userID, now.AddDate(0, 0, -recentShownDays))
	if err != nil {
		return nil, err
	}
	cands := recall.candidates(recallSignals{read: read, recent: recent, now: now})
	rnd := rand.New(rand.NewSource(now.UnixNano()))
	ranked := recommend.Rank(cands, recommend.Signals{Interest: interest}, rnd,
		rankPool(limit, len(cands)))
	picked := recommend.Diversify(ranked, limit, diversifyAuthorCap, diversifyCategoryCap)
	items := make([]recommendItem, 0, len(picked))
	for _, sc := range picked {
		items = append(items, itemOf(sc, recall.books[sc.BookID]))
	}
	return items, nil
}

// itemOf 把「打分结果 + 主库书目」拼成推荐卡。
func itemOf(sc recommend.Scored, bs store.BookSummary) recommendItem {
	return recommendItem{
		BookID: sc.BookID, Title: bs.Title, AuthorName: bs.AuthorName,
		CategoryL1: bs.CategoryL1, CategoryL2: bs.CategoryL2,
		HasCover: bs.HasCover, Reason: sc.Reason, Score: sc.Score,
	}
}

// rankPool 决定送去排序的候选条数（比 limit 多几倍，给多样性过滤留余量）。
func rankPool(limit, available int) int {
	pool := limit * rankPoolFactor
	if pool > available {
		return available
	}
	return pool
}

// clampedIntQuery 解析整型查询参数并夹到 [minV,maxV]（越界不报错，非法报错）。
//
// 与 intQuery 的差别是刻意的：推荐 limit 是展示参数，用户把 URL 改成 999
// 只是「想多要点」，按 400 拒绝没有收益；而 days / tz 影响统计口径，
// 越界必须报错，否则前端算出的日历和库里会对不上。
func clampedIntQuery(r *http.Request, name string, def, minV, maxV int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, badParam("参数 %s 必须是整数", name)
	}
	if n < minV {
		return minV, nil
	}
	if n > maxV {
		return maxV, nil
	}
	return n, nil
}
