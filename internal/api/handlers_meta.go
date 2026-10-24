package api

import (
	"context"
	"net/http"
	"time"

	"norvels/internal/store"
)

// healthResponse 对应 docs/api.md 的 GET /api/health。
type healthResponse struct {
	Status        string `json:"status"`
	DB            string `json:"db"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	resp := healthResponse{
		Status:        "ok",
		DB:            "ok",
		Version:       s.version(),
		UptimeSeconds: int64(time.Since(s.start).Seconds()),
	}
	if err := s.st.Ping(ctx); err != nil {
		resp.Status = "error"
		resp.DB = "error"
		s.logf("api: 健康检查失败: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.st.Stats(r.Context())
	if err != nil {
		s.fail(w, r, err, "统计不可用")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// versionResponse 对应 docs/api.md 的 GET /api/version。
type versionResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"builtAt"`
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	commit := s.cfg.Commit
	if commit == "" {
		commit = "dev"
	}
	builtAt := s.cfg.BuiltAt
	if builtAt == "" {
		builtAt = "unknown"
	}
	writeJSON(w, http.StatusOK, versionResponse{Version: s.version(), Commit: commit, BuiltAt: builtAt})
}

// tagsResponse 对应 docs/api.md 的 GET /api/tags。
type tagsResponse struct {
	Items []store.TagSummary `json:"items"`
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.ListTags(r.Context())
	if err != nil {
		s.fail(w, r, err, "标签不可用")
		return
	}
	if items == nil {
		items = []store.TagSummary{}
	}
	writeJSON(w, http.StatusOK, tagsResponse{Items: items})
}

// version 返回注入的版本号，缺省为 dev。
func (s *Server) version() string {
	if s.cfg.Version != "" {
		return s.cfg.Version
	}
	return "dev"
}
