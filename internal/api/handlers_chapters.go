package api

import (
	"errors"
	"net/http"
	"strings"

	"norvels/internal/store"
)

// 章节结构编辑（管理页 M1）：改标题 / 拆分 / 删除。
//
// 三条路径都直接改库、不进「保存」批次：章节结构与元信息草稿混在一次保存里，
// 最容易出现的错是「用户以为删掉的那章又被保存回去了」。

// chapterPatch 是 PATCH /api/books/{id}/chapters/{cid} 的请求体。
type chapterPatch struct {
	Title *string `json:"title"`
}

// handlePatchChapter 改一章标题：204 无响应体（前端已持有新标题，不必回读）。
func (s *Server) handlePatchChapter(w http.ResponseWriter, r *http.Request) {
	bookID, chapterID, ok := s.chapterPathIDs(w, r)
	if !ok {
		return
	}
	var body chapterPatch
	if !decodeJSONBody(w, r, &body, "请求体必须是 {\"title\":\"...\"}") {
		return
	}
	if body.Title == nil {
		s.badRequest(w, badParam("请求体必须包含 title"), codeInvalidParam)
		return
	}
	if err := s.st.UpdateChapterTitle(r.Context(), bookID, chapterID, *body.Title); err != nil {
		s.failChapterWrite(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// splitRequest 是 POST /api/books/{id}/chapters/{cid}/split 的请求体。
type splitRequest struct {
	// CutAfter 是 0 起的段落下标，含义「在这一段之后切开」，必须严格递增。
	CutAfter []int `json:"cutAfter"`
}

// splitResponse 返回拆完后的章数（前端据此提示「已拆分为 N 章」）。
type splitResponse struct {
	Chapters int `json:"chapters"`
}

// handleSplitChapter 按段落边界把一章拆成多章。
func (s *Server) handleSplitChapter(w http.ResponseWriter, r *http.Request) {
	bookID, chapterID, ok := s.chapterPathIDs(w, r)
	if !ok {
		return
	}
	var body splitRequest
	if !decodeJSONBody(w, r, &body, "请求体必须是 {\"cutAfter\":[0,1,…]}") {
		return
	}
	if len(body.CutAfter) == 0 {
		s.badRequest(w, badParam("cutAfter 至少给一个拆分点"), codeInvalidParam)
		return
	}
	pieces, err := s.st.SplitChapter(r.Context(), bookID, chapterID, body.CutAfter)
	if err != nil {
		s.failChapterWrite(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, splitResponse{Chapters: pieces})
}

// handleDeleteChapter 删一章：204；后面的章节序号由 store 自动前移。
func (s *Server) handleDeleteChapter(w http.ResponseWriter, r *http.Request) {
	bookID, chapterID, ok := s.chapterPathIDs(w, r)
	if !ok {
		return
	}
	if err := s.st.DeleteChapter(r.Context(), bookID, chapterID); err != nil {
		s.failChapterWrite(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// chapterContentPut 是 PUT /api/books/{id}/chapters/{cid}/content 的请求体。
type chapterContentPut struct {
	Content *string `json:"content"`
}

// chapterContentBodyLimit 是整章替换正文的请求体上限。
//
// 正文本身限 1 MiB（store 侧 fail-closed，超限给 400 + 上限文案），但 JSON 里一个字
// 可能被写成 \uXXXX（ASCII 转义后膨胀到 6 倍），所以请求体上限留到 8 MiB：
// 让「正文超限」由 store 判定并给出人能看懂的上限，而不是先被体积截断成
// 「请求体不是合法 JSON」。
const chapterContentBodyLimit = 8 << 20

// handlePutChapterContent 整章替换正文：**204** 无响应体。
//
// 为什么不回传新正文：前端保存后本来就要重拉 GET /api/books/{id}/chapters/{cid}
// （拿服务端算的 wordCount、以及被这次替换影响的相邻信息），再回一份就是两份真相。
func (s *Server) handlePutChapterContent(w http.ResponseWriter, r *http.Request) {
	bookID, chapterID, ok := s.chapterPathIDs(w, r)
	if !ok {
		return
	}
	var body chapterContentPut
	if !decodeJSONBodyLimit(w, r, &body, `请求体必须是 {"content":"…"}（正文上限 1 MiB）`,
		chapterContentBodyLimit) {
		return
	}
	if body.Content == nil {
		s.badRequest(w, badParam("请求体必须包含 content"), codeInvalidParam)
		return
	}
	if err := s.st.UpdateChapterContent(r.Context(), bookID, chapterID, *body.Content); err != nil {
		s.failChapterWrite(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// chapterPathIDs 解析路径里的书籍 id 与章节 id，失败时已写好 400/404 响应。
func (s *Server) chapterPathIDs(w http.ResponseWriter, r *http.Request) (bookID, chapterID int64, ok bool) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return 0, 0, false
	}
	chapterID, err = pathID(r, "cid")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return 0, 0, false
	}
	return bookID, chapterID, true
}

// failChapterWrite 把章节写路径的错误翻成响应。
//
// 入参非法要带上原因（「拆分点必须严格递增」这种话对用户有用），
// 其余走统一的 fail 映射；块损坏仍按 503 + content_corrupted 局部故障处理。
func (s *Server) failChapterWrite(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrInvalidArgument) {
		s.failParam(w, err)
		return
	}
	msg := "章节不存在"
	if strings.Contains(r.Method, http.MethodDelete) {
		msg = "章节不存在（可能已经被删掉了）"
	}
	s.fail(w, r, err, msg)
}
