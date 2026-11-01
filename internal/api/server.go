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

	"norvels/internal/ai"
	"norvels/internal/recstore"
	"norvels/internal/ruleimport"
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
	// RuleImportOptions 透传给 internal/ruleimport（取回 URL / legado:// 链接）：
	// 零值用默认受限 HTTP 客户端；测试注入 Fetcher 避免真实出网。
	RuleImportOptions ruleimport.Options
	// ---- AI 审核（设计 §6）----
	// APIKey 只从环境变量 NORVELS_AI_API_KEY 进内存，**永不落库、永不进日志**；
	// 空值表示功能等同不存在（worker 把待审记录直接落 needs_manual）。
	AIAPIKey string
	// AIBaseURL / AIModel 是 OpenAI 兼容端点与模型名；空值用 internal/ai 的默认值
	// （DeepSeek 官方端点 + deepseek-chat）。
	AIBaseURL string
	AIModel   string
	// ---- 推荐系统（设计 §1）----
	// Rec 是独立的推荐库（norvels-rec.db）。为 nil 时 /api/events 与
	// /api/recommend 返回 503，其余端点完全不受影响——推荐是辅助能力，
	// 缺库（磁盘满、旧版本残留）不该把整个服务拖成起不来。
	Rec *recstore.Store
}

// AIJudge 按配置构造裁决器；没有 key 时返回 nil（调用方按「功能不存在」处理）。
//
// 放在 api 包而不是 cmd：worker 也在这个包里，构造与使用同一处，cmd 只传配置字符串，
// 不必为了拿一个接口值而依赖 internal/ai。
func (c Config) AIJudge() ai.Judge {
	if c.AIAPIKey == "" {
		return nil
	}
	return ai.New(ai.Options{APIKey: c.AIAPIKey, BaseURL: c.AIBaseURL, Model: c.AIModel})
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
	s.registerRecommendRoutes(mux)
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
	// 我的书架（0016）：一本书只属于一个书架，加入 / 移出幂等。
	mux.HandleFunc("GET /api/shelf", s.handleListShelf)
	mux.HandleFunc("GET /api/shelf/ids", s.handleShelfBookIDs)
	mux.HandleFunc("PUT /api/shelf/{bookId}", s.handleAddToShelf)
	mux.HandleFunc("DELETE /api/shelf/{bookId}", s.handleRemoveFromShelf)
}

// registerLibraryRoutes 注册作者 / 系列 / 检索 / 标签 / 书单的读写端点。
func (s *Server) registerLibraryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/authors", s.handleListAuthors)
	mux.HandleFunc("GET /api/authors/{id}", s.handleGetAuthor)
	// 系列聚合（P3）：按 books.series 分组，无独立系列实体。
	mux.HandleFunc("GET /api/series", s.handleListSeries)
	mux.HandleFunc("GET /api/series/{name}", s.handleGetSeries)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	// 分类聚合（0015）：大分类 → 小分类 → 书数，供书架筛选器与分类页使用。
	mux.HandleFunc("GET /api/categories", s.handleListCategories)
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
	// 来源规则包（可插拔导入 P1 切片 1）：全量替换 + 只校验不落库。
	mux.HandleFunc("GET "+sourceProfilesPath, s.handleGetSourceProfiles)
	mux.HandleFunc("PUT "+sourceProfilesPath, s.handlePutSourceProfiles)
	mux.HandleFunc("POST "+sourceProfilesPath+"/validate", s.handleValidateSourceProfiles)
	// Legado 书源导入（预览 + 应用）：粘贴 JSON → 适配成规则包。
	mux.HandleFunc("POST "+sourceProfilesPath+"/import", s.handleImportLegadoSources)
	// AI 审核（0021，设计 §6 / §7）：列表 + 替换/保留 + 回滚。
	// 升级候选导入时就会落一条 pending 记录，替换必须人工确认，绝不自动替换。
	mux.HandleFunc("GET /api/import/reviews", s.handleListImportReviews)
	mux.HandleFunc("POST /api/import/reviews/{id}/decide", s.handleDecideImportReview)
	mux.HandleFunc("POST /api/import/reviews/{id}/rollback", s.handleRollbackImportReview)
}

