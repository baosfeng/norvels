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
	Version string // 版本号，注入 /api/health 与 /api/version
	Commit  string
	BuiltAt string
	// EnableCORS 开发期前后端分离时开启（仅放行 localhost:5173，见 middleware.go）。
	// 生产默认关：部署是同源的内嵌前端，跨域头只会多暴露信息。
	EnableCORS bool
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
	// ---- 推荐打分权重（P4-8）----
	// 三项可选覆盖，nil = 沿用 recommend.DefaultWeights() 的原值：
	// 未设置时推荐行为与引入本字段前逐位一致。非 nil 才覆盖对应维度。
	RecWeightInterestL1 *float64
	RecWeightHotReads   *float64
	RecWeightFreshness  *float64
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
	// routes 是注册时累积出的路由表，405/404 判定的唯一事实源（不要另写清单）。
	routes []route
}

// New 构造 HTTP handler（含 API 与 SPA 静态资源）。
func New(st *store.Store, cfg Config) http.Handler {
	_, h := newServer(st, cfg, webui.FS())
	return h
}

// newServer 与 New 等价，但返回 *Server 以便测试操纵内部状态与注入 FS。
//
// 路由只有一个来源：各领域的 route 构造函数返回 []route，registerRoutes 统一注册
// 并累积成 Server.routes；405/404 判定只读这张表（见 routes.go）。
// 新端点加进对应的 route 构造函数，不要在别处再手写一份清单。
func newServer(st *store.Store, cfg Config, fsys fs.FS) (*Server, http.Handler) {
	s := &Server{st: st, cfg: cfg, fsys: fsys, log: log.Default(), start: time.Now(),
		placeholder: webui.PlaceholderHTML()}
	mux := http.NewServeMux()
	s.routes = registerRoutes(mux, s)
	// 未命中任何 API 路由时统一返回 JSON 错误（否则 mux 会写纯文本）。
	mux.HandleFunc("/api", s.handleAPINotFound)
	mux.HandleFunc("/api/", s.handleAPINotFound)
	// /login、/logout 不属于 /api 前缀，兜底得单独挂：它们是页面路径，
	// 未被路由表命中时同样要按「路径存在 / 方法对不对」给 405 或 404，
	// 而不是落进 SPA fallback（否则动词用错会变成 200 的 index.html）。
	mux.HandleFunc(LoginPath, s.handleAPINotFound)
	mux.HandleFunc(logoutPath, s.handleAPINotFound)
	return s, s.wrap(mux)
}

