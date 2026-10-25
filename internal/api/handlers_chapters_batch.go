package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"norvels/internal/store"
)

// 章节批量编辑（管理页 M2）：批量删除 / 合并 / 重编号。
//
// 与单章三个动作同一口径：即时生效、不进「保存」批次；一次请求要么全成要么全不动
// （事务在 store 侧）。批量不给前端「逐章下发」的机会：删到一半撞上「某章已不存在」
// 会留下半成品目录，用户看到的章数和他勾选的对不上，还要自己找哪章没删掉。

// chapterIDsRequest 是批量删除与合并共用的请求体 {"chapterIds":[1,2]}。
type chapterIDsRequest struct {
	ChapterIDs []int64 `json:"chapterIds"`
}

// mergeResponse 返回合并后**全书章数**，而不是「这次并了几章」——并完恒为 1 章，
// 那个数对前端没信息量；用户要看到的提示是「已合并，全书还剩 N 章」。
type mergeResponse struct {
	Chapters int `json:"chapters"`
}

// renumberRequest 是重编号请求体：{"scope":"book"} 或 {"scope":"volume","volumeId":7}。
// scope=book 时 volumeId 被忽略（多余字段一概不管，前端不必先把它清空）。
type renumberRequest struct {
	Scope    string `json:"scope"`
	VolumeID int64  `json:"volumeId"`
}

// renumberResponse 返回本次重编号覆盖的章数：标题本来就对的章不会写库，
// 但这个数就是用户看到的「被重排的章的范围」，足以支撑「已重编号 N 章」的提示。
type renumberResponse struct {
	Renamed int `json:"renamed"`
}

