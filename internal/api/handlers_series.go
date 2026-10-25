package api

import (
	"net/http"
	"net/url"
	"strings"

	"norvels/internal/store"
)

// 系列聚合（P3 §系列聚合页）：series 是 books 上的自由文本列，没有独立实体，
// 因此这两个端点只是把「按 TRIM(series) 分组」的结果包成契约里的 JSON 形状。

// seriesListResponse 对应 docs/api.md 的 GET /api/series。
type seriesListResponse struct {
	Items []store.SeriesSummary `json:"items"`
	Total int                   `json:"total"`
}

// handleListSeries 处理 GET /api/series?q=&page=&size=：分页列出系列聚合。
func (s *Server) handleListSeries(w http.ResponseWriter, r *http.Request) {
	page, size, err := parsePagination(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	items, total, err := s.st.ListSeries(r.Context(),
		strings.TrimSpace(r.URL.Query().Get("q")), page, size)
	if err != nil {
		s.fail(w, r, err, "系列列表不可用")
		return
	}
	if items == nil {
		items = []store.SeriesSummary{}
	}
	writeJSON(w, http.StatusOK, seriesListResponse{Items: items, Total: total})
}

// seriesDetailResponse 对应 docs/api.md 的 GET /api/series/{name}。
type seriesDetailResponse struct {
	Name  string            `json:"name"`
	Books []store.BookBrief `json:"books"`
}

// handleGetSeries 处理 GET /api/series/{name}：系列名按 URL 编码传入，无书 404。
func (s *Server) handleGetSeries(w http.ResponseWriter, r *http.Request) {
	name, err := seriesPathName(r.PathValue("name"))
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	books, err := s.st.SeriesBooks(r.Context(), name)
	if err != nil {
		s.fail(w, r, err, "series not found")
		return
	}
	if books == nil {
		books = []store.BookBrief{}
	}
	writeJSON(w, http.StatusOK, seriesDetailResponse{Name: name, Books: books})
}

// seriesPathName 解码并校验路径里的系列名。
//
// mux 已经解过一次百分号编码；这里再解是为了容忍前端二次编码（%25E5…），
// 解不开的转义是请求错误（400）。名字只进参数化 SQL，不碰文件系统，
// 但仍挡掉路径分隔符与控制字符：/api/series/a%2Fb 不该被当成多级路径。
func seriesPathName(raw string) (string, error) {
	name, err := url.PathUnescape(raw)
	if err != nil {
		return "", badParam("系列名不是合法的 URL 编码")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", badParam("系列名不能为空")
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return "", badParam("系列名不能包含路径分隔符或控制字符")
	}
	return name, nil
}
