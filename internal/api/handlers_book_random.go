package api

import (
	"math/rand/v2"
	"net/http"
	"strings"

	"norvels/internal/store"
)

// 随机一本（P3 §筛选与发现）：可选按作者 / 标签 / 书单缩范围，无命中 404。
//
// 不新增 store 方法：先数总数、再随机取一行（两次 ListBooks，offset 落在一行上），
// 避免把整库书目读进内存——库大了那是几十 MB 的列表。

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

// randomBookBrief 先数总数再随机取一行；无书返回 (nil, nil)。
func (s *Server) randomBookBrief(r *http.Request, q store.BookQuery) (*store.BookBrief, error) {
	first, err := s.st.ListBooks(r.Context(), q)
	if err != nil {
		return nil, err
	}
	if first.Total <= 0 {
		return nil, nil
	}
	q.Page, q.Size = rand.IntN(first.Total)+1, 1
	page, err := s.st.ListBooks(r.Context(), q)
	if err != nil {
		return nil, err
	}
	if len(page.Items) == 0 {
		return nil, nil // 两次查询之间书被删掉了：按「没有」处理，由前端再点一次
	}
	return briefFromSummary(page.Items[0]), nil
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
