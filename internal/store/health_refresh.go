package store

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 本文件把「正文对账」移出请求路径：HealthReport 只读最近一次对账快照，
// 过期或缺失时标 stale 并让后台 goroutine 去刷，请求侧**绝不**等全表扫描。
//
// 为什么必须移：ReconcileContent 要读正文库全部块 id 再扫主库 chapters（生产 2,469 万行），
// 冷启动第一次请求为此多付 40+ 秒——/api/health/report 生产首测 23.9 s、连测三次里一次
// 直接 60 s 超时（NAS 回环）。而前端驾驶舱的 STATS_TIMEOUT_MS = 20_000：
// 21 s 的响应在驾驶舱里**本来就是超时失败**，健康卡片等于从来没显示过。
// 移出请求路径后：请求只花一次 COUNT(*)，对账由后台按间隔（默认 30 分钟）刷新。
//
// CLI 语义不变：norvels reconcile-content / check-content 仍然同步跑、同步打印。

const (
	// EnvContentRefreshInterval 覆盖正文对账的后台刷新间隔（Go duration 文本，如 15m）。
	EnvContentRefreshInterval = "NORVELS_CONTENT_REFRESH_INTERVAL"
	// defaultContentRefreshInterval 是默认刷新间隔：孤儿块/悬空引用不是秒级问题，
	// 30 分钟足够让运维在下次打开驾驶舱时看到最新状态，又不至于让后台天天扫全表。
	defaultContentRefreshInterval = 30 * time.Minute
	// minContentRefreshInterval 挡住 0 / 负数（time.NewTicker 会 panic）。
	minContentRefreshInterval = 10 * time.Second
	// contentRefreshTimeout 是单次后台对账的上限：卡住的对账必须能被下一轮取代。
	contentRefreshTimeout = 10 * time.Minute
)

// storeBackground 是本 Store 实例上所有「后台能力」的状态：
// 正文对账刷新的快照与循环、指纹覆盖表后台回填的去重标志。
//
// 与 metricsRefreshing 同一套去重口径：每一类同一时刻只允许一个在跑（atomic CAS），
// 重复触发直接返回，不排队、不叠加 goroutine。
type storeBackground struct {
	mu sync.Mutex
	// at / rep 是最近一次**成功**对账的检查时间与结果（at 零值 = 从未对过账）。
	at  time.Time
	rep ContentReconcileReport
	// lastErr 是最近一次**尝试**的错误（成功后清空）：请求侧把它透出成 Warnings。
	lastErr error
	// interval 是快照过期阈值（循环启动时按配置定一次）。
	interval time.Duration
	// started / stop 控制后台循环只启动一次、退出时能停。
	started bool
	stop    chan struct{}
	// bg / cancel 是后台对账自己的 context：与请求、与循环生命周期都解耦，
	// Stop 时统一取消（进程退出 / 测试收尾）。
	bg     context.Context
	cancel context.CancelFunc
	// running 去重「同一时刻只允许一个对账刷新在跑」。
	running atomic.Bool
	// coverageRunning 去重「同一时刻只允许一个指纹覆盖表回填在跑」。
	// 回填可能扫 5125 万行索引，两次并发只会互相拖慢，没有任何收益。
	coverageRunning atomic.Bool
}

// reconcileCache 是 Store 字段的类型名（字段声明在 open.go）：保留别名，
// 这样这次改动完全不用碰写区外的文件。
type reconcileCache = storeBackground

// ContentRefreshInterval 返回配置的刷新间隔（环境变量优先，非法值退回默认）。
func ContentRefreshInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv(EnvContentRefreshInterval))
	if raw == "" {
		return defaultContentRefreshInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < minContentRefreshInterval {
		return defaultContentRefreshInterval
	}
	return d
}

// StartContentRefresh 启动正文对账的后台循环：**立即**刷一次，之后每 interval 刷一次。
//
// 非阻塞、可重复调用（只有第一次生效）。ctx 结束（serve 的 signal ctx）或
// StopContentRefresh / CloseContentRefresh 都会让循环退出。
func (s *Store) StartContentRefresh(ctx context.Context) { s.startContentRefresh(ctx, true) }

// startContentRefresh 启动后台循环；immediate 决定启动时是否立刻刷一次。
//
// 两条调用路径的差别是有意的：serve 显式启动时立刻刷（进程一起来就有数据），
// 而健康请求惰性启动时不刷——请求侧本来就会按 stale 触发一次，
// 再无条件刷一次等于每次冷启动都白扫一遍全表。
func (s *Store) startContentRefresh(ctx context.Context, immediate bool) {
	c := &s.reconcile
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return
	}
	c.started, c.stop = true, make(chan struct{})
	if c.interval <= 0 {
		c.interval = ContentRefreshInterval()
	}
	interval, stop := c.interval, c.stop
	c.mu.Unlock()
	go s.runContentRefresh(ctx, stop, interval, immediate)
}

