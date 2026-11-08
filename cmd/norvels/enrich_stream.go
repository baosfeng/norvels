package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"norvels/internal/store"
)

// flushFunc 是一次落库：把一批变更记进批次日志并写进数据。
//
// 返回值是真正写进库的条数；error 表示「记 intent」就失败了（此时一条都没写，
// 调用方不能继续改数据）。部分书写失败不算 error，用 written 与 len(changes) 的差体现。
type flushFunc func(ctx context.Context, changes []store.EnrichChange) (int, error)

// streamSink 是流式循环的写侧依赖。
//
// Flush 为 nil 表示 dry-run（不落库，明细留在报告里）；Out 为 nil 表示写 os.Stdout
// （测试注入缓冲即可断言进度行，不必替换全局 stdout）。
type streamSink struct {
	Flush flushFunc
	Out   io.Writer
	// BatchID 是本次落库的批次号（同一批次贯穿整批）；dry-run 时为空。
	BatchID string
	// KeepChanges 要求把变更明细留在报告里（决策口径测试用）。
	// CLI 的 dry-run 不开：几万本的明细只为了算一个总数，没有消费者。
	KeepChanges bool
}

// sinkOf 把单个 flush 回调包成 sink（不注入 writer 时进度写 stdout）。
// 不落库时保留变更明细：这条路径只服务 dry-run 汇总与决策口径测试。
func sinkOf(flush flushFunc) streamSink {
	return streamSink{Flush: flush, KeepChanges: flush == nil}
}

// out 返回进度日志的目标 writer（默认 stdout）。
func (s streamSink) out() io.Writer {
	if s.Out == nil {
		return os.Stdout
	}
	return s.Out
}

// progressInterval 是进度日志的时间兜底：书源慢时凑满 --flush-every 本可能要几分钟，
// 到点先打一行，才看得到「进程还活着」。
const progressInterval = 30 * time.Second

// enrichStream 是流式主循环的状态，只有主协程读写它。
//
// 分工是刻意的：worker 只往 done 投结果，汇总与落库都由主协程按节流节奏发起。
// SQLite 是单写者，把写库塞进 worker 会变成 N 个协程抢同一把写锁；
// 主协程独占「收集 → 记日志 → 改数据」也让两阶段语义（先 intent 后改数据）有唯一执行点。
type enrichStream struct {
	rep    enrichReport
	engine searchEngine
	sink   streamSink
	flushN int
	// activeTotal 是本次真正要处理的本数（进度分母）；rep.Scanned 是候选数（报告口径）。
	activeTotal int
	processed   int
	pending     []store.EnrichChange
	ticker      *progressTicker
}

// newEnrichStream 初始化流式状态：scanned 是候选数（报告口径），active 是 --resume
// 之后真正要处理的本数（进度分母，否则 resume 时进度停在 0/35684 不动）。
func newEnrichStream(o enrichOpts, engine searchEngine, sink streamSink, scanned, active int) *enrichStream {
	return &enrichStream{
		rep:         enrichReport{Scanned: scanned, Skipped: map[string]int{}},
		engine:      engine,
		sink:        sink,
		flushN:      o.flushEvery,
		activeTotal: active,
		ticker:      newProgressTicker(o.flushEvery, time.Now()),
	}
}

// activeCandidates 按 --resume 的跳过集合切出本次真正要处理的书。
func activeCandidates(ctx context.Context, st *store.Store, cands []store.EnrichCandidate,
	o enrichOpts) []store.EnrichCandidate {
	skip := loadResumeSet(ctx, st, o)
	active := make([]store.EnrichCandidate, 0, len(cands))
	for _, c := range cands {
		if skip[c.BookID] {
			continue
		}
		active = append(active, c)
	}
	return active
}

// accept 收下「一本书」的结果：汇总 → 进度日志 → 按节流落库。只由主协程调用。
func (s *enrichStream) accept(ctx context.Context, out enrichOutcome) {
	s.processed++
	if out.hit {
		s.rep.HitBooks++
	}
	for _, reason := range out.skipped {
		s.rep.Skipped[reason]++
	}
	s.rep.TotalChanges += len(out.changes)
	switch {
	case s.sink.Flush != nil:
		s.pending = append(s.pending, out.changes...)
	case s.sink.KeepChanges:
		s.rep.Changes = append(s.rep.Changes, out.changes...)
	}
	// 先落库再打进度：进度行里的「已写库条数」才会包含本次 flush。
	if s.flushN > 0 && s.processed%s.flushN == 0 {
		s.flushPending(ctx)
	}
	s.reportProgress()
}

// finish 在循环收尾时补一次 flush，覆盖不足 --flush-every 本的尾巴。
func (s *enrichStream) finish(ctx context.Context) {
	s.flushPending(ctx)
	s.rep.Engine = s.engine.Stats()
}

// flushPending 把已收集的变更落库：先记 intent，记录成功才改数据。
//
// 记 intent 失败时这批变更**丢弃**并计入失败数，而不是留在 pending 里下次重试：
// 失败通常来自库被占或表不存在，重试只会让同一批变更在日志里重复失败；
// 报告里的失败条数是运维重跑的依据。
func (s *enrichStream) flushPending(ctx context.Context) {
	if s.sink.Flush == nil || len(s.pending) == 0 {
		return
	}
	batch := s.pending
	s.pending = nil
	written, err := s.sink.Flush(ctx, batch)
	s.recordFlushed(len(batch), written, err)
}

// recordFlushed 结算一次 flush：成功条数累加，差额与错误进失败数并打到 stderr。
func (s *enrichStream) recordFlushed(total, written int, err error) {
	s.rep.Written += written
	lost := total - written
	if err != nil {
		s.rep.Failed += total
		fmt.Fprintf(os.Stderr, "警告: 第 %d 本处记变更日志失败，%d 条变更未落库也未应用：%v\n",
			s.processed, total, err)
		return
	}
	if lost <= 0 {
		return
	}
	s.rep.Failed += lost
	fmt.Fprintf(os.Stderr, "警告: %d 条变更写库失败（本批已写 %d / %d 条，已记入批次日志）\n",
		lost, written, total)
}

// reportProgress 按「凑满 --flush-every 本」或「距上次超过 30 秒」取先到者打印一行进度。
func (s *enrichStream) reportProgress() {
	if !s.ticker.due(s.processed) {
		return
	}
	s.rep.Engine = s.engine.Stats()
	fmt.Fprintf(s.sink.out(), "进度：%d/%d 本 · 命中 %d 本 · 变更 %d 条 · 已写库 %d 条 · 请求 %d 次 · 用时 %s\n",
		s.processed, s.activeTotal, s.rep.HitBooks, s.rep.TotalChanges, s.rep.Written,
		s.rep.Engine.Requests, time.Since(s.ticker.start).Round(time.Second))
}

// progressTicker 是进度日志的节流器：每满 every 本，或距上次打印超过 progressInterval。
type progressTicker struct {
	every int
	last  time.Time
	start time.Time
}

// newProgressTicker 让 first 成为计时起点（首个「满 N 本」不必干等 30 秒）。
func newProgressTicker(every int, start time.Time) *progressTicker {
	return &progressTicker{every: every, last: start, start: start}
}

// due 报告这一次处理之后该不该打印进度。every<=0（只在结束落库）时只按时间节流。
func (p *progressTicker) due(processed int) bool {
	if p.every > 0 && processed%p.every == 0 {
		p.last = time.Now()
		return true
	}
	if time.Since(p.last) < progressInterval {
		return false
	}
	p.last = time.Now()
	return true
}