// bookRoutes 返回「书 → 章节 → 卷 → 书签」主链路与书的写操作的路由。
func (s *Server) bookRoutes() []route {
	return []route{
		newRoute(http.MethodGet, "/api/books", s.handleListBooks),
		newRoute(http.MethodGet, "/api/books/{id}", s.handleGetBook),
		// P3 发现/管理：随机一本、批量改元信息、新建章节（字面量路由优先于 {id}）。
		newRoute(http.MethodGet, "/api/books/random", s.handleRandomBook),
		newRoute(http.MethodPost, "/api/books/batch", s.handleBatchPatchBooks),
		newRoute(http.MethodPost, "/api/books/{id}/chapters", s.handleCreateChapter),
		newRoute(http.MethodGet, "/api/books/{id}/chapters", s.handleListChapters),
		newRoute(http.MethodGet, "/api/books/{id}/chapters/{cid}", s.handleGetChapter),
		newRoute(http.MethodPatch, "/api/books/{id}", s.handlePatchBook),
		newRoute(http.MethodDelete, "/api/books/{id}", s.handleDeleteBook),
		newRoute(http.MethodGet, "/api/books/{id}/cover", s.handleGetBookCover),
		// 封面写入（管理页 M1）：上传走 multipart，删除是幂等的（本来没有也 204）。
		newRoute(http.MethodPut, "/api/books/{id}/cover", s.handlePutBookCover),
		newRoute(http.MethodDelete, "/api/books/{id}/cover", s.handleDeleteBookCover),
		// 章节结构编辑（管理页 M1）：改标题 / 拆分 / 删除都是即时生效，不进「保存」批次。
		newRoute(http.MethodPatch, "/api/books/{id}/chapters/{cid}", s.handlePatchChapter),
		newRoute(http.MethodDelete, "/api/books/{id}/chapters/{cid}", s.handleDeleteChapter),
		newRoute(http.MethodPost, "/api/books/{id}/chapters/{cid}/split", s.handleSplitChapter),
		// 正文编辑（M1）：整章替换，单事务写新块 + 重算计数 + 回收旧块。
		newRoute(http.MethodPut, "/api/books/{id}/chapters/{cid}/content", s.handlePutChapterContent),
		// 章节批量编辑（M2）：批量删除 / 合并 / 重编号。单事务，不做「部分成功」。
		newRoute(http.MethodPost, "/api/books/{id}/chapters/batch-delete", s.handleBatchDeleteChapters),
		newRoute(http.MethodPost, "/api/books/{id}/chapters/merge", s.handleMergeChapters),
		newRoute(http.MethodPost, "/api/books/{id}/chapters/renumber", s.handleRenumberChapters),
		// 卷管理（C2）：新建 / 改名 / 删除 / 重排 + 章节跨卷移动，均单事务。
		newRoute(http.MethodPost, "/api/books/{id}/volumes", s.handleCreateVolume),
		newRoute(http.MethodPatch, "/api/books/{id}/volumes/{vid}", s.handleRenameVolume),
		newRoute(http.MethodDelete, "/api/books/{id}/volumes/{vid}", s.handleDeleteVolume),
		newRoute(http.MethodPut, "/api/books/{id}/volumes/order", s.handleReorderVolumes),
		newRoute(http.MethodPost, "/api/books/{id}/chapters/move", s.handleMoveChapters),
		// 书签（P1 阅读体验 spec §2.2）：GET/POST 挂在书下，DELETE 按书签 id 直删。
		newRoute(http.MethodGet, "/api/books/{id}/bookmarks", s.handleListBookmarks),
		newRoute(http.MethodPost, "/api/books/{id}/bookmarks", s.handleCreateBookmark),
		newRoute(http.MethodDelete, "/api/bookmarks/{id}", s.handleDeleteBookmark),
		// 追更角标确认：读完最后一章后清零 new_chapters。
		newRoute(http.MethodPost, "/api/books/{id}/ack-new-chapters", s.handleAckNewChapters),
		// 我的书架（0016）：一本书只属于一个书架，加入 / 移出幂等。
		newRoute(http.MethodGet, "/api/shelf", s.handleListShelf),
		newRoute(http.MethodGet, "/api/shelf/ids", s.handleShelfBookIDs),
		newRoute(http.MethodPut, "/api/shelf/{bookId}", s.handleAddToShelf),
		newRoute(http.MethodDelete, "/api/shelf/{bookId}", s.handleRemoveFromShelf),
	}
}

// libraryRoutes 返回作者 / 系列 / 检索 / 标签 / 书单的读写端点路由。
func (s *Server) libraryRoutes() []route {
	return []route{
		newRoute(http.MethodGet, "/api/authors", s.handleListAuthors),
		newRoute(http.MethodGet, "/api/authors/{id}", s.handleGetAuthor),
		// 系列聚合（P3）：按 books.series 分组，无独立系列实体。
		newRoute(http.MethodGet, "/api/series", s.handleListSeries),
		newRoute(http.MethodGet, "/api/series/{name}", s.handleGetSeries),
		newRoute(http.MethodGet, "/api/search", s.handleSearch),
		// 分类聚合（0015）：大分类 → 小分类 → 书数，供书架筛选器与分类页使用。
		newRoute(http.MethodGet, "/api/categories", s.handleListCategories),
		newRoute(http.MethodGet, "/api/tags", s.handleListTags),
		// 标签 CRUD（P3 管理侧补全）：同名新建返回已有（201/200），删除全库解绑。
		newRoute(http.MethodPost, "/api/tags", s.handleCreateTag),
		newRoute(http.MethodPatch, "/api/tags/{id}", s.handleRenameTag),
		newRoute(http.MethodDelete, "/api/tags/{id}", s.handleDeleteTag),
		// 书单（P3 §自定义书单）：扁平收藏夹；书目全量替换，空数组即清空。
		newRoute(http.MethodGet, "/api/collections", s.handleListCollections),
		newRoute(http.MethodPost, "/api/collections", s.handleCreateCollection),
		newRoute(http.MethodPatch, "/api/collections/{id}", s.handleRenameCollection),
		newRoute(http.MethodDelete, "/api/collections/{id}", s.handleDeleteCollection),
		newRoute(http.MethodPut, "/api/collections/{id}/books", s.handleReplaceCollectionBooks),
	}
}

