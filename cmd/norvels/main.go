// norvels 单二进制入口：serve / import / stats / migrate / version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"norvels/internal/api"
	"norvels/internal/config"
	"norvels/internal/store"
)

// 由 -ldflags "-X main.version=..." 注入。
var (
	version = "0.1.0-dev"
	commit  = "dev"
	builtAt = "unknown"
)

const usage = `norvels — 本地中文小说库

用法:
  norvels serve   [--addr :8080] [--db ./data/norvels.db] [--log-level info]
  norvels import  --dir <txt 目录> [--db ...] [--recursive] [--force] [--dry-run] [--workers N]
  norvels stats   [--db ...]
  norvels check-content [--db ...] [--all | --sample N]
  norvels reconcile-content [--db ...] [--fix]
  norvels backfill-fingerprints [--db ...] [--batch N]
  norvels backfill-quality [--db ...] [--batch N]
  norvels migrate [--db ...] [--dry-run] [--to N]
  norvels version

说明：v0.2 起检索范围是书名 / 作者 / 章节标题，正文不建索引，因此
      不再需要 reindex 子命令（原「重建 FTS 索引」已随正文检索一并移除）。
      老库升级时 0006 迁移会自动删掉分词镜像列与 FTS 索引，无需手工重建。

check-content 校验章节正文可读（迁移后自查用）：
  --all        全量校验所有章节（慢，迁移后建议跑一次）
  --sample N   随机抽 N 章（默认 50）
  两个参数互斥；正常时输出 OK，有坏块时列出全部损坏块并以非零码退出。

reconcile-content 对账主库与正文库（0018 起正文独立成文件）：
  报孤儿块（正文库有、主库没这本书）与悬空引用（章节指向不存在的块）；
  --fix        删除孤儿块（悬空引用只报不修：缺的正文只能从备份/重导恢复）。

backfill-fingerprints 给存量书补写章级内容指纹（升级到 0019 后跑一次）：
  0019 只建表、不回填历史数据，而导入判重只认指纹；已有指纹的书整本跳过，
  中断后重跑从断点继续。生产 79.5 GB 正文约几十分钟，--batch 控制进度打印粒度。

backfill-quality 给存量书补算质量分（升级到 0020 后跑一次）：
  0020 只加列、不回填历史数据，而版本择优要拿旧版本的质量分跟新版本比；
  已有质量分的书整本跳过，中断后重跑从断点继续。正文全空的书跳过并计数。
  噪声率按残留计（库里正文已清洗过，噪声行在入库时就丢了），口径见 docs/architecture.md。

优先级：命令行 flag > 环境变量 > 默认值。环境变量统一 NORVELS_ 前缀:
  NORVELS_ADDR         serve 监听地址（默认 :8080）
  NORVELS_DB           数据库路径（默认 ./data/norvels.db）
  NORVELS_AUTH_TOKEN   非空则启用鉴权（Bearer Token / 登录 Cookie）；空 = 开放模式
  NORVELS_LOG_LEVEL    debug|info|warn|error（默认 info）
  NORVELS_BOOKS_DIR    导入源目录（默认 /books，容器内只读挂载点）
`

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("norvels ")

	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	run, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "未知子命令 %q\n\n%s", args[0], usage)
		os.Exit(2)
	}
	if err := run(args[1:]); err != nil {
		log.Fatalf("错误: %v", err)
	}
}

// commands 是子命令分派表：名字 → 处理函数（参数是去掉子命令之后的 flag 列表）。
//
// 用表而不是 switch 是有意的：每加一个子命令就往 switch 里添一个 case，
// 圈复杂度会线性涨上去（加法本身没有分支复杂度），而这张表是平的、可数的。
var commands = map[string]func([]string) error{
	"serve":                 cmdServe,
	"import":                cmdImport,
	"stats":                 cmdStats,
	"check-content":         cmdCheckContent,
	"reconcile-content":     cmdReconcileContent,
	"backfill-fingerprints": cmdBackfillFingerprints,
	"backfill-quality":      cmdBackfillQuality,
	"migrate":               cmdMigrate,
	"version":               cmdVersion,
	"-v":                    cmdVersion,
	"--version":             cmdVersion,
	"help":                  cmdHelp,
	"-h":                    cmdHelp,
	"--help":                cmdHelp,
}

// cmdVersion 打印版本信息（构建期由 -ldflags 注入）。
func cmdVersion([]string) error {
	fmt.Printf("norvels %s (commit %s, built %s)\n", version, commit, builtAt)
	return nil
}

// cmdHelp 打印用法。
func cmdHelp([]string) error {
	fmt.Print(usage)
	return nil
}

// mustConfig 读取环境变量配置；非法配置直接终止（配置错误不该带病启动）。
func mustConfig() config.Config {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}
	return cfg
}

// openStore 打开库并执行迁移（启动即升级，失败则让进程失败）。
func openStore(ctx context.Context, path string) (*store.Store, error) {
	st, err := store.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := migrateAndLog(ctx, st); err != nil {
		st.Close()
		return nil, err
	}
	return st, nil
}

