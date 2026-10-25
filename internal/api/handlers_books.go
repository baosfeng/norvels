package api

import (
	"net/http"
	"strconv"
	"strings"

	"norvels/internal/store"
)

// bookSorts 是 GET /api/books 支持的排序字段。
var bookSorts = map[string]bool{
	"updated":  true,
	"title":    true,
	"chapters": true,
	"words":    true,
	"imported": true,
}

func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	page, size, err := parsePagination(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sort == "" {
		sort = "updated"
	}
	if !bookSorts[sort] {
		s.badRequest(w, badParam("参数 sort 只支持 updated|title|chapters|words|imported"), codeInvalidParam)
		return
	}
	authorID, _, err := int64Query(r, "authorId")
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	q := store.BookQuery{
		Q:        strings.TrimSpace(r.URL.Query().Get("q")),
		AuthorID: authorID,
		Tag:      strings.TrimSpace(r.URL.Query().Get("tag")),
		Sort:     sort,
		Page:     page,
		Size:     size,
	}
	res, err := s.st.ListBooks(r.Context(), q)
	if err != nil {
		s.fail(w, r, err, "书籍列表不可用")
		return
	}
	normalizePage(&res, page, size)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	s.writeBookDetail(w, r, id)
}

// chapterListResponse 对应 docs/api.md 的 GET /api/books/{id}/chapters（items + total）。
type chapterListResponse struct {
	Items []store.ChapterBrief `json:"items"`
	Total int                  `json:"total"`
}

func (s *Server) handleListChapters(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	from, err := intQuery(r, "from", 1, 1, maxPage)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	limit, err := intQuery(r, "limit", defaultChapterLimit, 1, maxChapterLimit)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	res, err := s.st.ListChapters(r.Context(), store.ChapterQuery{BookID: bookID, From: from, Limit: limit})
	if err != nil {
		s.fail(w, r, err, "book not found")
		return
	}
	items := res.Items
	if items == nil {
		items = []store.ChapterBrief{}
	}
	writeJSON(w, http.StatusOK, chapterListResponse{Items: items, Total: res.Total})
}

func (s *Server) handleGetChapter(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	chapterID, err := pathID(r, "cid")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	chapter, err := s.st.GetChapter(r.Context(), bookID, chapterID)
	if err != nil {
		s.fail(w, r, err, "chapter not found")
		return
	}
	if chapter == nil {
		writeError(w, http.StatusNotFound, codeNotFound, "chapter not found")
		return
	}
	writeJSON(w, http.StatusOK, chapter)
}

// bookPatch 是 PATCH /api/books/{id} 的请求体：任意字段子集，nil 表示「不改」。
//
// 未知字段按 encoding/json 默认行为忽略；但「只含未知字段」的 body 会归一化成空
// patch，最终按 400 invalid_param 拒绝——否则用户拼错字段名会得到一次静默成功。
type bookPatch struct {
	Title       *string   `json:"title"`
	Author      *string   `json:"author"`
	Intro       *string   `json:"intro"`
	Tags        *[]string `json:"tags"`
	Series      *string   `json:"series"`
	SeriesIndex *int      `json:"seriesIndex"`
}

// toStorePatch 把请求体转成库层补丁（两层的字段集合故意一一对应，转换是纯搬运）。
func (p bookPatch) toStorePatch() store.BookMetaPatch {
	return store.BookMetaPatch{
		Title:       p.Title,
		Author:      p.Author,
		Intro:       p.Intro,
		Series:      p.Series,
		SeriesIndex: p.SeriesIndex,
		Tags:        p.Tags,
	}
}

// handlePatchBook 处理 PATCH /api/books/{id}：改元数据/标签，成功返回最新 BookDetail。
func (s *Server) handlePatchBook(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	var body bookPatch
	if !decodeJSONBody(w, r, &body, "请求体必须是书籍字段组成的 JSON 对象") {
		return
	}
	patch := body.toStorePatch()
	if patch.IsEmpty() {
		writeError(w, http.StatusBadRequest, codeInvalidParam,
			"请求体至少包含一个可修改字段：title/author/intro/tags/series/seriesIndex")
		return
	}
	if err := s.st.UpdateBookMeta(r.Context(), id, patch); err != nil {
		s.fail(w, r, err, "book not found")
		return
	}
	s.writeBookDetail(w, r, id)
}

// handleGetBookCover 处理 GET /api/books/{id}/cover：命中返回原始图片，无封面 404。
//
// 封面在导入后就固定不变（只有覆盖导入才会换），给一天缓存足够：书架一屏几十张图，
// 每次都回源会让 NAS 上的单文件数据库变成瓶颈。
func (s *Server) handleGetBookCover(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	cover, err := s.st.GetCover(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, "cover not found")
		return
	}
	h := w.Header()
	h.Set("Content-Type", cover.MIME)
	h.Set("Content-Length", strconv.Itoa(len(cover.Bytes)))
	h.Set("Cache-Control", "public, max-age=86400")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cover.Bytes)
}

// writeBookDetail 读详情并写 200（volumes 归一化成 []，前端不必判 null）。
func (s *Server) writeBookDetail(w http.ResponseWriter, r *http.Request, id int64) {
	detail, err := s.st.GetBook(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, "book not found")
		return
	}
	if detail.Volumes == nil {
		detail.Volumes = []store.VolumeNode{}
	}
	writeJSON(w, http.StatusOK, detail)
}
