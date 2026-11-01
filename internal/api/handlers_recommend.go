package api

import (
	"context"
	"math/rand"
	"net/http"
	"sort"
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

	// 推荐 tab（设计 §4）：默认 for_you；resume 只续读；random 主打随机发现。
	recommendTabForYou = "for_you"
	recommendTabResume = "resume"
	recommendTabRandom = "random"

	// randomTabExploreRatio 是随机 tab 的 ε 探索比例。
	//
	// 默认 10% 对「为你推荐」是合理的克制；但用户点进「随机发现」就是想要
	// 不一样的东西，还只给 10% 等于挂了块招牌却不兑现。
	randomTabExploreRatio = 0.5
)

// recommendItem 是推荐卡（前端不查详情即可渲染）。
type recommendItem struct {
	BookID     int64  `json:"bookId"`
	Title      string `json:"title"`
	AuthorName string `json:"authorName"`
	// AuthorID 供前端「不喜欢这个作者」直接下发 dislike（缺作者时置灰）；
	// 只给名字不给 id 的话前端还得再查一次详情才能构造事件。
	AuthorID   int64  `json:"authorId"`
	CategoryL1 string `json:"categoryL1"`
	CategoryL2 string `json:"categoryL2"`
	HasCover   bool   `json:"hasCover"`
	Reason     string `json:"reason"`
	// ReasonCode 是理由的稳定标识（如 interest:武侠 / hot / similar.author），
	// 埋点与看板分组都用它：文案可以改，code 不能改。
	ReasonCode string  `json:"reasonCode"`
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
	similarID, err := similarQuery(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	// similar 与 tab 互斥且 similar 优先：给了种子书就按相似书召回，tab 不再参与
	// （此时连 tab 的合法性也不校验——前端从详情页跳过来可能带着旧的 tab 参数）。
	tab := recommendTabForYou
	if similarID == 0 {
		tab, err = parseRecommendTab(r)
		if err != nil {
			s.badRequest(w, err, codeInvalidParam)
			return
		}
	}
	ctx := r.Context()
	userID := store.ScopeUserID(ctx)
	interest, err := s.cfg.Rec.Interests(ctx, userID)
	if err != nil {
		s.failRec(w, r, err, "推荐信号不可用")
		return
	}
	var recall *recallResult
	if similarID > 0 {
		recall, err = s.recallSimilar(ctx, similarID)
	} else {
		recall, err = s.recallCandidates(ctx, userID, interest, tab)
	}
	if err != nil {
		s.fail(w, r, err, "推荐候选不可用")
		return
	}
	items, err := s.buildRecommendations(ctx, userID, interest, recall, limit, tab)
	if err != nil {
		s.failRec(w, r, err, "推荐排序不可用")
		return
	}
	writeJSON(w, http.StatusOK, recommendResponse{Items: items, GeneratedAt: time.Now().UTC()})
}

// parseRecommendTab 解析 tab 参数：空 = for_you，非法值 400（不静默当默认）。
//
// 非法值必须报错而不是回退默认：前端拼错 tab 时会一直拿到「为你推荐」，
// 排查时看不出任何异常，只有一个不生效的按钮。
func parseRecommendTab(r *http.Request) (string, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("tab"))
	switch raw {
	case "":
		return recommendTabForYou, nil
	case recommendTabForYou, recommendTabResume, recommendTabRandom:
		return raw, nil
	}
	return "", badParam("参数 tab 必须是 %s|%s|%s",
		recommendTabForYou, recommendTabResume, recommendTabRandom)
}

// similarQuery 解析 ?similar=<bookId>：缺省 0；给出时必须是正整数，否则 400。
func similarQuery(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("similar"))
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, badParam("参数 similar 必须是正整数 bookId")
	}
	return id, nil
}