// migrateAndLog 执行迁移并打印「库版本 N → 应用 M 个迁移 → 版本 M」。
//
// 迁移失败时返回错误（调用方让启动失败）：不带着半迁移的库继续跑。
func migrateAndLog(ctx context.Context, st *store.Store) error {
	status, err := st.MigrateTo(ctx, store.LatestVersion())
	if err != nil {
		// 版本守卫的拒绝要原样透出：包装成「迁移失败」会让用户以为库坏了、
		// 去折腾迁移参数，而真实出路是升级镜像或从备份恢复。
		if errors.Is(err, store.ErrSchemaTooNew) {
			return err
		}
		return fmt.Errorf("数据库迁移失败（迁移 %d 出错，库版本停在 %d，未继续启动）: %w",
			status.FailedVersion, status.ToVersion, err)
	}
	if status.AdoptedLegacy {
		log.Printf("识别到旧版数据库（无 schema_migrations），已把版本 1 记为已应用，不重跑建表")
	}
	log.Printf("库版本 %d → 应用 %d 个迁移 → 版本 %d（打包共 %d 个）",
		status.FromVersion, len(status.Applied), status.ToVersion, status.Total)
	for _, m := range status.Applied {
		log.Printf("  已应用 %s", m.Label())
	}
	return nil
}

func cmdServe(args []string) error {
	cfg := mustConfig()
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", cfg.Addr, "监听地址")
	dbPath := fs.String("db", cfg.DB, "数据库路径")
	authToken := fs.String("auth-token", cfg.AuthToken, "鉴权令牌（空 = 开放模式）")
	logLevel := fs.String("log-level", cfg.LogLevel, "日志级别 debug|info|warn|error")
	fs.Parse(args)
	setupLogging(*logLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := openStore(context.Background(), *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	// 库级聚合值（store_metrics）缺失或口径落后时后台补算一次：不阻塞启动，
	// 算完前 /api/stats 与 /api/health/report 返回 stale 标记。
	st.OnMetricsRefreshed(func(err error) {
		if err != nil {
			log.Printf("后台重算库级聚合值失败: %v", err)
			return
		}
		log.Print("后台重算库级聚合值完成")
	})
	st.EnsureMetricsAsync(ctx)

	token := *authToken
	warnAuthMode(token == "")
	apiCfg := buildAPIConfig(cfg, ctx, token, *dbPath)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.New(st, apiCfg),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	// AI 审核后台 worker：单线程串行，ctx 取消即停。没有 key 时它照样跑，
	// 只是把待审记录直接落 needs_manual（审核页显示「AI 未启用」）。
	api.StartReviewWorker(ctx, st, apiCfg.AIJudge())
	logAIState(cfg, st.AIEnabled(context.Background()))

	errCh := make(chan error, 1)
	go func() {
		log.Printf("监听 %s，数据库 %s，鉴权 %s", *addr, st.Path(), authModeLabel(token))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Print("收到退出信号，正在关闭…")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// setupLogging 按级别设置标准库 log 的可见性（debug/info 全开，warn/error 静音 info 级输出）。
func setupLogging(level string) {
	lvl, err := config.ParseLevel(level)
	if err != nil {
		log.Printf("日志级别配置无效（%v），回落 info", err)
		lvl = slog.LevelInfo
	}
	if lvl >= slog.LevelWarn {
		log.SetOutput(levelWriter{})
	}
}

// levelWriter 在 warn/error 级别下丢弃标准库 log 的普通行。
//
// 标准库 log 没有级别概念，而项目诊断日志都用 log.Printf 输出；
// 这里用「warn+ 时整体静音 info 级输出」的保守策略，错误仍走 log.Fatalf。
type levelWriter struct{}

func (levelWriter) Write(p []byte) (int, error) { return len(p), nil }

// warnAuthMode 在开放模式下打印显著告警（公开发布前提，见 ADR-0001 §3）。
func warnAuthMode(openMode bool) {
	if !openMode {
		return
	}
	log.Printf("WARN: 未启用鉴权，请勿暴露到公网（设置 NORVELS_AUTH_TOKEN 可开启）")
}

// buildAPIConfig 组装 API 配置：版本信息、鉴权、上传目录与 AI 审核参数。
func buildAPIConfig(cfg config.Config, ctx context.Context, token, dbPath string) api.Config {
	return api.Config{
		Version: version, Commit: commit, BuiltAt: builtAt,
		AuthToken: token,
		Context:   ctx, // 进程收到 SIGTERM 时取消后台导入任务
		// 上传临时目录放数据卷同目录：容器里的 /tmp 可能是小 tmpfs，
		// 而单批上传峰值可达 256 MiB；导入结束整目录删除。
		UploadDir: filepath.Join(filepath.Dir(dbPath), "uploads"),
		// AI 审核（0021）：key 只在内存里；开关在 settings 的 ai.enabled（默认关）。
		AIAPIKey:  cfg.AIAPIKey,
		AIBaseURL: cfg.AIBaseURL,
		AIModel:   cfg.AIModel,
	}
}

// logAIState 打印 AI 审核的启动状态（key 有没有、开关开没开）。
//
// 不打印 key 本身：日志会进容器日志与工单，key 只该留在进程环境里。
func logAIState(cfg config.Config, enabled bool) {
	switch {
	case cfg.AIEnabled() && enabled:
		log.Printf("AI 审核已启用（模型 %s）", aiModelLabel(cfg.AIModel))
	case cfg.AIEnabled():
		log.Printf("AI 审核有 key 但开关未打开（settings ai.enabled=false），待审记录只落人工确认")
	default:
		log.Printf("AI 审核未配置 key（NORVELS_AI_API_KEY），待审记录只落人工确认")
	}
}

// aiModelLabel 返回日志里的模型名（未配置时说明用默认值）。
func aiModelLabel(model string) string {
	if model == "" {
		return "内置默认"
	}
	return model
}

// authModeLabel 返回日志里的鉴权模式描述。
func authModeLabel(token string) string {
	if token == "" {
		return "开放模式"
	}
	return "已启用（Bearer Token / 登录 Cookie）"
}
