package api

import (
	"errors"
	"net/http"
	"sort"

	"norvels/internal/store"
)

// 批量改元信息（P3 管理侧补全）：一次请求最多 200 本，单事务逐本应用同一 patch。
//
// 单本失败只进 failed（带可展示 message），不整体回滚——失败项按 id 单独重试。
// 校验与单本 PATCH 共用 validateBookPatch，保证同一份 patch 在两处口径一致。

// maxBatchBooks 与 store 的上限一致（store 侧同样拒绝）。
const maxBatchBooks = 200

// batchPatchRequest 是 POST /api/books/batch 的请求体。
type batchPatchRequest struct {
	IDs   []int64   `json:"ids"`
	Patch bookPatch `json:"patch"`
}

// batchFailure 是失败清单里的一项（message 可直接展示）。
type batchFailure struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

// batchPatchResponse 对应契约里的 {updated, failed}。
type batchPatchResponse struct {
	Updated []int64        `json:"updated"`
	Failed  []batchFailure `json:"failed"`
}

// handleBatchPatchBooks 处理 POST /api/books/batch。
func (s *Server) handleBatchPatchBooks(w http.ResponseWriter, r *http.Request) {
	var body batchPatchRequest
	if !decodeJSONBody(w, r, &body, "请求体必须是 {ids:[...], patch:{...}}") {
		return
	}
	if err := validateBatchIDs(body.IDs); err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	if err := validateBookPatch(body.Patch); err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	patch := body.Patch.toStorePatch()
	if patch.IsEmpty() {
		writeError(w, http.StatusBadRequest, codeInvalidParam, bookPatchFieldsHint)
		return
	}
	updated, failed, err := s.st.PatchBooksBatch(r.Context(), body.IDs, patch)
	if err != nil {
		if errors.Is(err, store.ErrInvalidArgument) {
			s.failParam(w, err)
			return
		}
		s.fail(w, r, err, "批量更新不可用")
		return
	}
	writeJSON(w, http.StatusOK, newBatchPatchResponse(updated, failed))
}

// validateBatchIDs 校验批量书 id 列表：非空、不超上限、每个 id 是正整数。
func validateBatchIDs(ids []int64) error {
	if len(ids) == 0 {
		return badParam("ids 至少要包含一本书")
	}
	if len(ids) > maxBatchBooks {
		return badParam("一次最多 %d 本书", maxBatchBooks)
	}
	for _, id := range ids {
		if id <= 0 {
			return badParam("ids 里的书 id 必须是正整数")
		}
	}
	return nil
}

// newBatchPatchResponse 把 store 的失败 map 摊平成按 id 升序的数组，
// 并保证 updated / failed 是 [] 而不是 null（前端直接遍历）。
func newBatchPatchResponse(updated []int64, failed map[int64]error) batchPatchResponse {
	resp := batchPatchResponse{Updated: updated, Failed: make([]batchFailure, 0, len(failed))}
	if resp.Updated == nil {
		resp.Updated = []int64{}
	}
	ids := make([]int64, 0, len(failed))
	for id := range failed {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		resp.Failed = append(resp.Failed, batchFailure{ID: id, Message: batchMessage(failed[id])})
	}
	return resp
}

// batchMessage 把 store 错误翻成可展示的一句话（去掉 "store: invalid argument:" 前缀）。
func batchMessage(err error) string {
	if errors.Is(err, store.ErrInvalidArgument) {
		return paramMessage(err)
	}
	if isNotFound(err) {
		return "书籍不存在"
	}
	return "更新失败"
}
