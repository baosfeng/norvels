package api

import (
	"net/http"
	"strings"

	"norvels/internal/store"
)

// 驾驶舱端点（契约见 docs/api.md §8）。

// progressListResponse 对应 GET /api/reading/progress。
type progressListResponse struct {
	Items []store.ReadingProgress `json:"items"`
}

// handleListProgress 返回最近在读的 N 条（默认 5，上限 50）。
func (s *Server) handleListProgress(w http.ResponseWriter, r *http.Request) {
	limit, err := intQuery(r, "limit", defaultProgressLimit, 1, maxProgressLimit)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	items, err := s.st.ListReadingProgress(r.Context(), limit)
	if err != nil {
		s.fail(w, r, err, "阅读进度不可用")
		return
	}
	if items == nil {
		items = []store.ReadingProgress{}
	}
	writeJSON(w, http.StatusOK, progressListResponse{Items: items})
}

// progressRequest 是 PUT /api/reading/progress 的请求体。
type progressRequest struct {
	BookID    int64   `json:"bookId"`
	ChapterID int64   `json:"chapterId"`
	Percent   float64 `json:"percent"`
	// ParaIndex 是章内段落位置（0 起，缺省 0）：进入章节时据此滚回上次读到的段落。
	ParaIndex int `json:"paraIndex"`
}

// handleSaveProgress 按 bookId upsert 进度；成功返回 204。
func (s *Server) handleSaveProgress(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeProgressRequest(w, r)
	if !ok {
		return
	}
	item := store.ReadingProgress{
		BookID: req.BookID, ChapterID: req.ChapterID,
		Percent: req.Percent, ParaIndex: req.ParaIndex,
	}
	if err := s.st.SaveReadingProgress(r.Context(), item); err != nil {
		s.fail(w, r, err, "书籍或章节不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeProgressRequest 解析并做字段级校验（bookId/chapterId 必填，percent 在 0~1）。
func decodeProgressRequest(w http.ResponseWriter, r *http.Request) (progressRequest, bool) {
	var req progressRequest
	if !decodeJSONBody(w, r, &req, "请求体必须是合法 JSON 对象") {
		return req, false
	}
	if req.BookID <= 0 {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "bookId 必须是正整数")
		return req, false
	}
	if req.ChapterID <= 0 {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "chapterId 必须是正整数")
		return req, false
	}
	if _, ok := store.NormalizePercent(req.Percent); !ok {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "percent 必须在 0~1 之间")
		return req, false
	}
	if req.ParaIndex < 0 {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "paraIndex 不能为负")
		return req, false
	}
	return req, true
}

// handleHealthReport 返回驾驶舱库健康报告（单项失败不整体 500）。
func (s *Server) handleHealthReport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.st.HealthReport(r.Context()))
}

// handleImportTrend 返回近 N 天导入量（无数据日期补 0）。
func (s *Server) handleImportTrend(w http.ResponseWriter, r *http.Request) {
	days, err := intQuery(r, "days", defaultTrendDays, 1, maxTrendDays)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	points, err := s.st.ImportTrend(r.Context(), days)
	if err != nil {
		s.fail(w, r, err, "导入趋势不可用")
		return
	}
	if points == nil {
		points = []store.ImportTrendPoint{}
	}
	writeJSON(w, http.StatusOK, points)
}

// 驾驶舱端点的参数边界（与 docs/api.md §8 一致）。
const (
	defaultProgressLimit = 5
	maxProgressLimit     = 50
	defaultTrendDays     = 14
	maxTrendDays         = 366
)

// normalizeSort 归一化排序参数：空值回落到 def，非法值返回 false。
func normalizeSort(raw, def string, allowed map[string]bool) (string, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return def, true
	}
	if !allowed[v] {
		return "", false
	}
	return v, true
}
