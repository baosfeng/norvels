// norvels 单二进制入口：import / serve / stats / reindex / version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"norvels/internal/api"
	"norvels/internal/importer"
	"norvels/internal/parser"
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
  norvels serve   [--addr :8080] [--db ./data/norvels.db]
  norvels import  --dir <txt 目录> [--db ...] [--recursive] [--force] [--dry-run] [--workers N]
  norvels stats   [--db ...]
  norvels reindex [--db ...]
  norvels version

常用环境变量:
  NORVELS_ADDR   serve 监听地址（默认 :8080）
  NORVELS_DB     数据库路径（默认 ./data/norvels.db）
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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func openStore(ctx context.Context, path string) (*store.Store, error) {
	st, err := store.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := st.Migrate(ctx); err != nil {
		st.Close()
		return nil, err
	}
	return st, nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", envOr("NORVELS_ADDR", ":8080"), "监听地址")
	dbPath := fs.String("db", envOr("NORVELS_DB", "./data/norvels.db"), "数据库路径")
	fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := openStore(context.Background(), *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := &http.Server{
		Addr: *addr,
		Handler: api.New(st, api.Config{
			Version: version, Commit: commit, BuiltAt: builtAt,
			Context: ctx, // 进程收到 SIGTERM 时取消后台导入任务
		}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("监听 %s，数据库 %s", *addr, st.Path())
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

func cmdImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	dir := fs.String("dir", "", "txt 所在目录（必填）")
	dbPath := fs.String("db", envOr("NORVELS_DB", "./data/norvels.db"), "数据库路径")
	recursive := fs.Bool("recursive", true, "递归子目录")
	force := fs.Bool("force", false, "忽略幂等检查，覆盖已有记录")
	dryRun := fs.Bool("dry-run", false, "只解析不写库")
	workers := fs.Int("workers", 0, "并发解析数（默认 CPU 核数，上限 8）")
	minChapters := fs.Int("min-chapters", 0, "章节数低于该值跳过")
	fs.Parse(args)
	if *dir == "" {
		return errors.New("必须指定 --dir")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := openStore(context.Background(), *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	start := time.Now()
	lastPct := -1
	res, err := importer.Run(ctx, st, importer.Options{
		Dir:         *dir,
		Recursive:   *recursive,
		Force:       *force,
		DryRun:      *dryRun,
		Workers:     *workers,
		MinChapters: *minChapters,
		ParserOpts:  parser.DefaultOptions(),
	}, func(p importer.Progress) {
		if p.Total == 0 {
			return
		}
		pct := p.Index * 100 / p.Total
		if pct != lastPct && pct%5 == 0 {
			lastPct = pct
			log.Printf("[%3d%%] %d/%d 导入=%d 跳过=%d 失败=%d | %s",
				pct, p.Index, p.Total, p.Imported, p.Skipped, p.Failed, trim(p.Path, 60))
		}
	})
	if err != nil {
		return err
	}
	if !*dryRun {
		if err := st.Checkpoint(context.Background()); err != nil {
			log.Printf("wal checkpoint 失败: %v", err)
		}
	}
	log.Printf("完成：导入=%d 跳过=%d 失败=%d 用时=%s",
		res.Imported, res.Skipped, res.Failed, time.Since(start).Round(time.Second))
	return nil
}

func cmdStats(args []string) error {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	dbPath := fs.String("db", envOr("NORVELS_DB", "./data/norvels.db"), "数据库路径")
	fs.Parse(args)
	ctx := context.Background()
	st, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	s, err := st.Stats(ctx)
	if err != nil {
		return err
	}
	w := os.Stdout
	fmt.Fprintf(w, "书籍: %d\n章节: %d\n作者: %d\n字数: %d\n库大小: %s\n",
		s.Books, s.Chapters, s.Authors, s.Words, humanBytes(s.DBBytes))
	if s.LastImportAt != nil {
		fmt.Fprintf(w, "最近导入: %s\n", s.LastImportAt.Format(time.RFC3339))
	}
	return nil
}

func cmdReindex(args []string) error {
	fs := flag.NewFlagSet("reindex", flag.ExitOnError)
	dbPath := fs.String("db", envOr("NORVELS_DB", "./data/norvels.db"), "数据库路径")
	fs.Parse(args)
	ctx := context.Background()
	st, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	start := time.Now()
	if err := st.Reindex(ctx); err != nil {
		return err
	}
	log.Printf("索引重建完成，用时 %s", time.Since(start).Round(time.Second))
	return nil
}

func trim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(n)/float64(div), "KMGT"[exp])
}
