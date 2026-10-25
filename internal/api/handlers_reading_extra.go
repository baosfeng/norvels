package api

import (
	"errors"
	"net/http"
	"strings"

	"norvels/internal/store"
)

// 阅读会话 / 历史 / 统计与追更角标确认（P3 §阅读时长统计、§阅读历史）。
//
// 会话由前端按「页面可见时累计、≥60 s 且切换章节/隐藏/卸载才上报」的节奏写，
// 一条上报就是一行，不做 upsert 合并（明细是统计原材料）。

// 会话时长与统计窗口的边界（与 p3 契约一致）。
const (
	minSessionSeconds = 1
	maxSessionSeconds = 86400
	defaultStatsDays  = 90
	minStatsDays      = 1
	maxStatsDays      = 365
	// 时区偏移（分钟）：UTC-12:00 ~ UTC+14:00，东八区为 480。
	minTZMinutes = -720
	maxTZMinutes = 840
)

// handleAckNewChapters 处理 POST /api/books/{id}/ack-new-chapters：角标清零，204。
func (s *Server) handleAckNewChapters(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	if err := s.st.AckNewChapters(r.Context(), id); err != nil {
		s.fail(w, r, err, "book not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readingSessionRequest 是 POST /api/reading/sessions 的请求体。
type readingSessionRequest struct {
	BookID    int64  `json:"bookId"`
	ChapterID int64  `json:"chapterId"`
	Seconds   int    `json:"seconds"`
	StartedAt string `json:"startedAt"`
}

// handleAddReadingSession 处理 POST /api/reading/sessions：记一次会话，204。
func (s *Server) handleAddReadingSession(w http.ResponseWriter, r *http.Request) {
	var body readingSessionRequest
	if !decodeJSONBody(w, r, &body, "请求体必须是合法 JSON 对象") {
		return
	}
	if err := validateSessionRequest(body); err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	in := store.ReadingSessionInput{
		BookID:    body.BookID,
		ChapterID: body.ChapterID,
		Seconds:   body.Seconds,
		StartedAt: strings.TrimSpace(body.StartedAt),
	}
	if err := s.st.AddReadingSession(r.Context(), in); err != nil {
		s.failReadingWrite(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateSessionRequest 做字段级校验（store 侧还有同一套守卫，这里先拦成 400）。
func validateSessionRequest(b readingSessionRequest) error {
	if b.BookID <= 0 {
		return badParam("bookId 必须是正整数")
	}
	if b.ChapterID <= 0 {
		return badParam("chapterId 必须是正整数")
	}
	if b.Seconds < minSessionSeconds || b.Seconds > maxSessionSeconds {
		return badParam("seconds 必须在 %d~%d 之间", minSessionSeconds, maxSessionSeconds)
	}
	return nil
}

// readingHistoryResponse 对应 docs/api.md 的 GET /api/reading/history。
type readingHistoryResponse struct {
	Items []store.HistoryRow `json:"items"`
	Total int                `json:"total"`
}

// handleReadingHistory 处理 GET /api/reading/history?page=&size=：按书聚合分页。
func (s *Server) handleReadingHistory(w http.ResponseWriter, r *http.Request) {
	page, size, err := parsePagination(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	items, total, err := s.st.ReadingHistory(r.Context(), page, size)
	if err != nil {
		s.fail(w, r, err, "阅读历史不可用")
		return
	}
	if items == nil {
		items = []store.HistoryRow{}
	}
	writeJSON(w, http.StatusOK, readingHistoryResponse{Items: items, Total: total})
}

// handleReadingStats 处理 GET /api/stats/reading?days=&tz=：统计与热力图。
//
// days 默认 90（1–365）；tz 是分钟偏移（东八区 480，默认 0）。
func (s *Server) handleReadingStats(w http.ResponseWriter, r *http.Request) {
	days, err := intQuery(r, "days", defaultStatsDays, minStatsDays, maxStatsDays)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	tz, err := intQuery(r, "tz", 0, minTZMinutes, maxTZMinutes)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	// tz 是调用方的分钟偏移（东八区 480，默认 0）：store 按它聚合「今日 / 连续天数 / 热力图」，
	// 否则容器跑在 UTC 时，晚上 8 点后的阅读会被算到「昨天」。
	stats, err := s.st.ReadingStats(r.Context(), days, tz)
	if err != nil {
		s.fail(w, r, err, "阅读统计不可用")
		return
	}
	if stats.Days == nil {
		stats.Days = []store.DaySeconds{}
	}
	writeJSON(w, http.StatusOK, stats)
}

// failReadingWrite 把阅读写入失败翻译成 404（书/章不存在）/400（入参）/500。
func (s *Server) failReadingWrite(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.fail(w, r, err, "书籍或章节不存在")
	case errors.Is(err, store.ErrInvalidArgument):
		s.failParam(w, err)
	default:
		s.fail(w, r, err, "阅读数据写入失败")
	}
}
