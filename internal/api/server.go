// Package api 提供 REST API 与前端静态资源服务。
//
// 路由清单见 docs/api.md；本包只依赖 internal/store 的类型与方法、internal/webui 的静态 FS。
package api

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"norvels/internal/store"
	"norvels/internal/webui"
)

// Config 是服务端配置。
type Config struct {
	Version    string // 版本号，注入 /api/health 与 /api/version
	Commit     string
	BuiltAt    string
	EnableCORS bool // 开发期前后端分离时开启（仅放行 localhost:5173）
	// RequestID 为 true 时生成 X-Request-Id 响应头并写入访问日志。
	RequestID bool
	// DisableAccessLog 关闭访问日志（测试或需要完全安静的场景）。
	DisableAccessLog bool
	// Context 是导入任务的父 context；serve 关闭时应取消它以优雅停止导入。
	// 为空时使用 context.Background()。
	Context context.Context
}

// Server 是 API 聚合器。
type Server struct {
	st    *store.Store
	cfg   Config
	fsys  fs.FS
	log   *log.Logger
	start time.Time
	// placeholder 是前端未构建时 SPA fallback 使用的占位页。
	placeholder []byte
	// staticCache 缓存静态资源摘要（键为 FS 内路径，含负缓存）。
	staticCache sync.Map
	// importMu 让「查 running 任务 + 建任务」成为临界区，避免并发 POST 各建一个任务。
	importMu sync.Mutex
}

// New 构造 HTTP handler（含 API 与 SPA 静态资源）。
func New(st *store.Store, cfg Config) http.Handler {
	_, h := newServer(st, cfg, webui.FS())
	return h
}

// newServer 与 New 等价，但返回 *Server 以便测试操纵内部状态与注入 FS。
func newServer(st *store.Store, cfg Config, fsys fs.FS) (*Server, http.Handler) {
	s := &Server{
		st:          st,
		cfg:         cfg,
		fsys:        fsys,
		log:         log.Default(),
		start:       time.Now(),
		placeholder: webui.PlaceholderHTML(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/books", s.handleListBooks)
	mux.HandleFunc("GET /api/books/{id}", s.handleGetBook)
	mux.HandleFunc("GET /api/books/{id}/chapters", s.handleListChapters)
	mux.HandleFunc("GET /api/books/{id}/chapters/{cid}", s.handleGetChapter)
	mux.HandleFunc("GET /api/authors", s.handleListAuthors)
	mux.HandleFunc("GET /api/authors/{id}", s.handleGetAuthor)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/tags", s.handleListTags)
	mux.HandleFunc("POST /api/import", s.handleStartImport)
	mux.HandleFunc("GET /api/import/jobs", s.handleListImportJobs)
	mux.HandleFunc("GET /api/import/jobs/{id}", s.handleGetImportJob)
	mux.HandleFunc("GET /api/version", s.handleVersion)
	// 未命中任何 API 路由时统一返回 JSON 错误（否则 mux 会写纯文本）。
	mux.HandleFunc("/api", s.handleAPINotFound)
	mux.HandleFunc("/api/", s.handleAPINotFound)
	return s, s.wrap(mux)
}

// baseContext 返回导入任务的父 context。
func (s *Server) baseContext() context.Context {
	if s.cfg.Context != nil {
		return s.cfg.Context
	}
	return context.Background()
}
