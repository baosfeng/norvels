package store

import (
	"context"
	"testing"
	"time"
)

// 本文件锁死「正文对账移出请求路径」的三条性质：
//
//	1. 请求只读快照：还没有任何快照时也要立刻返回（标 stale），绝不等全表扫描；
//	2. 快照过期仍给出上次的数字（标 stale），后台自己补；
//	3. 后台循环只启动一次、退出能停（atomic 去重，与 metricsRefreshing 同一口径）。
//
// 为什么值得单独测：ReconcileContent 在生产要扫 2,469 万行章节，一旦它回到请求路径，
// /api/health/report 就会重新变成 20 秒以上——而驾驶舱的 STATS_TIMEOUT_MS = 20_000，
// 那就是「健康卡片永远超时」。

// TestContentSnapshotStaleUntilFirstRefresh 首次请求不等对账：立刻返回并标 stale。
func TestContentSnapshotStaleUntilFirstRefresh(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	saveOrFail(t, ctx, st, syntheticBook("体检之书", "/books/probe.txt",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		syntheticChapters(2)))

	first := st.HealthReport(ctx).Content
	if first == nil {
		t.Fatal("健康报告缺少 content 段")
	}
	if !first.Stale || first.CheckedAt != "" {
		t.Fatalf("尚无快照时应 stale 且不带 CheckedAt：%+v", first)
	}

	// 同步跑一次对账（CLI 语义）：快照立刻可见。
	if _, err := st.ReconcileContent(ctx, false); err != nil {
		t.Fatalf("ReconcileContent: %v", err)
	}
	fresh := st.HealthReport(ctx).Content
	if fresh.Stale || fresh.CheckedAt == "" {
		t.Fatalf("对账之后应给出新鲜快照：%+v", fresh)
	}
	if again := st.HealthReport(ctx).Content; again.CheckedAt != fresh.CheckedAt {
		t.Fatalf("两次请求应命中同一份快照：%q vs %q", again.CheckedAt, fresh.CheckedAt)
	}
}

// TestContentSnapshotNotRescanned 请求绝不重跑对账：库变了但快照没到期时数字不动。
func TestContentSnapshotNotRescanned(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	saveOrFail(t, ctx, st, syntheticBook("快照之书", "/books/snap.txt",
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		syntheticChapters(2)))
	if _, err := st.ReconcileContent(ctx, false); err != nil {
		t.Fatalf("ReconcileContent: %v", err)
	}
	waitRefreshIdle(t, st)
	before := st.HealthReport(ctx).Content

	// 把正文块全删掉：真去对账会立刻数出悬空引用（章节指着不存在的块）。
	if _, err := st.cdb.ExecContext(ctx, "DELETE FROM chapter_blocks"); err != nil {
		t.Fatalf("删块: %v", err)
	}
	after := st.HealthReport(ctx).Content
	if after.DanglingRefs != before.DanglingRefs || after.CheckedAt != before.CheckedAt {
		t.Fatalf("请求路径重跑了对账：%+v vs %+v", after, before)
	}
	if live, err := st.reconcileContent(ctx, false); err != nil || live.DanglingRefs == 0 {
		t.Fatalf("直接对账应看到悬空引用：%+v err=%v", live, err)
	}
}

// TestContentSnapshotExpiresKeepsNumbers 过期不改数字，只标 stale（并触发后台补）。
func TestContentSnapshotExpiresKeepsNumbers(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.ReconcileContent(ctx, false); err != nil {
		t.Fatalf("ReconcileContent: %v", err)
	}
	waitRefreshIdle(t, st)
	fresh := st.HealthReport(ctx).Content
	// 把快照时间往回拨两个间隔：模拟「进程跑了很久，快照早就该刷了」。
	st.reconcile.mu.Lock()
	st.reconcile.at = nowUTC().Add(-2 * ContentRefreshInterval())
	st.reconcile.mu.Unlock()

	stale := st.HealthReport(ctx).Content
	if !stale.Stale {
		t.Fatalf("过期快照必须标 stale：%+v", stale)
	}
	if stale.CheckedAt != fresh.CheckedAt || stale.OrphanBlocks != fresh.OrphanBlocks {
		t.Fatalf("过期时应保留上次数字：%+v vs %+v", stale, fresh)
	}
}

// TestContentRefreshLoopStopsOnClose 后台循环只启动一次，Close 之后不再刷新。
func TestContentRefreshLoopStopsOnClose(t *testing.T) {
	t.Setenv(EnvDisableBackground, "") // 这个用例本身要验证后台循环，单独打开
	ctx := context.Background()
	st := newTestStore(t)
	st.reconcile.mu.Lock()
	st.reconcile.interval = 5 * time.Millisecond // 测试里把 30 分钟缩成 5 毫秒
	st.reconcile.mu.Unlock()

	st.StartContentRefresh(ctx)
	waitSnapshot(t, st)
	st.StartContentRefresh(ctx) // 幂等：只有第一次生效
	st.CloseContentRefresh()
	waitRefreshIdle(t, st)

	stopped := snapshotAt(st)
	time.Sleep(40 * time.Millisecond)
	if snapshotAt(st) != stopped {
		t.Error("Close 之后仍在后台刷新")
	}
}

// TestContentRefreshIntervalConfig 间隔可配：非法值 / 过小值退回默认。
func TestContentRefreshIntervalConfig(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"", defaultContentRefreshInterval},
		{"15m", 15 * time.Minute},
		{"bogus", defaultContentRefreshInterval},
		{"1s", defaultContentRefreshInterval},
		{"-5m", defaultContentRefreshInterval},
	}
	for _, c := range cases {
		t.Setenv(EnvContentRefreshInterval, c.raw)
		if got := ContentRefreshInterval(); got != c.want {
			t.Errorf("ContentRefreshInterval(%q) = %v，期望 %v", c.raw, got, c.want)
		}
	}
}

// waitSnapshot 等到第一份快照落库（后台首次刷新完成）。
func waitSnapshot(t *testing.T, st *Store) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !snapshotAt(st).IsZero() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("3 秒内后台没有产出任何对账快照")
}

// waitRefreshIdle 等到没有刷新在跑（Close 之后用来消除在途 goroutine 的竞态）。
func waitRefreshIdle(t *testing.T, st *Store) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !st.reconcile.running.Load() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("3 秒内后台刷新没有停下来")
}

// snapshotAt 读出快照时间（测试观测点）。
func snapshotAt(st *Store) time.Time {
	st.reconcile.mu.Lock()
	defer st.reconcile.mu.Unlock()
	return st.reconcile.at
}
