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
	// AuthToken 非空时启用鉴权：/api/* 与页面要求 Bearer Token 或 Cookie，
	// /api/health 与 /login 免鉴权。空 = 开放模式（启动时由调用方打 WARN）。
	AuthToken string
	// UploadDir 是上传临时根目录：每个任务建一个子目录，导入结束整目录删除。
	// 空值回落到 os.TempDir()；生产建议指向数据库同目录下的 uploads/。
	UploadDir string
	// MaxUploadFileBytes / MaxUploadTotalBytes / MaxUploadFiles 是上传校验上限，
	// 0 值表示用默认值（64 MiB / 256 MiB / 20）。测试注入小值以快速触发 413/400。
	MaxUploadFileBytes  int64
	MaxUploadTotalBytes int64
	MaxUploadFiles      int
}

// AuthEnabled 报告是否启用鉴权。
func (c Config) AuthEnabled() bool { return c.AuthToken != "" }

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
	s := &Server{st: st, cfg: cfg, fsys: fsys, log: log.Default(), start: time.Now(),
		placeholder: webui.PlaceholderHTML()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/books", s.handleListBooks)
	mux.HandleFunc("GET /api/books/{id}", s.handleGetBook)
	mux.HandleFunc("GET /api/books/{id}/chapters", s.handleListChapters)
	mux.HandleFunc("GET /api/books/{id}/chapters/{cid}", s.handleGetChapter)
	mux.HandleFunc("PATCH /api/books/{id}", s.handlePatchBook)
	mux.HandleFunc("DELETE /api/books/{id}", s.handleDeleteBook)
	mux.HandleFunc("GET /api/books/{id}/cover", s.handleGetBookCover)
	// 封面写入（管理页 M1）：上传走 multipart，删除是幂等的（本来没有也 204）。
	mux.HandleFunc("PUT /api/books/{id}/cover", s.handlePutBookCover)
	mux.HandleFunc("DELETE /api/books/{id}/cover", s.handleDeleteBookCover)
	// 章节结构编辑（管理页 M1）：改标题 / 拆分 / 删除都是即时生效，不进「保存」批次。
	mux.HandleFunc("PATCH /api/books/{id}/chapters/{cid}", s.handlePatchChapter)
	mux.HandleFunc("DELETE /api/books/{id}/chapters/{cid}", s.handleDeleteChapter)
	mux.HandleFunc("POST /api/books/{id}/chapters/{cid}/split", s.handleSplitChapter)
	// 正文编辑（M1）：整章替换，单事务写新块 + 重算计数 + 回收旧块。
	mux.HandleFunc("PUT /api/books/{id}/chapters/{cid}/content", s.handlePutChapterContent)
	// 章节批量编辑（M2）：批量删除 / 合并 / 重编号。单事务，不做「部分成功」。
	mux.HandleFunc("POST /api/books/{id}/chapters/batch-delete", s.handleBatchDeleteChapters)
	mux.HandleFunc("POST /api/books/{id}/chapters/merge", s.handleMergeChapters)
	mux.HandleFunc("POST /api/books/{id}/chapters/renumber", s.handleRenumberChapters)
	// 卷管理（C2）：新建 / 改名 / 删除 / 重排 + 章节跨卷移动，均单事务。
	mux.HandleFunc("POST /api/books/{id}/volumes", s.handleCreateVolume)
	mux.HandleFunc("PATCH /api/books/{id}/volumes/{vid}", s.handleRenameVolume)
	mux.HandleFunc("DELETE /api/books/{id}/volumes/{vid}", s.handleDeleteVolume)
	mux.HandleFunc("PUT /api/books/{id}/volumes/order", s.handleReorderVolumes)
	mux.HandleFunc("POST /api/books/{id}/chapters/move", s.handleMoveChapters)
	// 书签（P1 阅读体验 spec §2.2）：GET/POST 挂在书下，DELETE 按书签 id 直删。
	mux.HandleFunc("GET /api/books/{id}/bookmarks", s.handleListBookmarks)
	mux.HandleFunc("POST /api/books/{id}/bookmarks", s.handleCreateBookmark)
	mux.HandleFunc("DELETE /api/bookmarks/{id}", s.handleDeleteBookmark)
	mux.HandleFunc("GET /api/authors", s.handleListAuthors)
	mux.HandleFunc("GET /api/authors/{id}", s.handleGetAuthor)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/tags", s.handleListTags)
	mux.HandleFunc("POST /api/import", s.handleStartImport)
	mux.HandleFunc("POST /api/import/upload", s.handleUploadImport)
	mux.HandleFunc("GET /api/import/jobs", s.handleListImportJobs)
	mux.HandleFunc("GET /api/import/jobs/{id}", s.handleGetImportJob)
	mux.HandleFunc("POST /api/import/jobs/{id}/cancel", s.handleCancelImportJob)
	// P1-B 导入侧：文件级终态列表与单文件重试（spec §2.2）。
	mux.HandleFunc("GET /api/import/jobs/{id}/files", s.handleListImportJobFiles)
	mux.HandleFunc("POST /api/import/jobs/{id}/retry", s.handleRetryImportJob)
	// 全局设置（spec §2.2）：自定义章节识别规则 + 词表打标规则。
	mux.HandleFunc("GET /api/settings/chapter-rules", s.handleGetChapterRules)
	mux.HandleFunc("PUT /api/settings/chapter-rules", s.handlePutChapterRules)
	mux.HandleFunc("GET /api/settings/tag-rules", s.handleGetTagRules)
	mux.HandleFunc("PUT /api/settings/tag-rules", s.handlePutTagRules)
	mux.HandleFunc("GET /api/version", s.handleVersion)
	// 驾驶舱（docs/api.md §8）
	mux.HandleFunc("GET /api/health/report", s.handleHealthReport)
	mux.HandleFunc("GET /api/stats/import-trend", s.handleImportTrend)
	mux.HandleFunc("GET /api/reading/progress", s.handleListProgress)
	mux.HandleFunc("PUT /api/reading/progress", s.handleSaveProgress)
	mux.HandleFunc("GET "+LoginPath, s.handleLoginPage)
	mux.HandleFunc("POST "+LoginPath, s.handleLoginSubmit)
	mux.HandleFunc("POST "+logoutPath, s.handleLogout)
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
