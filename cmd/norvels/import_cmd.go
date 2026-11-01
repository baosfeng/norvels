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

	"norvels/internal/config"
	"norvels/internal/importer"
	"norvels/internal/parser"
	"norvels/internal/store"
)

// importFlags 是 import 子命令解析后的参数（与 flag 一一对应）。
type importFlags struct {
	dir         string
	dbPath      string
	recursive   bool
	force       bool
	dryRun      bool
	workers     int
	minChapters int
	aiOn        bool
	aiLimit     int
	aiModel     string
	aiBaseURL   string
}

// parseImportFlags 解析 flag 并做入口校验。
//
// 单独一段而不是内联在 cmdImport 里：flag 每加一个，cmdImport 就长两行，
// 而门禁对单函数 70 行是硬上限——把「读参数」和「跑流程」分开，两边都不再涨。
func parseImportFlags(args []string, cfg config.Config) (importFlags, error) {
	var o importFlags
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	fs.StringVar(&o.dir, "dir", cfg.BooksDir, "txt 所在目录，或单个 txt 文件（默认取 NORVELS_BOOKS_DIR）")
	fs.StringVar(&o.dbPath, "db", cfg.DB, "数据库路径")
	fs.BoolVar(&o.recursive, "recursive", true, "递归子目录")
	fs.BoolVar(&o.force, "force", false, "忽略幂等检查，覆盖已有记录")
	fs.BoolVar(&o.dryRun, "dry-run", false, "只解析不写库")
	fs.IntVar(&o.workers, "workers", 0, "并发解析数（默认 CPU 核数，上限 8）")
	fs.IntVar(&o.minChapters, "min-chapters", 0, "章节数低于该值跳过")
	fs.BoolVar(&o.aiOn, "ai", false, "入库前让外部 AI 逐本判定该不该收（需要 NORVELS_AI_API_KEY）")
	fs.IntVar(&o.aiLimit, "ai-limit", 200, "本次最多评审多少本（<=0 不限）")
	fs.StringVar(&o.aiModel, "ai-model", cfg.AIModel, "判定用的模型名（默认取 NORVELS_AI_MODEL）")
	fs.StringVar(&o.aiBaseURL, "ai-base-url", cfg.AIBaseURL, "OpenAI 兼容端点（默认取 NORVELS_AI_BASE_URL）")
	logLevel := fs.String("log-level", cfg.LogLevel, "日志级别 debug|info|warn|error")
	fs.Parse(args)
	setupLogging(*logLevel)
	if o.dir == "" {
		return importFlags{}, errors.New("必须指定 --dir（目录或单个 txt 文件）或设置 NORVELS_BOOKS_DIR")
	}
	return o, nil
}

func cmdImport(args []string) error {
	cfg := mustConfig()
	o, err := parseImportFlags(args, cfg)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := openStore(context.Background(), o.dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	curation, judge, err := wireCuration(ctx, o, cfg, st)
	if err != nil {
		return err
	}
	start := time.Now()
	res, err := importer.Run(ctx, st, o.importerOptions(judge), newProgressFn())
	if err != nil {
		return err
	}
	finishImport(ctx, st, o.dryRun, res, curation)
	log.Printf("完成：导入=%d 跳过=%d 失败=%d 用时=%s",
		res.Imported, res.Skipped, res.Failed, time.Since(start).Round(time.Second))
	return nil
}

// importerOptions 把 flag 摊成导入选项（判定器由调用方注入）。
func (o importFlags) importerOptions(judge importer.Judge) importer.Options {
	return importer.Options{
		Dir:         o.dir,
		Recursive:   o.recursive,
		Force:       o.force,
		DryRun:      o.dryRun,
		Workers:     o.workers,
		MinChapters: o.minChapters,
		ParserOpts:  parser.DefaultOptions(),
		Judge:       judge,
	}
}

// wireCuration 构造上架判定器；未开 --ai 时返回 (nil, nil, nil)，导入链路零成本。
func wireCuration(ctx context.Context, o importFlags, cfg config.Config,
	st *store.Store) (*aiCuration, importer.Judge, error) {
	curation, err := buildAIJudge(ctx, o.aiOn, aiConfig{
		key: cfg.AIAPIKey, baseURL: o.aiBaseURL, model: o.aiModel,
	}, st, o.aiLimit)
	if err != nil {
		return nil, nil, err
	}
	if curation == nil {
		return nil, nil, nil
	}
	return curation, curation.judge, nil
}

// finishImport 收尾：非 dry-run 时 checkpoint，然后打印升级候选与 AI 评审报告。
func finishImport(ctx context.Context, st *store.Store, dryRun bool, res *importer.Result, curation *aiCuration) {
	if !dryRun {
		if err := st.Checkpoint(ctx); err != nil {
			log.Printf("wal checkpoint 失败: %v", err)
		}
	}
	reportUpgradeCandidates(res)
	curation.report()
}

// newProgressFn 返回一个「每 5% 打一行」的进度回调。
func newProgressFn() func(importer.Progress) {
	lastPct := -1
	return func(p importer.Progress) {
		if p.Total == 0 {
			return
		}
		pct := p.Index * 100 / p.Total
		if pct != lastPct && pct%5 == 0 {
			lastPct = pct
			log.Printf("[%3d%%] %d/%d 导入=%d 跳过=%d 失败=%d | %s",
				pct, p.Index, p.Total, p.Imported, p.Skipped, p.Failed, trim(p.Path, 60))
		}
	}
}

// reportUpgradeCandidates 打印本次导入判出的「升级候选」（设计 §5）。
//
// 候选**不会**被自动替换（设计 §7）：这里只是让命令行用户看到「库外有一本更好的版本」，
// 否则「跳过=1」看上去和普通重复没有区别。结构化字段在任务文件备注与 API 里。
func reportUpgradeCandidates(res *importer.Result) {
	for _, c := range res.UpgradeCandidates {
		log.Printf("升级候选：%s → 旧书 %d《%s》命中率 %.0f%%，更优 %d 项（未替换，需人工确认）",
			c.DisplayPath, c.OldBookID, c.OldBookTitle, c.HitRate*100, len(c.Better))
	}
}