// ensureContentRefreshLoop 惰性启动后台循环（健康请求第一次进来时调用）。
//
// 用独立于请求的 context：请求结束（客户端断开）不能把周期刷新一起带走。
func (s *Store) ensureContentRefreshLoop() { s.startContentRefresh(context.Background(), false) }

// runContentRefresh 是后台循环体：immediate 时先刷一次，之后按 interval 定时刷。
func (s *Store) runContentRefresh(ctx context.Context, stop <-chan struct{}, interval time.Duration, immediate bool) {
	if immediate {
		s.refreshContentAsync()
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			s.refreshContentAsync()
		}
	}
}

// StopContentRefresh 停掉后台循环并取消进行中的对账（可重复调用）。
func (s *Store) StopContentRefresh() {
	c := &s.reconcile
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if !c.started {
		return
	}
	c.started = false
	close(c.stop)
}

// CloseContentRefresh 是进程退出路径的入口（与 StopContentRefresh 等价）。
//
// 单独留一个名字：调用方（serve 收尾 / 测试 Cleanup）读起来是「关掉这个后台能力」，
// 而不是「暂停刷新」。不等待进行中的 goroutine——它拿着可被取消的 context，
// 阻塞退出比让 OS 收掉一个 goroutine 更糟。
func (s *Store) CloseContentRefresh() { s.StopContentRefresh() }

// refreshContentAsync 触发一次后台对账；已有刷新在跑时直接返回。
func (s *Store) refreshContentAsync() {
	if !s.reconcile.running.CompareAndSwap(false, true) {
		return
	}
	base := s.backgroundContext()
	go func() {
		defer s.reconcile.running.Store(false)
		rctx, cancel := context.WithTimeout(base, contentRefreshTimeout)
		defer cancel()
		// 结果由 ReconcileContent 自己记进快照（成功与失败都记）：
		// 这里只负责把「跑一次」挪到后台。
		_, _ = s.ReconcileContent(rctx, false)
	}()
}

// backgroundContext 返回后台对账用的 context（与请求生命周期无关、可被 Stop 取消）。
func (s *Store) backgroundContext() context.Context {
	c := &s.reconcile
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bg == nil {
		c.bg, c.cancel = context.WithCancel(context.Background())
	}
	return c.bg
}

// recordContentSnapshot 记下一次对账结果（成功记快照，失败只记错误）。
func (s *Store) recordContentSnapshot(rep ContentReconcileReport, err error) {
	c := &s.reconcile
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.lastErr = err
		return
	}
	c.at, c.rep, c.lastErr = rep.CheckedAt, rep, nil
}

// reconcileSnapshot 读出最近一次对账快照，并判断它是否过期。
//
// stale=true 有三种成因：从未对过账、超过刷新间隔、上次刷新失败。
// 三种都只是「数字可能旧」，不影响接口 200；过期时顺带踢一次后台刷新。
func (s *Store) reconcileSnapshot() (ContentReconcileReport, bool, error) {
	s.ensureContentRefreshLoop()
	c := &s.reconcile
	c.mu.Lock()
	at, rep, lastErr, interval := c.at, c.rep, c.lastErr, c.interval
	c.mu.Unlock()
	if interval <= 0 {
		interval = ContentRefreshInterval()
	}
	if at.IsZero() {
		s.refreshContentAsync()
		return ContentReconcileReport{}, true, lastErr
	}
	stale := lastErr != nil || nowUTC().Sub(at) >= interval
	if stale {
		s.refreshContentAsync()
	}
	return rep, stale, lastErr
}

// contentHealth 把快照翻成健康报告的 Content 段（永不阻塞、永不 500）。
func (s *Store) contentHealth(r *HealthReport) *ContentHealth {
	rep, stale, err := s.reconcileSnapshot()
	if err != nil {
		r.warn("contentReconcile", err)
	}
	h := &ContentHealth{
		OrphanBlocks: rep.OrphanBlocks, OrphanBooks: rep.OrphanBooks,
		DanglingRefs: rep.DanglingRefs, MissingTable: rep.MissingTable,
		Stale: stale,
	}
	if !rep.CheckedAt.IsZero() {
		h.CheckedAt = formatTime(rep.CheckedAt)
	}
	return h
}
