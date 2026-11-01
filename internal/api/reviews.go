package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"norvels/internal/store"
)

// 本文件是 AI 审核（设计 §6 / §7）的 HTTP 出口：列表 + 两个决策端点。
//
// 契约（docs/api/api-reviews.md 是事实源）：
//
//	GET  /api/import/reviews?status=pending|ready|all&limit=50
//	POST /api/import/reviews/{id}/decide   {"action":"replace"|"keep"}
//	POST /api/import/reviews/{id}/rollback
//
// 三个端点都返回「决策后的记录」（列表里的同一形状），前端不必再拉一次列表；
// 状态机不允许的迁移一律 409（store.ErrReviewState），不存在的记录 404。

const reviewBodyLimit = 4 << 10 // 4 KiB：决策请求体只有一个 action 字段

// reviewBookView 是审核列表里的一本书。
type reviewBookView struct {
	ID           int64                 `json:"id"`
	Title        string                `json:"title"`
	ChapterCount int                   `json:"chapterCount"`
	WordCount    int                   `json:"wordCount"`
	SupersededBy int64                 `json:"supersededBy"`
	Quality      store.BookQualityView `json:"quality"`
}

// reviewQualityView 是规则侧的质量对比（与导入判定同一份口径，见 store.CompareBookQuality）。
type reviewQualityView struct {
	Old    store.BookQualityView    `json:"old"`
	New    store.BookQualityView    `json:"new"`
	Better []store.QualityDimension `json:"better"`
	Worse  []store.QualityDimension `json:"worse"`
	Note   string                   `json:"note,omitempty"`
}

// reviewAIView 是 AI 侧的结论。needs_manual 时 winner 为空、reasons 写「为什么没有结论」。
type reviewAIView struct {
	Winner     string   `json:"winner,omitempty"`
	Confidence float64  `json:"confidence"`
	Reasons    []string `json:"reasons"`
}

// reviewItem 是列表里的一条审核记录。
type reviewItem struct {
	ID          int64             `json:"id"`
	JobID       int64             `json:"jobId"`
	Status      string            `json:"status"`
	HitRatio    float64           `json:"hitRatio"`
	OldBook     reviewBookView    `json:"oldBook"`
	NewBook     reviewBookView    `json:"newBook"`
	Quality     reviewQualityView `json:"quality"`
	AI          reviewAIView      `json:"ai"`
	RuleReasons []string          `json:"ruleReasons"`
	CreatedAt   time.Time         `json:"createdAt"`
	DecidedAt   *time.Time        `json:"decidedAt,omitempty"`
	DecidedBy   string            `json:"decidedBy"`
}

// reviewListResponse 是 GET /api/import/reviews 的响应体。
//
// aiEnabled 是 AI 开关的当前值：false 时前端提示「仅规则结论，需人工判断」，
// 不必自己再查设置（开关在库里，前端拿不到）。
type reviewListResponse struct {
	Items     []reviewItem `json:"items"`
	AIEnabled bool         `json:"aiEnabled"`
}

// reviewDecideRequest 是 POST /api/import/reviews/{id}/decide 的请求体。
type reviewDecideRequest struct {
	Action string `json:"action"`
}

// handleListImportReviews 处理 GET /api/import/reviews。
//
// 默认只返回 pending（前端待办列表的默认视图）；status=all 才返回全部历史。
// 未知 status 值按「不筛」处理（只读展示路径，不因一个枚举值报 400）。
func (s *Server) handleListImportReviews(w http.ResponseWriter, r *http.Request) {
	limit, err := intQuery(r, "limit", defaultReviewListLimit, 1, maxReviewListLimit)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = store.ReviewStatusPending
	}
	revs, err := s.st.ListImportReviews(r.Context(), status, limit)
	if err != nil {
		s.fail(w, r, err, "审核列表不可用")
		return
	}
	items, err := s.buildReviewItems(r, revs)
	if err != nil {
		s.fail(w, r, err, "审核列表不可用")
		return
	}
	writeJSON(w, http.StatusOK, reviewListResponse{Items: items, AIEnabled: s.st.AIEnabled(r.Context())})
}

// handleDecideImportReview 处理 POST /api/import/reviews/{id}/decide。
func (s *Server) handleDecideImportReview(w http.ResponseWriter, r *http.Request) {
	id, ok := s.reviewPathID(w, r)
	if !ok {
		return
	}
	req, ok := decodeDecideRequest(w, r)
	if !ok {
		return
	}
	rev, err := s.st.DecideImportReview(r.Context(), id, req.Action, decidedBy(r))
	if err != nil {
		s.failReview(w, r, err)
		return
	}
	s.writeReviewItem(w, r, rev)
}

// handleRollbackImportReview 处理 POST /api/import/reviews/{id}/rollback。
func (s *Server) handleRollbackImportReview(w http.ResponseWriter, r *http.Request) {
	id, ok := s.reviewPathID(w, r)
	if !ok {
		return
	}
	rev, err := s.st.RollbackImportReview(r.Context(), id, decidedBy(r))
	if err != nil {
		s.failReview(w, r, err)
		return
	}
	s.writeReviewItem(w, r, rev)
}

// ---------- 辅助 ----------

// 审核列表的条数上限（与 store 侧口径一致：默认 50，上限 200）。
const (
	defaultReviewListLimit = 50
	maxReviewListLimit     = 200
)

