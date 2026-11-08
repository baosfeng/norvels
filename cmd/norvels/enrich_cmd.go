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
//	               [--budget 30m] [--max-requests 20000] [--limit N] [--resume] [--since RFC3339]
//	               [--min-title-sim 0.86] [--flush-every 200]
//	norvels enrich --split-long-chapters [--apply] [--min-words 30000] [--target-words 8000]
//	norvels enrich --rollback <batchID> [--apply]
//
// 默认 dry-run：不加 --apply 只打印「会改什么」，绝不写库。每一次 --apply 生成一个
// batch id 写进 enrich_changes，--rollback 按批次把旧值写回。
//
// --apply 是流式落库：每处理满 --flush-every 本（默认 200）就把这批变更记进同一个
// batch、再改进数据，并打一行进度。中途被杀只丢最后未满的一批，不会丢整批。
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
	ssince := fs.String("since", "", "只考虑 imported_at >= 该时间的书（RFC3339，空 = 不限）")
	resume := fs.Bool("resume", false, "跳过本批次已处理过的书（配合上次 --apply 的 batch）")
	resumeBatch := fs.String("resume-batch", "", "配合 --resume：上次的 batch id")
	flushEvery := fs.Int("flush-every", 200, "每处理满 N 本就把变更落库并打进度（0 = 只在结束时落库）")
	minTitleSim := fs.Float64("min-title-sim", enrich.DefaultOptions().MinTitleSim, "命中书名相似度下限")
	splitLong := fs.Bool("split-long-chapters", false, "只做超长章节切分")
	minWords := fs.Int("min-words", 30000, "超长章节阈值（字）")
	targetWords := fs.Int("target-words", 8000, "切分后每章目标字数")
	rollback := fs.String("rollback", "", "回滚指定 batch 的改动（配合 --apply 才真写）")
	fs.Parse(args)

	if err := validateEnrichFlags(*apply, *readOnly, *flushEvery); err != nil {
		return err
	}
	var sinceTime time.Time
	if *ssince != "" {
		t, err := time.Parse(time.RFC3339, *ssince)
		if err != nil {
			return fmt.Errorf("--since 解析失败（需要 RFC3339）：%w", err)
		}
		sinceTime = t
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
		apply: *apply, planOnly: *planOnly, fieldsFlag: *fields,
		workers: *workers, qps: *qps, maxSources: *maxSources, timeout: *timeout,
		budget: *budget, maxRequests: *maxRequests, limit: *limit,
		resume: *resume, resumeBatch: *resumeBatch, minTitleSim: *minTitleSim,
		flushEvery: *flushEvery, since: sinceTime,
	})
}

// validateEnrichFlags 校验互相冲突或越界的参数（不一致的组合直接报错，不静默按其一执行）。
func validateEnrichFlags(apply, readOnly bool, flushEvery int) error {
	if apply && readOnly {
		return fmt.Errorf("--apply 与 --read-only 不能同时使用")
	}
	if flushEvery < 0 {
		return fmt.Errorf("--flush-every 不能为负（0 = 只在结束时落库）")
	}
	return nil
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
	apply    bool
	planOnly bool
	// fieldsFlag 是 --fields 的原始串；fields 是解析后的集合（决策层用它过滤字段）。
	fieldsFlag  string
	fields      map[string]bool
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
	// flushEvery 是流式落库的批量：每处理满这么多本就把已收集的变更落库一次
	// （0 = 只在结束时落库，等于旧的一次性行为）。
	flushEvery int
	since      time.Time
}

// splitOpts 是切分命令的参数集合。
type splitOpts struct {
	apply       bool
	minWords    int
	targetWords int
	limit       int
}

// runEnrich 主流程：取候选 → 检索 → 决策 → （apply 时）分批落库 → 打印汇总。
func runEnrich(ctx context.Context, st *store.Store, o enrichOpts) error {
	return runEnrichTo(ctx, st, o, nil)
}

