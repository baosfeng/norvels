package api

import (
	"net/http"

	"norvels/internal/releases"
)

// releasesResponse 对应 docs/api/api-dashboard.md 的 GET /api/releases。
//
// current 只在该版本确实有条目时才给值（否则空串）：前端据此决定「正在使用」的高亮，
// 用运行版本顶替不存在的条目会让用户以为「这版没有更新说明」是设计如此。
type releasesResponse struct {
	Current string           `json:"current"`
	Items   []releases.Entry `json:"items"`
}

// handleReleases 返回内置发版说明（internal/releases/CHANGELOG.md，go:embed 进二进制）。
//
// 解析失败按 500 返回而不是给空列表：说明文件是随二进制发布的（不来自外部输入），
// 解析不了就是发布事故，用空列表顶替只会让「产品里没有更新日志」看起来像正常情况。
func (s *Server) handleReleases(w http.ResponseWriter, r *http.Request) {
	notes, err := s.releaseNotes()
	if err != nil {
		s.logf("api: 解析发版说明失败: %v", err)
		writeError(w, http.StatusInternalServerError, codeInternal, "发版说明不可用")
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

// releaseNotes 解析说明源并包成响应；current 与说明文件对齐（见 releaseVersion）。
func (s *Server) releaseNotes() (releasesResponse, error) {
	notes, err := releases.Build(s.releaseMD, s.releaseVersion())
	if err != nil {
		return releasesResponse{}, err
	}
	if notes.Items == nil {
		notes.Items = []releases.Entry{}
	}
	return releasesResponse{Current: notes.Current, Items: notes.Items}, nil
}

// releaseVersion 返回与说明文件对齐用的版本号：dev 构建没有真实版本，不做匹配。
func (s *Server) releaseVersion() string {
	if v := s.version(); v != "" && v != "dev" {
		return v
	}
	return ""
}
