package api

import (
	"net/http"
	"strings"

	"norvels/internal/store"
)

// 随机一本（P3 §筛选与发现）：可选按作者 / 标签 / 书单缩范围，无命中 404。
//
// 采样在 store.RandomBook 里：无筛选走 id 采样（一次索引查找，不再按 offset 扫半个表），
// 带筛选才退回「数总数 + 随机偏移」。handler 只管参数解析与响应形状。

func (s *Server) handleRandomBook(w http.ResponseWriter, r *http.Request) {
	ids, err := parseBookIDFilters(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	q := store.BookQuery{
		AuthorID:   ids.authorID,
		Tag:        strings.TrimSpace(r.URL.Query().Get("tag")),
		Collection: ids.collectionID,
		Page:       1,
		Size:       1,
	}
	brief, err := s.randomBookBrief(r, q)
	if err != nil {
		s.fail(w, r, err, "随机选书不可用")
		return
	}
	if brief == nil {
		writeError(w, http.StatusNotFound, codeNotFound, "没有符合条件的书")
		return
	}
	writeJSON(w, http.StatusOK, brief)
}

// randomBookBrief 取一本随机的书；无书返回 (nil, nil)。
func (s *Server) randomBookBrief(r *http.Request, q store.BookQuery) (*store.BookBrief, error) {
	bs, err := s.st.RandomBook(r.Context(), q)
	if err != nil || bs == nil {
		return nil, err
	}
	return briefFromSummary(*bs), nil
}

// briefFromSummary 把列表行裁成 BookBrief（与系列详情的形状一致）。
func briefFromSummary(b store.BookSummary) *store.BookBrief {
	return &store.BookBrief{
		ID: b.ID, Title: b.Title, AuthorID: b.AuthorID, AuthorName: b.AuthorName,
		ChapterCount: b.ChapterCount, WordCount: b.WordCount, HasCover: b.HasCover,
		Rating: b.Rating, NewChapters: b.NewChapters, ReadStatus: b.ReadStatus,
		Series: b.Series, SeriesIndex: b.SeriesIndex, UpdatedAt: b.UpdatedAt,
	}
}