// runEnrichTo 与 runEnrich 相同，但可以注入落库/日志出口（override 非 nil 时覆盖默认 sink）。
func runEnrichTo(ctx context.Context, st *store.Store, o enrichOpts, override *streamSink) error {
	started := time.Now()
	o, cands, err := prepareEnrich(ctx, st, o)
	if err != nil {
		return err
	}
	printEnrichHeader(o, cands)
	if o.planOnly {
		printPlan(cands)
		return nil
	}
	engine, err := newEnrichEngine(ctx, st, o)
	if err != nil {
		return err
	}
	sink := enrichSink(st, o)
	if override != nil {
		sink = *override
	}
	rep := runEnrichStreamTo(ctx, st, engine, cands, o, sink)
	rep.Elapsed = time.Since(started)
	rep.BatchID = sink.BatchID
	printEnrichReport(rep, o.apply)
	// 没产生任何变更时不给批次号：那个批次在库里是空的，--rollback 只会报「没有记录」。
	if rep.BatchID == "" || rep.changeCount() == 0 {
		return nil
	}
	printFlushFailure(rep)
	fmt.Fprintf(os.Stdout, "批次号：%s（回滚用 --rollback %s --apply）\n", rep.BatchID, rep.BatchID)
	return nil
}

// enrichSink 决定这次运行怎么落库：--apply 走「同一批次号贯穿整批」的流式落库；
// dry-run 不落库也不留明细（几十万本的明细留在内存里没有消费者）。
func enrichSink(st *store.Store, o enrichOpts) streamSink {
	if !o.apply {
		return streamSink{}
	}
	batchID := newBatchID()
	return streamSink{Flush: batchFlusher(st, batchID), BatchID: batchID}
}

// batchFlusher 返回按批次落库的回调：每批都写进**同一个** batchID，
// 这样 --rollback 一次能回滚整批，而中途被杀也只丢最后未 flush 的那一批。
//
// 两阶段在这里落实：先把「要改什么」记成变更日志，记录成功后才改数据。
// 反过来（先改库、后记日志）时进程被 kill，会留下改了库却没有回滚依据的改动。
func batchFlusher(st *store.Store, batchID string) flushFunc {
	return func(ctx context.Context, changes []store.EnrichChange) (int, error) {
		if err := st.RecordEnrichChanges(ctx, batchID, changes); err != nil {
			return 0, err
		}
		return applyEnrichChanges(ctx, st, changes), nil
	}
}

// printFlushFailure 在报告之后补一行落库失败提示（失败条数为 0 时不打印）。
func printFlushFailure(rep enrichReport) {
	if rep.Failed == 0 {
		return
	}
	fmt.Fprintf(os.Stdout, "注意：%d 条变更没落库（见上方 stderr 警告），回滚只覆盖已记录的部分\n", rep.Failed)
}

// prepareEnrich 解析 --fields 并挑候选：--plan 与 --apply 都走这里，
// 保证「候选口径」与「决策/写入字段」是同一份 want。
func prepareEnrich(ctx context.Context, st *store.Store, o enrichOpts) (enrichOpts, []store.EnrichCandidate, error) {
	want, err := parseFields(o.fieldsFlag)
	if err != nil {
		return o, nil, err
	}
	o.fields = want
	cands, err := collectCandidates(ctx, st, want, o.limit, o.since)
	if err != nil {
		return o, nil, err
	}
	return o, cands, nil
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
func collectCandidates(ctx context.Context, st *store.Store, want map[string]bool, limit int, since ...time.Time) ([]store.EnrichCandidate, error) {
	var sinceTime time.Time
	if len(since) > 0 {
		sinceTime = since[0]
	}
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
		var list []store.EnrichCandidate
		var err error
		if sinceTime.IsZero() {
			list, err = st.EnrichCandidates(ctx, k, limit)
		} else {
			list, err = st.EnrichCandidatesSince(ctx, k, limit, sinceTime)
		}
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