// buildRecommendations 读「已读 / 近期曝光」信号，打分后做多样性重排并装配响应。
//
// 已读与曝光在这里才读：召回阶段已经排过一轮，但两个集合只影响打分与过滤，
// 一次读出即可，不必让每路召回各查一遍。
func (s *Server) buildRecommendations(ctx context.Context, userID int64,
	interest map[string]float64, recall *recallResult, limit int, tab string) ([]recommendItem, error) {
	now := time.Now().UTC()
	read, err := s.cfg.Rec.ReadBooks(ctx, userID)
	if err != nil {
		return nil, err
	}
	recent, err := s.cfg.Rec.RecentImpressions(ctx, userID, now.AddDate(0, 0, -recentShownDays))
	if err != nil {
		return nil, err
	}
	// 「读完」来自主库进度（recommend 库只知道「读过」）：两者口径不同，必须分开读。
	finished, err := s.st.FinishedBookIDs(ctx)
	if err != nil {
		return nil, err
	}
	// 负反馈与「读完」同层：都是「不该出现在结果里」的硬过滤，进打分之前就剔掉。
	negatives, err := s.cfg.Rec.NegativeItems(ctx, userID)
	if err != nil {
		return nil, err
	}
	cands := recall.candidates(recallSignals{
		read: read, finished: finished, recent: recent, now: now,
	})
	cands = dropNegatives(cands, negatives)
	// 作者级拉黑与书级同层：一次 dislike 带 authorId 时整位作者的作品都不再出现。
	negAuthors, err := s.cfg.Rec.NegativeAuthors(ctx, userID)
	if err != nil {
		return nil, err
	}
	cands = dropNegativeAuthors(cands, negAuthors)
	sig := recommend.Signals{Interest: interest, ExploreRatio: tabExploreRatio(tab)}
	ranked := recommend.Rank(cands, sig, rand.New(rand.NewSource(now.UnixNano())),
		rankLimitForTab(tab, limit, len(cands)))
	picked := diversifyForTab(ranked, limit, tab)
	if tab == recommendTabResume {
		picked = sortResumeFirst(picked, recall.resume)
	}
	items := make([]recommendItem, 0, len(picked))
	for _, sc := range picked {
		items = append(items, itemOf(sc, recall.books[sc.BookID]))
	}
	return items, nil
}

// dropNegativeAuthors 剔除作者被拉黑的候选（与书级拉黑同层过滤）。
func dropNegativeAuthors(cands []recommend.Candidate, authors map[int64]bool) []recommend.Candidate {
	if len(authors) == 0 {
		return cands
	}
	out := cands[:0]
	for _, c := range cands {
		if c.AuthorID > 0 && authors[c.AuthorID] {
			continue
		}
		out = append(out, c)
	}
	return out
}

// dropNegatives 剔除被标记「不感兴趣」的候选（原地过滤，省一次分配）。
func dropNegatives(cands []recommend.Candidate, negatives map[int64]bool) []recommend.Candidate {
	if len(negatives) == 0 {
		return cands
	}
	out := cands[:0]
	for _, c := range cands {
		if negatives[c.BookID] {
			continue
		}
		out = append(out, c)
	}
	return out
}

// rankLimitForTab 决定送去排序的条数。
//
// 随机 tab 不做超额排序：它的探索配额按「最终条数」算，多排 3 倍再截断会把
// 追加在尾部的「随机发现」先裁掉（实测 limit=10 时只剩 4 条随机）。
func rankLimitForTab(tab string, limit, available int) int {
	if tab == recommendTabRandom {
		return min(limit, available)
	}
	return rankPool(limit, available)
}

// tabExploreRatio 按 tab 给出探索比例（0 表示用 Rank 的默认值 10%）。
func tabExploreRatio(tab string) float64 {
	if tab == recommendTabRandom {
		return randomTabExploreRatio
	}
	return 0
}

// diversifyForTab 做多样性重排；resume tab 不设配额。
//
// 续读列表全是用户自己读到一半的书，被「同作者 ≤2」藏掉一本是纯粹的损失；
// 这个 tab 的目标是「把你没读完的还给你」，不是探索新书。
func diversifyForTab(items []recommend.Scored, limit int, tab string) []recommend.Scored {
	if tab == recommendTabResume {
		return recommend.Diversify(items, limit, limit, limit)
	}
	return recommend.Diversify(items, limit, diversifyAuthorCap, diversifyCategoryCap)
}

// sortResumeFirst 把续读候选按进度倒序排到前面，其余保持打分顺序。
func sortResumeFirst(items []recommend.Scored, resume map[int64]float64) []recommend.Scored {
	sort.SliceStable(items, func(i, j int) bool {
		pi, iResume := resume[items[i].BookID]
		pj, jResume := resume[items[j].BookID]
		if iResume != jResume {
			return iResume
		}
		if iResume && pi != pj {
			return pi > pj
		}
		return false
	})
	return items
}

// itemOf 把「打分结果 + 主库书目」拼成推荐卡。
func itemOf(sc recommend.Scored, bs store.BookSummary) recommendItem {
	return recommendItem{
		BookID: sc.BookID, Title: bs.Title, AuthorName: bs.AuthorName,
		AuthorID:   bs.AuthorID,
		CategoryL1: bs.CategoryL1, CategoryL2: bs.CategoryL2,
		HasCover: bs.HasCover, Reason: sc.Reason, ReasonCode: sc.ReasonCode, Score: sc.Score,
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