// handleBatchDeleteChapters 批量删章：204；其后章节序号由 store 在同一事务里一次重排。
func (s *Server) handleBatchDeleteChapters(w http.ResponseWriter, r *http.Request) {
	bookID, ok := s.bookPathID(w, r)
	if !ok {
		return
	}
	ids, ok := s.decodeChapterIDs(w, r)
	if !ok {
		return
	}
	if err := s.st.DeleteChapters(r.Context(), bookID, ids); err != nil {
		s.failChapterBatch(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMergeChapters 合并同卷连续章节：200 + 全书章数；首章保留 id。
func (s *Server) handleMergeChapters(w http.ResponseWriter, r *http.Request) {
	bookID, ok := s.bookPathID(w, r)
	if !ok {
		return
	}
	ids, ok := s.decodeChapterIDs(w, r)
	if !ok {
		return
	}
	// 「少于 2 章」在这里就拦成 400：store 把它归进 ErrNotMergeable（=409），
	// 但勾一章是调用方把参数用错了，不是「这份目录结构上合不了」。
	if len(ids) < 2 {
		s.badRequest(w, badParam("至少选 2 章才能合并（给了 %d 章）", len(ids)), codeInvalidParam)
		return
	}
	if err := s.st.MergeChapters(r.Context(), bookID, ids); err != nil {
		s.failChapterBatch(w, r, err)
		return
	}
	count, err := s.bookChapterCount(r.Context(), bookID)
	if err != nil {
		s.fail(w, r, err, "书籍不存在")
		return
	}
	writeJSON(w, http.StatusOK, mergeResponse{Chapters: count})
}

// handleRenumberChapters 按 idx+1 重写标题：200 + 覆盖章数；只改 title，不动正文与 idx。
func (s *Server) handleRenumberChapters(w http.ResponseWriter, r *http.Request) {
	bookID, ok := s.bookPathID(w, r)
	if !ok {
		return
	}
	var body renumberRequest
	if !decodeJSONBody(w, r, &body, `请求体必须是 {"scope":"book"} 或 {"scope":"volume","volumeId":7}`) {
		return
	}
	scope, err := parseRenumberScope(body)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	if err := s.st.RenumberChapters(r.Context(), bookID, scope); err != nil {
		s.failChapterBatch(w, r, err)
		return
	}
	renamed, err := s.renumberedCount(r.Context(), bookID, scope)
	if err != nil {
		s.fail(w, r, err, "书籍不存在")
		return
	}
	writeJSON(w, http.StatusOK, renumberResponse{Renamed: renamed})
}

// parseRenumberScope 把请求体翻成 store.RenumberScope；scope 非法或 volume 缺 id 都是 400。
//
// 缺省值不兜底（空 scope 不当 book）：重编号会改写整本目录的标题，猜错范围的代价
// 远大于让前端多写四个字母。
func parseRenumberScope(body renumberRequest) (store.RenumberScope, error) {
	switch body.Scope {
	case "book":
		return store.RenumberScope{}, nil
	case "volume":
		if body.VolumeID <= 0 {
			return store.RenumberScope{}, badParam("scope=volume 时必须给正整数 volumeId")
		}
		return store.RenumberScope{VolumeID: body.VolumeID}, nil
	default:
		return store.RenumberScope{}, badParam(`scope 只能是 "book" 或 "volume"（收到 %q）`, body.Scope)
	}
}

// bookPathID 解析路径里的书籍 id，失败时已写好 400。
func (s *Server) bookPathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return 0, false
	}
	return bookID, true
}

// decodeChapterIDs 解析 {"chapterIds":[...]}；空数组先拦成 400，
// 免得用户「什么都没选」被 store 的参数校验文案解释成别的问题。
func (s *Server) decodeChapterIDs(w http.ResponseWriter, r *http.Request) ([]int64, bool) {
	var body chapterIDsRequest
	if !decodeJSONBody(w, r, &body, `请求体必须是 {"chapterIds":[1,2]}`) {
		return nil, false
	}
	if len(body.ChapterIDs) == 0 {
		s.badRequest(w, badParam("chapterIds 不能为空"), codeInvalidParam)
		return nil, false
	}
	return body.ChapterIDs, true
}

// bookChapterCount 读全书章数（合并后回填用）。
func (s *Server) bookChapterCount(ctx context.Context, bookID int64) (int, error) {
	detail, err := s.st.GetBook(ctx, bookID)
	if err != nil {
		return 0, err
	}
	return detail.ChapterCount, nil
}

// renumberedCount 数出这次重编号覆盖了哪些章（全书 = 总章数；单卷 = 该卷章数）。
func (s *Server) renumberedCount(ctx context.Context, bookID int64, scope store.RenumberScope) (int, error) {
	detail, err := s.st.GetBook(ctx, bookID)
	if err != nil {
		return 0, err
	}
	if scope.VolumeID == 0 {
		return detail.ChapterCount, nil
	}
	total := 0
	for _, volume := range detail.Volumes {
		if volume.ID == scope.VolumeID {
			total += len(volume.Chapters)
		}
	}
	return total, nil
}

// failChapterBatch 把批量/合并/重编号的 store 错误翻成响应。
//
// 合并冲突用 409 而不是 400：跨卷、不连续不是「参数写错了」，而是这份目录结构上
// 没有可执行的合并——原样重发改好的请求也没用，得让用户重选章节。400 会让前端把它
// 当成表单校验错误，把一个还能继续的操作整块标红；原因（跨卷 / 不连续）原样带回，
// 因为用户唯一能做的动作就是照着它重选。
func (s *Server) failChapterBatch(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotMergeable) {
		s.logf("api: %s %s 无法合并: %v", r.Method, r.URL.Path, err)
		writeError(w, http.StatusConflict, codeNotMergeable, notMergeableMessage(err))
		return
	}
	if errors.Is(err, store.ErrInvalidArgument) {
		s.failParam(w, err)
		return
	}
	// 其余（章节/卷不属于这本书、已删）走统一映射：404，并提示「可能已被删掉」，
	// 因为批量操作最常见的失败就是多端同时改目录。
	s.fail(w, r, err, "章节或卷不存在（可能已经被删掉了）")
}

// notMergeableMessage 只取 store 错误里的原因段（跨卷 / 不连续 / 少于 2 章）。
func notMergeableMessage(err error) string {
	msg := strings.TrimPrefix(err.Error(), store.ErrNotMergeable.Error())
	msg = strings.TrimPrefix(msg, ": ")
	if msg == "" {
		return "所选章节无法合并（需要同卷且连续）"
	}
	return msg
}
