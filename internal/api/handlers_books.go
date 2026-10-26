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
	// rating：评分倒序（0 = 未评自然排在最后），同分按最近更新。
	"rating": true,
}

func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	q, err := bookListQuery(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	res, err := s.st.ListBooks(r.Context(), q)
	if err != nil {
		s.fail(w, r, err, "书籍列表不可用")
		return
	}
	normalizePage(&res, q.Page, q.Size)
	writeJSON(w, http.StatusOK, res)
}

// bookListQuery 解析并校验 GET /api/books 的查询参数。
//
// 抽成函数的原因：每加一个筛选维度就要多一段「取值 → 校验 → 落库口径」，
// 留在 handler 里会把圈复杂度顶过 10（门禁 4）。
func bookListQuery(r *http.Request) (store.BookQuery, error) {
	page, size, err := parsePagination(r)
	if err != nil {
		return store.BookQuery{}, err
	}
	sort, ok := normalizeSort(r.URL.Query().Get("sort"), "updated", bookSorts)
	if !ok {
		return store.BookQuery{}, badParam("参数 sort 只支持 updated|title|chapters|words|imported|rating")
	}
	ids, err := parseBookIDFilters(r)
	if err != nil {
		return store.BookQuery{}, err
	}
	rating, err := intQuery(r, "rating", 0, 1, 5)
	if err != nil {
		return store.BookQuery{}, err
	}
	status, err := parseBookStatusFilters(r)
	if err != nil {
		return store.BookQuery{}, err
	}
	return store.BookQuery{
		Q:            strings.TrimSpace(r.URL.Query().Get("q")),
		AuthorID:     ids.authorID,
		Tag:          strings.TrimSpace(r.URL.Query().Get("tag")),
		Series:       strings.TrimSpace(r.URL.Query().Get("series")),
		Rating:       rating,
		Collection:   ids.collectionID,
		Sort:         sort,
		Page:         page,
		Size:         size,
		ReadStatus:   status.read,
		SerialStatus: status.serial,
		// 分类（0015）：只给 L1 = 该大分类全部，两级都给 = 精确到小分类。
		CategoryL1: strings.TrimSpace(r.URL.Query().Get("categoryL1")),
		CategoryL2: strings.TrimSpace(r.URL.Query().Get("categoryL2")),
	}, nil
}

// bookIDFilterValues 是整数型筛选：作者 id 与书单 id。
type bookIDFilterValues struct {
	authorID     int64
	collectionID int64
}

func parseBookIDFilters(r *http.Request) (bookIDFilterValues, error) {
	authorID, _, err := int64Query(r, "authorId")
	if err != nil {
		return bookIDFilterValues{}, err
	}
	collectionID, _, err := int64Query(r, "collection")
	if err != nil {
		return bookIDFilterValues{}, err
	}
	return bookIDFilterValues{authorID: authorID, collectionID: collectionID}, nil
}

// bookStatusFilterValues 是枚举型筛选，空串一律表示「不过滤」。
type bookStatusFilterValues struct {
	read   string
	serial string
}

func parseBookStatusFilters(r *http.Request) (bookStatusFilterValues, error) {
	read := strings.TrimSpace(r.URL.Query().Get("readStatus"))
	if read != "" && !store.ValidReadStatus(read) {
		return bookStatusFilterValues{}, badParam("参数 readStatus 只支持 want|reading|done（留空表示不过滤）")
	}
	// 连载状态筛选（管理页的筛选条）：none 筛出「未知」，留空表示不过滤。
	serial := strings.TrimSpace(r.URL.Query().Get("serialStatus"))
	if serial != "" && serial != store.SerialStatusNone && !store.ValidSerialStatus(serial) {
		return bookStatusFilterValues{}, badParam("参数 serialStatus 只支持 ongoing|completed|none（留空表示不过滤）")
	}
	return bookStatusFilterValues{read: read, serial: serial}, nil
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
	if err := validateBookPatch(body); err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	patch := body.toStorePatch()
	if patch.IsEmpty() {
		writeError(w, http.StatusBadRequest, codeInvalidParam, bookPatchFieldsHint)
		return
	}
	if err := s.st.UpdateBookMeta(r.Context(), id, patch); err != nil {
		s.fail(w, r, err, "book not found")
		return
	}
	s.writeBookDetail(w, r, id)
}

// handleDeleteBook 处理 DELETE /api/books/{id}：单事务级联删除，成功 204 无响应体。
//
// 不存在返回 404；删掉的书连同章节、正文块、卷、封面、作者关联、阅读进度一起消失，
// 作者的作品数/字数在同一事务内重算（见 store.DeleteBook）。
func (s *Server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	if err := s.st.DeleteBook(r.Context(), id); err != nil {
		s.fail(w, r, err, "book not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	if detail.Authors == nil {
		detail.Authors = []string{}
	}
	writeJSON(w, http.StatusOK, detail)
}
