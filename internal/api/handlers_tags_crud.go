package api

import (
	"context"
	"errors"
	"net/http"

	"norvels/internal/store"
)

// 标签 CRUD（P3 §管理侧补全）：标签是扁平分类，名字全库唯一（tags.name UNIQUE）。
//
// 读路径仍是 GET /api/tags（handlers_meta.go），这里只补齐写路径。
// 删除是「全库解绑」：book_tags 的关联由 store 在同一事务里清掉，书本身不动。

// tagNameRequest 是 POST/PATCH 标签的请求体。
type tagNameRequest struct {
	Name string `json:"name"`
}

// handleCreateTag 处理 POST /api/tags：新建 201，同名返回已有标签 200。
//
// 同名不报错是契约取舍：标签是分类词，重复点一次不该 409；
// 调用方（管理台）按状态码区分「新建成功」与「本来就有」的提示文案。
func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var body tagNameRequest
	if !decodeJSONBody(w, r, &body, `请求体必须是 {"name":"..."}`) {
		return
	}
	tag, err := s.st.CreateTag(r.Context(), body.Name)
	if err != nil {
		if errors.Is(err, store.ErrInvalidArgument) {
			s.failParam(w, err)
			return
		}
		s.fail(w, r, err, "标签写入失败")
		return
	}
	status := http.StatusOK
	if tag.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, tag)
}

// handleRenameTag 处理 PATCH /api/tags/{id}：改名，返回改名后的标签。
func (s *Server) handleRenameTag(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	var body tagNameRequest
	if !decodeJSONBody(w, r, &body, `请求体必须是 {"name":"..."}`) {
		return
	}
	if err := s.st.RenameTag(r.Context(), id, body.Name); err != nil {
		s.failTagRename(w, r, err)
		return
	}
	tag, ok, err := s.findTag(r.Context(), id)
	if err != nil || !ok {
		s.fail(w, r, err, "标签不存在")
		return
	}
	writeJSON(w, http.StatusOK, tag)
}

// failTagRename 把改名失败翻译成对前端有用的状态码。
//
// 重名要单独的 409 文案（「换个名字」），而不是 fail 的通用并发冲突文案；
// 非法名字沿用参数家族（400 + store 给的原因）。
func (s *Server) failTagRename(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, codeConflict, "标签名已存在")
	case errors.Is(err, store.ErrInvalidArgument):
		s.failParam(w, err)
	default:
		s.fail(w, r, err, "标签不存在")
	}
}

// handleDeleteTag 处理 DELETE /api/tags/{id}：204，全库解绑。
func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	if err := s.st.DeleteTag(r.Context(), id); err != nil {
		s.fail(w, r, err, "标签不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// findTag 从标签列表里回读指定 id 的最新行（改名后用，标签量小不必单开查询）。
func (s *Server) findTag(ctx context.Context, id int64) (store.TagSummary, bool, error) {
	items, err := s.st.ListTagsWithCount(ctx)
	if err != nil {
		return store.TagSummary{}, false, err
	}
	for _, it := range items {
		if it.ID == id {
			return it, true, nil
		}
	}
	return store.TagSummary{}, false, nil
}