// reviewPathID 解析并校验路径里的审核记录 id。
func (s *Server) reviewPathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return 0, false
	}
	return id, true
}

// decodeDecideRequest 解析决策请求体：只允许一个 JSON 对象，且 action 必须是 replace / keep。
func decodeDecideRequest(w http.ResponseWriter, r *http.Request) (reviewDecideRequest, bool) {
	var req reviewDecideRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, reviewBodyLimit))
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, codeInvalidParam, "请求体不能为空（action 必填）")
			return req, false
		}
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求体必须是合法 JSON 对象")
		return req, false
	}
	if req.Action != store.ReviewActionReplace && req.Action != store.ReviewActionKeep {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "action 只支持 replace / keep")
		return req, false
	}
	return req, true
}

// decidedBy 返回本次决策的账号标识（未启用鉴权时为空串）。
func decidedBy(r *http.Request) string {
	id := store.ScopeUserID(r.Context())
	if id <= 0 {
		return ""
	}
	return "user:" + strconv.FormatInt(id, 10)
}

// failReview 把审核相关错误映射成 HTTP 响应：404 / 400 / 409（状态机） / 500。
func (s *Server) failReview(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrReviewState):
		s.logf("api: %s %s 审核状态机拒绝: %v", r.Method, r.URL.Path, err)
		writeError(w, http.StatusConflict, codeReviewState, reviewStateMessage(err))
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, codeNotFound, "审核记录不存在")
	default:
		s.fail(w, r, err, "审核记录不存在")
	}
}

// reviewStateMessage 取状态机错误里的人话部分（store 的错误文本带包前缀）。
func reviewStateMessage(err error) string {
	msg := err.Error()
	if i := lastColon(msg); i >= 0 && i+1 < len(msg) {
		return msg[i+1:]
	}
	return "该记录当前状态不允许这次操作"
}

// lastColon 返回最后一个「: 」的位置（找不到返回 -1）。
func lastColon(s string) int {
	for i := len(s) - 2; i >= 0; i-- {
		if s[i] == ':' && s[i+1] == ' ' {
			return i
		}
	}
	return -1
}

// buildReviewItems 把记录批量翻成展示形状（书信息一次批量取，避免 N+1）。
func (s *Server) buildReviewItems(r *http.Request, revs []store.ImportReview) ([]reviewItem, error) {
	items := make([]reviewItem, 0, len(revs))
	ids := make([]int64, 0, len(revs)*2)
	for _, rev := range revs {
		ids = append(ids, rev.OldBookID, rev.NewBookID)
	}
	books, err := s.st.BooksForReview(r.Context(), ids)
	if err != nil {
		return nil, err
	}
	for _, rev := range revs {
		items = append(items, s.buildReviewItem(rev, books))
	}
	return items, nil
}

// buildReviewItem 翻一条记录（书缺失时按「已删除」的空摘要展示，不报错）。
func (s *Server) buildReviewItem(rev store.ImportReview, books map[int64]store.ReviewBook) reviewItem {
	oldBook := bookView(books[rev.OldBookID], rev.OldBookID)
	newBook := bookView(books[rev.NewBookID], rev.NewBookID)
	pick := store.CompareBookQuality(oldBook.Quality.BookQuality, newBook.Quality.BookQuality)
	return reviewItem{
		ID:       rev.ID,
		JobID:    rev.JobID,
		Status:   rev.Status,
		HitRatio: rev.HitRatio,
		OldBook:  oldBook,
		NewBook:  newBook,
		Quality: reviewQualityView{
			Old: oldBook.Quality, New: newBook.Quality,
			Better: nonNilDims(pick.Better), Worse: nonNilDims(pick.Worse), Note: pick.Note,
		},
		AI: reviewAIView{
			Winner: rev.AIWinner, Confidence: rev.AIConfidence,
			Reasons: nonNilStrings(rev.AIReasons),
		},
		RuleReasons: nonNilStrings(rev.RuleReasons),
		CreatedAt:   rev.CreatedAt,
		DecidedAt:   rev.DecidedAt,
		DecidedBy:   rev.DecidedBy,
	}
}

// bookView 把 store 的书翻成展示形状；书已被删时只保留 id。
//
// id 用入参而不是 b.ID：查不到书时 b 是零值，只有调用方知道这条记录指的是哪本书。
func bookView(b store.ReviewBook, id int64) reviewBookView {
	return reviewBookView{
		ID: id, Title: b.Title, ChapterCount: b.ChapterCount, WordCount: b.WordCount,
		SupersededBy: b.SupersededBy, Quality: b.Quality.View(),
	}
}

// writeReviewItem 返回「决策后的那条记录」。
func (s *Server) writeReviewItem(w http.ResponseWriter, r *http.Request, rev store.ImportReview) {
	books, err := s.st.BooksForReview(r.Context(), []int64{rev.OldBookID, rev.NewBookID})
	if err != nil {
		s.fail(w, r, err, "审核记录不可用")
		return
	}
	writeJSON(w, http.StatusOK, s.buildReviewItem(rev, books))
}

// nonNilStrings 保证 JSON 里是 [] 而不是 null（前端不必判 null）。
func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// nonNilDims 保证 JSON 里是 [] 而不是 null。
func nonNilDims(v []store.QualityDimension) []store.QualityDimension {
	if v == nil {
		return []store.QualityDimension{}
	}
	return v
}