// registerMiscRoutes 注册健康检查、统计、设置、阅读进度与登录。
func (s *Server) registerMiscRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	// 全局设置（spec §2.2）：自定义章节识别规则 + 词表打标规则。
	mux.HandleFunc("GET /api/settings/chapter-rules", s.handleGetChapterRules)
	mux.HandleFunc("PUT /api/settings/chapter-rules", s.handlePutChapterRules)
	mux.HandleFunc("GET /api/settings/tag-rules", s.handleGetTagRules)
	// 全局净化规则（Legado replaceRule）：读写 + 试跑（不落库）。
	mux.HandleFunc("GET /api/settings/replace-rules", s.handleGetReplaceRules)
	mux.HandleFunc("PUT /api/settings/replace-rules", s.handlePutReplaceRules)
	mux.HandleFunc("POST /api/settings/replace-rules/preview", s.handlePreviewReplaceRules)
	mux.HandleFunc("PUT /api/settings/tag-rules", s.handlePutTagRules)
	// 内容去重阈值（0019 起）：章节命中率 ≥ 阈值视为同一本书。
	mux.HandleFunc("GET /api/settings/dedupe", s.handleGetDedupeSettings)
	mux.HandleFunc("PUT /api/settings/dedupe", s.handlePutDedupeSettings)
	// AI 审核开关（设计 §6）：开关落库（ai.enabled），key 只从环境变量进内存。
	mux.HandleFunc("GET /api/settings/ai", s.handleGetAISettings)
	mux.HandleFunc("PUT /api/settings/ai", s.handlePutAISettings)
	// 能力清单（策略注册表 + 上限）：前端硬编码上限的唯一来源。
	mux.HandleFunc("GET "+capabilitiesPath, s.handleCapabilities)
	mux.HandleFunc("GET /api/version", s.handleVersion)
	// 驾驶舱（docs/api.md §8）
	mux.HandleFunc("GET /api/health/report", s.handleHealthReport)
	// 健康卡下钻：某一类问题的书 / 章节明细（判定口径与报告同源）。
	mux.HandleFunc("GET /api/health/issues", s.handleHealthIssues)
	mux.HandleFunc("GET /api/stats/import-trend", s.handleImportTrend)
	mux.HandleFunc("GET /api/reading/progress", s.handleListProgress)
	mux.HandleFunc("PUT /api/reading/progress", s.handleSaveProgress)
	// 阅读时长与历史（P3）：会话明细 → 统计/热力图与按书聚合的历史。
	mux.HandleFunc("POST /api/reading/sessions", s.handleAddReadingSession)
	mux.HandleFunc("GET /api/reading/history", s.handleReadingHistory)
	mux.HandleFunc("DELETE /api/reading/history", s.handleClearReadingHistory)
	mux.HandleFunc("DELETE /api/reading/history/{bookId}", s.handleDeleteReadingHistory)
	mux.HandleFunc("GET /api/stats/reading", s.handleReadingStats)
	mux.HandleFunc("GET "+LoginPath, s.handleLoginPage)
	mux.HandleFunc("POST "+LoginPath, s.handleLoginSubmit)
	mux.HandleFunc("POST "+logoutPath, s.handleLogout)
	// 账号（0016）：只注册 + 输账号登录，无密码。注册即登录。
	mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("GET /api/auth/me", s.handleAuthMe)
	// 登录页的「选账号」列表：免鉴权（理由见 handleListUsers 注释）。
	mux.HandleFunc("GET /api/auth/users", s.handleListUsers)
}

// registerRecommendRoutes 注册发现页的埋点上报与推荐流。
//
// 单独一组而不是并进 registerMiscRoutes：这两个端点依赖独立库（Config.Rec），
// 缺失时整体降级为 503，放在单独函数里让「哪些端点会受推荐库影响」一眼可见。
func (s *Server) registerRecommendRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/events", s.handleRecordEvents)
	mux.HandleFunc("GET /api/recommend", s.handleRecommend)
	// 看板聚合（驾驶舱「推荐与埋点」区块）：只读，口径见 handlers_recommend_stats.go。
	mux.HandleFunc("GET /api/recommend/stats", s.handleRecommendStats)
}

// baseContext 返回导入任务的父 context。
func (s *Server) baseContext() context.Context {
	if s.cfg.Context != nil {
		return s.cfg.Context
	}
	return context.Background()
}
