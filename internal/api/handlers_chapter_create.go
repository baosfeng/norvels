package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 新建章节（P3 §管理侧补全）：追加到末尾或插到指定章之后，单事务写正文块并重排 idx。

const (
	// maxCreateChapterBodyBytes 是建章请求体上限：1 MiB 正文 + JSON 转义余量。
	maxCreateChapterBodyBytes = 2 << 20
	// maxCreateChapterContentBytes 与 store 的正文上限一致（1 MiB，UTF-8 编码后）。
	maxCreateChapterContentBytes = 1 << 20
)

// createChapterRequest 是 POST /api/books/{id}/chapters 的请求体。
type createChapterRequest struct {
	Title          string `json:"title"`
	Content        string `json:"content"`
	AfterChapterID int64  `json:"afterChapterId"`
}

// handleCreateChapter 处理 POST /api/books/{id}/chapters：201 + 新章目录字段。
func (s *Server) handleCreateChapter(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	var body createChapterRequest
	if !decodeJSONBodyLimit(w, r, &body,
		`请求体必须是 {"title":"...","content":"..."}`, maxCreateChapterBodyBytes) {
		return
	}
	if err := validateCreateChapter(body); err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	newID, err := s.st.InsertChapter(r.Context(), bookID,
		model.ParsedChapter{Title: body.Title, Content: body.Content}, body.AfterChapterID)
	if err != nil {
		s.failCreateChapter(w, r, err, body.AfterChapterID)
		return
	}
	brief, err := s.chapterBriefOf(r.Context(), bookID, newID)
	if err != nil {
		s.fail(w, r, err, "chapter not found")
		return
	}
	writeJSON(w, http.StatusCreated, brief)
}

// validateCreateChapter 做字段级校验：标题/正文去空白后非空、正文 ≤1 MiB、落点非负。
func validateCreateChapter(b createChapterRequest) error {
	if strings.TrimSpace(b.Title) == "" {
		return badParam("title 不能为空")
	}
	if strings.TrimSpace(b.Content) == "" {
		return badParam("content 不能为空（去空白后为空）")
	}
	if len(b.Content) > maxCreateChapterContentBytes {
		return badParam("content 超过 %d 字节（1 MiB）", maxCreateChapterContentBytes)
	}
	if b.AfterChapterID < 0 {
		return badParam("afterChapterId 不能为负")
	}
	return nil
}

// failCreateChapter：afterChapterId 不属于这本书是请求错误（400），书不存在是 404。
//
// store 对两者都返回 ErrNotFound（章节查询带 book_id 条件），api 只能靠
// 「有没有给 afterChapterId」区分「该刷新目录」还是「该改请求」。
func (s *Server) failCreateChapter(w http.ResponseWriter, r *http.Request, err error, afterChapterID int64) {
	switch {
	case afterChapterID > 0 && errors.Is(err, store.ErrNotFound):
		s.badRequest(w, badParam("afterChapterId 不属于这本书"), codeInvalidParam)
	case errors.Is(err, store.ErrInvalidArgument):
		s.failParam(w, err)
	default:
		s.fail(w, r, err, "book not found")
	}
}

// chapterBriefOf 读回新章在目录里的核心字段（含卷归属）：按 idx 定位到「一页一章」。
//
// 新章刚写完，直接读正文块只为拿 idx/字数；目录结构以详情 reload 为准。
func (s *Server) chapterBriefOf(ctx context.Context, bookID, chapterID int64) (store.ChapterBrief, error) {
	ch, err := s.st.GetChapter(ctx, bookID, chapterID)
	if err != nil {
		return store.ChapterBrief{}, err
	}
	if ch == nil {
		return store.ChapterBrief{}, badParam("新章节读回失败")
	}
	brief := store.ChapterBrief{ID: ch.ID, Idx: ch.Idx, Title: ch.Title, WordCount: ch.WordCount}
	page, err := s.st.ListChapters(ctx, store.ChapterQuery{BookID: bookID, From: ch.Idx + 1, Limit: 1})
	if err != nil {
		return brief, nil // 卷归属取不到不影响建章成功（前端会 reload 详情）
	}
	if len(page.Items) == 1 && page.Items[0].ID == chapterID {
		brief.VolumeID = page.Items[0].VolumeID
	}
	return brief, nil
}
