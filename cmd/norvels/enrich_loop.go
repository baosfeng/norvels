package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"norvels/internal/crawl"
	"norvels/internal/enrich"
	"norvels/internal/store"
)

// enrichReport 是一次补全运行的汇总（打印 + 落库都用它）。
type enrichReport struct {
	BatchID  string
	Scanned  int
	HitBooks int
	Written  int
	Changes  []store.EnrichChange
	Skipped  map[string]int
	Engine   crawl.Stats
	Elapsed  time.Duration
}

// newEnrichEngine 读库里的在线书源并构造检索执行器。
//
// 没有在线书源不算错误：本地词表分类完全离线可用，而 CLI 会在调用处打印提示，
// 让运维知道「这次只能补分类」。硬报错会把一条能出结果的路径（分类）也堵死。
func newEnrichEngine(ctx context.Context, st *store.Store, o enrichOpts) (*crawl.Engine, error) {
	profiles, err := st.ListSourceProfiles(ctx)
	if err != nil {
		return nil, err
	}
	engine, err := crawl.NewEngine(profiles, crawl.Options{
		Workers: o.workers, QPS: o.qps, Timeout: o.timeout,
		MaxSources: o.maxSources, MaxRequests: o.maxRequests, Budget: o.budget,
		MinTitleSim: o.minTitleSim,
	})
	if err != nil {
		return nil, err
	}
	return engine, nil
}

// searchEngine 是 enrich 循环需要的检索能力：*crawl.Engine 实现它，
// 测试可以注入固定命中的假引擎（不联网也能验证决策与变更口径）。
type searchEngine interface {
	SearchBook(ctx context.Context, title string) (crawl.Result, error)
	Stats() crawl.Stats
}

// enrichDecisionOptions 把运行参数映射成决策参数：--fields 在这里进入决策层，
// 只有选中的字段会被判断，未选中的字段既不产生变更、也不产生跳过原因。
func enrichDecisionOptions(o enrichOpts) enrich.Options {
	opt := enrich.DefaultOptions()
	opt.MinTitleSim = o.minTitleSim
	opt.Fields = o.fields
	return opt
}

// runEnrichLoop 用 worker 池并发检索并逐本决策（写库在调用方，按批记录）。
func runEnrichLoop(ctx context.Context, st *store.Store, engine searchEngine,
	cands []store.EnrichCandidate, o enrichOpts) enrichReport {
	rep := enrichReport{Scanned: len(cands), Skipped: map[string]int{}}
	opt := enrichDecisionOptions(o)
	skip := loadResumeSet(ctx, st, o)
	done := make(chan enrichOutcome, o.workers)
	sem := make(chan struct{}, maxInt(o.workers, 1))
	var wg sync.WaitGroup
	// collectDone 保证「汇总真的跑完」才返回：只 close(done) 就返回的话，
	// 主协程可能读到还没被消费的报告（并发写同一个 rep 更是数据竞争）。
	collectDone := make(chan struct{})
	go func() {
		collectOutcomes(done, &rep)
		close(collectDone)
	}()
	for _, c := range cands {
		if skip[c.BookID] {
			continue
		}
		wg.Add(1)
		go func(c store.EnrichCandidate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			done <- enrichOne(ctx, st, engine, c, opt)
		}(c)
	}
	wg.Wait()
	close(done)
	<-collectDone
	return rep
}

// enrichOutcome 是一本书的处理结果（并发安全的回收单位）。
type enrichOutcome struct {
	hit     bool
	changes []store.EnrichChange
	skipped []string
}

// collectOutcomes 把并发结果汇总进报告（唯一写 rep 的 goroutine）。
func collectOutcomes(done <-chan enrichOutcome, rep *enrichReport) {
	for out := range done {
		if out.hit {
			rep.HitBooks++
		}
		rep.Changes = append(rep.Changes, out.changes...)
		for _, s := range out.skipped {
			rep.Skipped[s]++
		}
	}
}

// loadResumeSet 读上次批次处理过的 book_id（--resume 时跳过它们）。
func loadResumeSet(ctx context.Context, st *store.Store, o enrichOpts) map[int64]bool {
	set := map[int64]bool{}
	if !o.resume || o.resumeBatch == "" {
		return set
	}
	changes, err := st.EnrichBatchChanges(ctx, o.resumeBatch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 读取 resume 批次失败，本次不跳过任何书：%v\n", err)
		return set
	}
	for _, c := range changes {
		set[c.BookID] = true
	}
	return set
}

// enrichOne 处理一本书：检索 → 决策 →（apply 时）写库。
func enrichOne(ctx context.Context, st *store.Store, engine searchEngine,
	c store.EnrichCandidate, opt enrich.Options) enrichOutcome {
	res, err := engine.SearchBook(ctx, c.Title)
	if err != nil && !errors.Is(err, crawl.ErrNoSource) {
		return enrichOutcome{skipped: []string{"crawl_error"}}
	}
	dec := enrich.Decide(subjectOf(c), res.Hits, opt)
	out := enrichOutcome{hit: len(res.Hits) > 0, skipped: dec.Skipped}
	out.changes = decideChanges(c, dec)
	return out
}

// subjectOf 把库内候选转成决策输入。
func subjectOf(c store.EnrichCandidate) enrich.Subject {
	return enrich.Subject{Title: c.Title, Author: c.Author, Intro: c.Intro,
		CategoryL1: c.CategoryL1, CategoryL2: c.CategoryL2}
}

// maxInt 取较大值。
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// sortedSkipKeys 稳定排序跳过原因（报告可对照）。
func sortedSkipKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