// importRoutes 返回导入任务、文件级终态与重试的路由。
func (s *Server) importRoutes() []route {
	return []route{
		newRoute(http.MethodPost, "/api/import", s.handleStartImport),
		newRoute(http.MethodPost, "/api/import/upload", s.handleUploadImport),
		newRoute(http.MethodGet, "/api/import/jobs", s.handleListImportJobs),
		newRoute(http.MethodGet, "/api/import/jobs/{id}", s.handleGetImportJob),
		newRoute(http.MethodPost, "/api/import/jobs/{id}/cancel", s.handleCancelImportJob),
		// P1-B 导入侧：文件级终态列表与单文件重试（spec §2.2）。
		newRoute(http.MethodGet, "/api/import/jobs/{id}/files", s.handleListImportJobFiles),
		newRoute(http.MethodPost, "/api/import/jobs/{id}/retry", s.handleRetryImportJob),
		// 来源规则包（可插拔导入 P1 切片 1）：全量替换 + 只校验不落库。
		newRoute(http.MethodGet, sourceProfilesPath, s.handleGetSourceProfiles),
		newRoute(http.MethodPut, sourceProfilesPath, s.handlePutSourceProfiles),
		newRoute(http.MethodPost, sourceProfilesPath+"/validate", s.handleValidateSourceProfiles),
		// Legado 书源导入（预览 + 应用）：粘贴 JSON → 适配成规则包。
		newRoute(http.MethodPost, sourceProfilesPath+"/import", s.handleImportLegadoSources),
		// AI 审核（0021，设计 §6 / §7）：列表 + 替换/保留 + 回滚。
		// 升级候选导入时就会落一条 pending 记录，替换必须人工确认，绝不自动替换。
		newRoute(http.MethodGet, "/api/import/reviews", s.handleListImportReviews),
		newRoute(http.MethodPost, "/api/import/reviews/{id}/decide", s.handleDecideImportReview),
		newRoute(http.MethodPost, "/api/import/reviews/{id}/rollback", s.handleRollbackImportReview),
	}
}

