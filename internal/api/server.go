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
//
// 路由按领域分四组注册：新端点加进对应的 register*，newServer 本身保持极短
// （门禁 5 的单函数 70 行上限对路由表同样适用）。
func newServer(st *store.Store, cfg Config, fsys fs.FS) (*Server, http.Handler) {
	s := &Server{st: st, cfg: cfg, fsys: fsys, log: log.Default(), start: time.Now(),
		placeholder: webui.PlaceholderHTML()}
	mux := http.NewServeMux()
	s.registerBookRoutes(mux)
	s.registerLibraryRoutes(mux)
	s.registerImportRoutes(mux)
	s.registerMiscRoutes(mux)
	// 未命中任何 API 路由时统一返回 JSON 错误（否则 mux 会写纯文本）。
	mux.HandleFunc("/api", s.handleAPINotFound)
	mux.HandleFunc("/api/", s.handleAPINotFound)
	return s, s.wrap(mux)
}

// registerBookRoutes 注册「书 → 章节 → 卷 → 书签」主链路与书的写操作。
func (s *Server) registerBookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/books", s.handleListBooks)
	mux.HandleFunc("GET /api/books/{id}", s.handleGetBook)
	// P3 发现/管理：随机一本、批量改元信息、新建章节（字面量路由优先于 {id}）。
	mux.HandleFunc("GET /api/books/random", s.handleRandomBook)
	mux.HandleFunc("POST /api/books/batch", s.handleBatchPatchBooks)
	mux.HandleFunc("POST /api/books/{id}/chapters", s.handleCreateChapter)
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
	// 追更角标确认：读完最后一章后清零 new_chapters。
	mux.HandleFunc("POST /api/books/{id}/ack-new-chapters", s.handleAckNewChapters)
}

// registerLibraryRoutes 注册作者 / 系列 / 检索 / 标签 / 书单的读写端点。
func (s *Server) registerLibraryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/authors", s.handleListAuthors)
	mux.HandleFunc("GET /api/authors/{id}", s.handleGetAuthor)
	// 系列聚合（P3）：按 books.series 分组，无独立系列实体。
	mux.HandleFunc("GET /api/series", s.handleListSeries)
	mux.HandleFunc("GET /api/series/{name}", s.handleGetSeries)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/tags", s.handleListTags)
	// 标签 CRUD（P3 管理侧补全）：同名新建返回已有（201/200），删除全库解绑。
	mux.HandleFunc("POST /api/tags", s.handleCreateTag)
	mux.HandleFunc("PATCH /api/tags/{id}", s.handleRenameTag)
	mux.HandleFunc("DELETE /api/tags/{id}", s.handleDeleteTag)
	// 书单（P3 §自定义书单）：扁平收藏夹；书目全量替换，空数组即清空。
	mux.HandleFunc("GET /api/collections", s.handleListCollections)
	mux.HandleFunc("POST /api/collections", s.handleCreateCollection)
	mux.HandleFunc("PATCH /api/collections/{id}", s.handleRenameCollection)
	mux.HandleFunc("DELETE /api/collections/{id}", s.handleDeleteCollection)
	mux.HandleFunc("PUT /api/collections/{id}/books", s.handleReplaceCollectionBooks)
}

// registerImportRoutes 注册导入任务、文件级终态与重试。
func (s *Server) registerImportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/import", s.handleStartImport)
	mux.HandleFunc("POST /api/import/upload", s.handleUploadImport)
	mux.HandleFunc("GET /api/import/jobs", s.handleListImportJobs)
	mux.HandleFunc("GET /api/import/jobs/{id}", s.handleGetImportJob)
	mux.HandleFunc("POST /api/import/jobs/{id}/cancel", s.handleCancelImportJob)
	// P1-B 导入侧：文件级终态列表与单文件重试（spec §2.2）。
	mux.HandleFunc("GET /api/import/jobs/{id}/files", s.handleListImportJobFiles)
	mux.HandleFunc("POST /api/import/jobs/{id}/retry", s.handleRetryImportJob)
}

// registerMiscRoutes 注册健康检查、统计、设置、阅读进度与登录。
func (s *Server) registerMiscRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/stats", s.handleStats)
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
	// 阅读时长与历史（P3）：会话明细 → 统计/热力图与按书聚合的历史。
	mux.HandleFunc("POST /api/reading/sessions", s.handleAddReadingSession)
	mux.HandleFunc("GET /api/reading/history", s.handleReadingHistory)
	mux.HandleFunc("GET /api/stats/reading", s.handleReadingStats)
	mux.HandleFunc("GET "+LoginPath, s.handleLoginPage)
	mux.HandleFunc("POST "+LoginPath, s.handleLoginSubmit)
	mux.HandleFunc("POST "+logoutPath, s.handleLogout)
}

// baseContext 返回导入任务的父 context。
func (s *Server) baseContext() context.Context {
	if s.cfg.Context != nil {
		return s.cfg.Context
	}
	return context.Background()
}
