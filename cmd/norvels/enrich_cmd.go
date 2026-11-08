package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"norvels/internal/enrich"
	"norvels/internal/store"
)

// cmdEnrich 按在线书源补全元数据，并把占位作者 / 未分类 / 超长章节一起收敛。
//
//	norvels enrich [--db ...] [--apply] [--plan] [--fields author,intro,category]
//	               [--workers 4] [--qps 1] [--max-sources 3] [--timeout 10s]
//	               [--budget 30m] [--max-requests 20000] [--limit N] [--resume]
//	               [--min-title-sim 0.86] [--dry-run-report FILE]
//	norvels enrich --split-long-chapters [--apply] [--min-words 30000] [--target-words 8000]
//	norvels enrich --rollback <batchID> [--apply]
//
// 默认 dry-run：不加 --apply 只打印「会改什么」，绝不写库。每一次 --apply 生成一个
// batch id 写进 enrich_changes，--rollback 按批次把旧值写回。
func cmdEnrich(args []string) error {
	cfg := mustConfig()
	fs := flag.NewFlagSet("enrich", flag.ExitOnError)
	dbPath := fs.String("db", cfg.DB, "数据库路径")
	apply := fs.Bool("apply", false, "真正写库（默认只 dry-run 打印将改什么）")
	readOnly := fs.Bool("read-only", false, "只读打开库（不迁移、不建表；在生产库上先看规模时用）")
	retry := fs.Int("retry", 6, "写库遇到 database is locked 的重试次数（服务同时在跑时用）")
	planOnly := fs.Bool("plan", false, "只列待补清单与规模，不联网")
	fields := fs.String("fields", "author,intro,category", "要补的字段（author,intro,category）")
	workers := fs.Int("workers", 4, "并发检索数")
	qps := fs.Float64("qps", 1, "单个书源每秒请求数")
	maxSources := fs.Int("max-sources", 3, "一本书最多问几个书源")
	timeout := fs.Duration("timeout", 10*time.Second, "单次请求超时")
	budget := fs.Duration("budget", 30*time.Minute, "墙钟预算（0 = 不限）")
	maxRequests := fs.Int("max-requests", 20000, "全局请求上限（0 = 不限）")
	limit := fs.Int("limit", 0, "每类最多处理多少本（0 = 不限）")
	resume := fs.Bool("resume", false, "跳过本批次已处理过的书（配合上次 --apply 的 batch）")
	resumeBatch := fs.String("resume-batch", "", "配合 --resume：上次的 batch id")
	minTitleSim := fs.Float64("min-title-sim", enrich.DefaultOptions().MinTitleSim, "命中书名相似度下限")
	splitLong := fs.Bool("split-long-chapters", false, "只做超长章节切分")
	minWords := fs.Int("min-words", 30000, "超长章节阈值（字）")
	targetWords := fs.Int("target-words", 8000, "切分后每章目标字数")
	rollback := fs.String("rollback", "", "回滚指定 batch 的改动（配合 --apply 才真写）")
	fs.Parse(args)

	if *apply && *readOnly {
		return fmt.Errorf("--apply 与 --read-only 不能同时使用")
	}
	lockRetry = *retry
	ctx := context.Background()
	st, err := openEnrichStore(ctx, *dbPath, *readOnly)
	if err != nil {
		return err
	}
	defer st.Close() //nolint:errcheck // CLI 退出路径

	if *rollback != "" {
		return runEnrichRollback(ctx, st, *rollback, *apply)
	}
	if *splitLong {
		return runSplitLongChapters(ctx, st, splitOpts{
			apply: *apply, minWords: *minWords, targetWords: *targetWords, limit: *limit,
		})
	}
	return runEnrich(ctx, st, enrichOpts{
		apply: *apply, planOnly: *planOnly, fields: *fields,
		workers: *workers, qps: *qps, maxSources: *maxSources, timeout: *timeout,
		budget: *budget, maxRequests: *maxRequests, limit: *limit,
		resume: *resume, resumeBatch: *resumeBatch, minTitleSim: *minTitleSim,
	})
}

// openEnrichStore 按需只读打开：只读模式不迁移，生产库被服务占着时也能先看规模。
func openEnrichStore(ctx context.Context, path string, readOnly bool) (*store.Store, error) {
	if readOnly {
		return store.OpenReadOnly(ctx, path)
	}
	return openStore(ctx, path)
}