// miscRoutes 返回健康检查、统计、设置、阅读进度与登录的路由。
func (s *Server) miscRoutes() []route {
	return []route{
		newRoute(http.MethodGet, "/api/health", s.handleHealth),
		newRoute(http.MethodGet, "/api/stats", s.handleStats),
		// 全局设置（spec §2.2）：自定义章节识别规则 + 词表打标规则。
		newRoute(http.MethodGet, "/api/settings/chapter-rules", s.handleGetChapterRules),
		newRoute(http.MethodPut, "/api/settings/chapter-rules", s.handlePutChapterRules),
		newRoute(http.MethodGet, "/api/settings/tag-rules", s.handleGetTagRules),
		// 全局净化规则（Legado replaceRule）：读写 + 试跑（不落库）。
		newRoute(http.MethodGet, "/api/settings/replace-rules", s.handleGetReplaceRules),
		newRoute(http.MethodPut, "/api/settings/replace-rules", s.handlePutReplaceRules),
		newRoute(http.MethodPost, "/api/settings/replace-rules/preview", s.handlePreviewReplaceRules),
		newRoute(http.MethodPut, "/api/settings/tag-rules", s.handlePutTagRules),
		// 内容去重阈值（0019 起）：章节命中率 ≥ 阈值视为同一本书。
		newRoute(http.MethodGet, "/api/settings/dedupe", s.handleGetDedupeSettings),
		newRoute(http.MethodPut, "/api/settings/dedupe", s.handlePutDedupeSettings),
		// AI 审核开关（设计 §6）：开关落库（ai.enabled），key 只从环境变量进内存。
		newRoute(http.MethodGet, "/api/settings/ai", s.handleGetAISettings),
		newRoute(http.MethodPut, "/api/settings/ai", s.handlePutAISettings),
		// 能力清单（策略注册表 + 上限）：前端硬编码上限的唯一来源。
		newRoute(http.MethodGet, capabilitiesPath, s.handleCapabilities),
		newRoute(http.MethodGet, "/api/version", s.handleVersion),
		// 驾驶舱（docs/api.md §8）
		newRoute(http.MethodGet, "/api/health/report", s.handleHealthReport),
		// 健康卡下钻：某一类问题的书 / 章节明细（判定口径与报告同源）。
		newRoute(http.MethodGet, "/api/health/issues", s.handleHealthIssues),
		newRoute(http.MethodGet, "/api/stats/import-trend", s.handleImportTrend),
		newRoute(http.MethodGet, "/api/reading/progress", s.handleListProgress),
		newRoute(http.MethodPut, "/api/reading/progress", s.handleSaveProgress),
		// 阅读时长与历史（P3）：会话明细 → 统计/热力图与按书聚合的历史。
		newRoute(http.MethodPost, "/api/reading/sessions", s.handleAddReadingSession),
		newRoute(http.MethodGet, "/api/reading/history", s.handleReadingHistory),
		newRoute(http.MethodDelete, "/api/reading/history", s.handleClearReadingHistory),
		newRoute(http.MethodDelete, "/api/reading/history/{bookId}", s.handleDeleteReadingHistory),
		newRoute(http.MethodGet, "/api/stats/reading", s.handleReadingStats),
		newRoute(http.MethodGet, LoginPath, s.handleLoginPage),
		newRoute(http.MethodPost, LoginPath, s.handleLoginSubmit),
		newRoute(http.MethodPost, logoutPath, s.handleLogout),
		// 账号（0016）：只注册 + 输账号登录，无密码。注册即登录。
		newRoute(http.MethodPost, "/api/auth/register", s.handleRegister),
		newRoute(http.MethodPost, "/api/auth/login", s.handleLogin),
		newRoute(http.MethodPost, "/api/auth/logout", s.handleAuthLogout),
		newRoute(http.MethodGet, "/api/auth/me", s.handleAuthMe),
		// 登录页的「选账号」列表：免鉴权（理由见 handleListUsers 注释）。
		newRoute(http.MethodGet, "/api/auth/users", s.handleListUsers),
	}
}

// recommendRoutes 返回发现页的埋点上报与推荐流的路由。
//
// 单独一组而不是并进 miscRoutes：这两个端点依赖独立库（Config.Rec），
// 缺失时整体降级为 503，放在单独函数里让「哪些端点会受推荐库影响」一眼可见。
func (s *Server) recommendRoutes() []route {
	return []route{
		newRoute(http.MethodPost, "/api/events", s.handleRecordEvents),
		newRoute(http.MethodGet, "/api/recommend", s.handleRecommend),
		// 看板聚合（驾驶舱「推荐与埋点」区块）：只读，口径见 handlers_recommend_stats.go。
		newRoute(http.MethodGet, "/api/recommend/stats", s.handleRecommendStats),
	}
}

// baseContext 返回导入任务的父 context。
func (s *Server) baseContext() context.Context {
	if s.cfg.Context != nil {
		return s.cfg.Context
	}
	return context.Background()
}
