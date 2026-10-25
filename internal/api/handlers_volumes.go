package api

import (
	"errors"
	"net/http"

	"norvels/internal/store"
)

// 卷管理（批次 C2）：新建 / 改名 / 删除 / 重排，以及章节跨卷移动。
//
// 与章节结构编辑同一口径：即时生效、不进「保存」批次；每个端点单事务，
// 失败整体回滚，不存在「卷建了但计数没动」的中间态。

// volumeTitleRequest 是新建与改名共用的请求体 {"title":"卷三"}。
type volumeTitleRequest struct {
	Title string `json:"title"`
}

// volumeResponse 返回单个卷节点。chapters 恒为空数组：卷结构刚变过，
// 章节归属交给前端重拉详情（这里不再多查一遍目录）。
type volumeResponse struct {
	Volume store.VolumeNode `json:"volume"`
}

// volumeOrderRequest 是重排请求体 {"ids":[3,1,2]}。
type volumeOrderRequest struct {
	IDs []int64 `json:"ids"`
}

// moveChaptersRequest 是移动章节请求体；volumeId=0 表示移到「未分卷」。
type moveChaptersRequest struct {
	ChapterIDs []int64 `json:"chapterIds"`
	VolumeID   int64   `json:"volumeId"`
}

// moveChaptersResponse 返回本次移动的章数。
type moveChaptersResponse struct {
	Moved int `json:"moved"`
}

// handleCreateVolume 新建卷：201 + 卷节点（idx 由 store 分配在末尾）。
func (s *Server) handleCreateVolume(w http.ResponseWriter, r *http.Request) {
	bookID, ok := s.bookPathID(w, r)
	if !ok {
		return
	}
	title, ok := s.decodeVolumeTitle(w, r)
	if !ok {
		return
	}
	volume, err := s.st.CreateVolume(r.Context(), bookID, title)
	if err != nil {
		s.failVolume(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, volumeResponse{Volume: *volume})
}

// handleRenameVolume 改卷名：200 + 卷节点。
func (s *Server) handleRenameVolume(w http.ResponseWriter, r *http.Request) {
	bookID, ok := s.bookPathID(w, r)
	if !ok {
		return
	}
	volumeID, ok := s.volumePathID(w, r)
	if !ok {
		return
	}
	title, ok := s.decodeVolumeTitle(w, r)
	if !ok {
		return
	}
	volume, err := s.st.RenameVolume(r.Context(), bookID, volumeID, title)
	if err != nil {
		s.failVolume(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, volumeResponse{Volume: *volume})
}

// handleDeleteVolume 删卷：204；卷内章节由 store 落到未分卷。
func (s *Server) handleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	bookID, ok := s.bookPathID(w, r)
	if !ok {
		return
	}
	volumeID, ok := s.volumePathID(w, r)
	if !ok {
		return
	}
	if err := s.st.DeleteVolume(r.Context(), bookID, volumeID); err != nil {
		s.failVolume(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReorderVolumes 重排卷序：200 + 空对象（前端重拉详情拿新的 idx 顺序）。
func (s *Server) handleReorderVolumes(w http.ResponseWriter, r *http.Request) {
	bookID, ok := s.bookPathID(w, r)
	if !ok {
		return
	}
	var body volumeOrderRequest
	if !decodeJSONBody(w, r, &body, `请求体必须是 {"ids":[3,1,2]}`) {
		return
	}
	if err := s.st.ReorderVolumes(r.Context(), bookID, body.IDs); err != nil {
		s.failVolume(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

// handleMoveChapters 章节跨卷移动：200 {"moved":2}；volumeId=0 表示移回未分卷。
func (s *Server) handleMoveChapters(w http.ResponseWriter, r *http.Request) {
	bookID, ok := s.bookPathID(w, r)
	if !ok {
		return
	}
	var body moveChaptersRequest
	if !decodeJSONBody(w, r, &body, `请求体必须是 {"chapterIds":[11,12],"volumeId":3}`) {
		return
	}
	moved, err := s.st.MoveChaptersToVolume(r.Context(), bookID, body.ChapterIDs, body.VolumeID)
	if err != nil {
		s.failVolume(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, moveChaptersResponse{Moved: moved})
}

// decodeVolumeTitle 解析 {"title":"..."}。空白标题交给 store 判（400 带原因），
// 这里只拦「请求体不是这个形状」。
func (s *Server) decodeVolumeTitle(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body volumeTitleRequest
	if !decodeJSONBody(w, r, &body, `请求体必须是 {"title":"卷三"}`) {
		return "", false
	}
	return body.Title, true
}

// volumePathID 解析路径里的卷 id（{vid}），失败时已写好 400。
func (s *Server) volumePathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	volumeID, err := pathID(r, "vid")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return 0, false
	}
	return volumeID, true
}

// failVolume 把卷管理类 store 错误翻成响应。
//
// 参数错误（空名 / 重排缺 id / 重名之外的入参问题）走 failParam 把原因带给前端：
// 用户要照着它改名或重选，泛化文案帮不上忙；重名由 fail 统一映射 409。
func (s *Server) failVolume(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrInvalidArgument) {
		s.failParam(w, err)
		return
	}
	s.fail(w, r, err, "卷或章节不存在（可能已经被删掉了）")
}