// enrichOpts 是主流程的参数集合（拆一个结构体是为了让 runEnrich 少于 70 行）。
type enrichOpts struct {
	apply       bool
	planOnly    bool
	fields      string
	workers     int
	qps         float64
	maxSources  int
	timeout     time.Duration
	budget      time.Duration
	maxRequests int
	limit       int
	resume      bool
	resumeBatch string
	minTitleSim float64
}

// splitOpts 是切分命令的参数集合。
type splitOpts struct {
	apply       bool
	minWords    int
	targetWords int
	limit       int
}

// runEnrich 主流程：取候选 → 检索 → 决策 → （可选）写库 → 打印汇总。
func runEnrich(ctx context.Context, st *store.Store, o enrichOpts) error {
	started := time.Now()
	want, err := parseFields(o.fields)
	if err != nil {
		return err
	}
	cands, err := collectCandidates(ctx, st, want, o.limit)
	if err != nil {
		return err
	}
	printEnrichHeader(o, want, cands)
	if o.planOnly {
		printPlan(cands)
		return nil
	}
	engine, err := newEnrichEngine(ctx, st, o)
	if err != nil {
		return err
	}
	batchID := newBatchID()
	rep := runEnrichLoop(ctx, st, engine, cands, o)
	rep.Elapsed = time.Since(started)
	rep.BatchID = batchID
	rep.Engine = engine.Stats()
	if !o.apply || len(rep.Changes) == 0 {
		printEnrichReport(rep, o.apply)
		return nil
	}
	// 两阶段：先把「要改什么」落成变更日志，再改数据。
	// 反过来（先改库、后记日志）时进程被 kill，会留下改了库却没有回滚依据的改动。
	if err := st.RecordEnrichChanges(ctx, batchID, rep.Changes); err != nil {
		return err
	}
	rep.Written = applyEnrichChanges(ctx, st, rep.Changes)
	printEnrichReport(rep, o.apply)
	if rep.Written < len(rep.Changes) {
		fmt.Fprintf(os.Stdout, "注意：%d 条变更没写成功（见上方警告），回滚仍按整批记录的 %d 条处理\n",
			len(rep.Changes)-rep.Written, len(rep.Changes))
	}
	fmt.Fprintf(os.Stdout, "批次号：%s（回滚用 --rollback %s --apply）\n", batchID, batchID)
	return nil
}

// parseFields 把 --fields 解析成字段集合，未知字段直接报错（不静默忽略）。
func parseFields(raw string) (map[string]bool, error) {
	out := make(map[string]bool, 3)
	for _, f := range strings.Split(raw, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		switch f {
		case store.EnrichFieldAuthor, store.EnrichFieldIntro, store.EnrichFieldCategory:
			out[f] = true
		default:
			return nil, fmt.Errorf("未知字段 %q（可选 author,intro,category）", f)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--fields 不能为空")
	}
	return out, nil
}

// newBatchID 生成本次运行的批次号：时间戳 + 6 位随机后缀，便于按时间排序与辨认。
func newBatchID() string {
	buf := make([]byte, 3)
	_, _ = rand.Read(buf)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(buf)
}

// collectCandidates 把请求的字段映射成三类待补清单（跨类去重，保留首次出现的顺序）。
func collectCandidates(ctx context.Context, st *store.Store, want map[string]bool, limit int) ([]store.EnrichCandidate, error) {
	kinds := make([]string, 0, 3)
	if want[store.EnrichFieldAuthor] {
		kinds = append(kinds, store.EnrichKindPlaceholderAuthor)
	}
	if want[store.EnrichFieldIntro] {
		kinds = append(kinds, store.EnrichKindMissingIntro)
	}
	if want[store.EnrichFieldCategory] {
		kinds = append(kinds, store.EnrichKindUnclassified)
	}
	seen := make(map[int64]bool, 1024)
	out := make([]store.EnrichCandidate, 0, 1024)
	for _, k := range kinds {
		list, err := st.EnrichCandidates(ctx, k, limit)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			if seen[c.BookID] {
				continue
			}
			seen[c.BookID] = true
			out = append(out, c)
		}
	}
	return out, nil
}

// sortedKeys 返回 map 的稳定有序键（报告与日志用）。
func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
