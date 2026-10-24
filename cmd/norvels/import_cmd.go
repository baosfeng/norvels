package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"norvels/internal/importer"
	"norvels/internal/parser"
)

func cmdImport(args []string) error {
	cfg := mustConfig()
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	dir := fs.String("dir", cfg.BooksDir, "txt 所在目录（默认取 NORVELS_BOOKS_DIR）")
	dbPath := fs.String("db", cfg.DB, "数据库路径")
	recursive := fs.Bool("recursive", true, "递归子目录")
	force := fs.Bool("force", false, "忽略幂等检查，覆盖已有记录")
	dryRun := fs.Bool("dry-run", false, "只解析不写库")
	workers := fs.Int("workers", 0, "并发解析数（默认 CPU 核数，上限 8）")
	minChapters := fs.Int("min-chapters", 0, "章节数低于该值跳过")
	logLevel := fs.String("log-level", cfg.LogLevel, "日志级别 debug|info|warn|error")
	fs.Parse(args)
	setupLogging(*logLevel)
	if *dir == "" {
		return errors.New("必须指定 --dir 或设置 NORVELS_BOOKS_DIR")
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
