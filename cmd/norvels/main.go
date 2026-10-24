// norvels 单二进制入口：serve / import / stats / reindex / migrate / version
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
  norvels reindex [--db ...]
  norvels migrate [--db ...] [--dry-run] [--to N]
  norvels version

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
	var err error
	switch args[0] {
	case "serve":
		err = cmdServe(args[1:])
	case "import":
		err = cmdImport(args[1:])
	case "stats":
		err = cmdStats(args[1:])
	case "reindex":
		err = cmdReindex(args[1:])
	case "migrate":
		err = cmdMigrate(args[1:])
	case "version", "-v", "--version":
		fmt.Printf("norvels %s (commit %s, built %s)\n", version, commit, builtAt)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令 %q\n\n%s", args[0], usage)
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("错误: %v", err)
	}
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

	token := *authToken
	warnAuthMode(token == "")
	srv := &http.Server{
		Addr: *addr,
		Handler: api.New(st, api.Config{
			Version: version, Commit: commit, BuiltAt: builtAt,
			AuthToken: token,
			Context:   ctx, // 进程收到 SIGTERM 时取消后台导入任务
		}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

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

// authModeLabel 返回日志里的鉴权模式描述。
func authModeLabel(token string) string {
	if token == "" {
		return "开放模式"
	}
	return "已启用（Bearer Token / 登录 Cookie）"
}
