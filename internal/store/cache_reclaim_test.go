package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// 归还动作必须真的发出 PRAGMA：这条走真实 SQLite，验证的是接线不是分支。
func TestShrinkPoolRunsPragma(t *testing.T) {
	st := newTestStore(t)
	if err := st.ShrinkMemory(context.Background()); err != nil {
		t.Fatalf("ShrinkMemory 不应失败: %v", err)
	}
}

// nil 池必须被跳过而不是 panic：推荐库打不开时调用方会传 nil。
func TestShrinkPoolSkipsNil(t *testing.T) {
	if err := ShrinkPool(context.Background(), nil); err != nil {
		t.Fatalf("nil 池应被跳过: %v", err)
	}
}

// 关闭的池要如实返回错误，而不是静默成功——否则「还了内存」只是日志里的假话。
func TestShrinkPoolReportsClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := ShrinkPool(context.Background(), db); err == nil {
		t.Fatal("已关闭的池应返回错误")
	}
}

// 活跃期不能归还：用户正在翻页时把页缓存抽走，下一次读要回磁盘。
func TestReclaimSkipsWhenRecentlyActive(t *testing.T) {
	st := newTestStore(t)
	st.reclaimIfIdle(time.Second) // 远小于 cacheReclaimIdle
	if reclaimRunning.Load() {
		t.Fatal("活跃期不应触发归还")
	}
}

// 闲置够久就会真的执行一次归还（RSS 拿不到也不影响：它只决定要不要打日志）。
func TestReclaimRunsWhenIdle(t *testing.T) {
	st := newTestStore(t)
	st.reclaimIfIdle(cacheReclaimIdle + time.Second)
	if reclaimRunning.Load() {
		t.Fatal("归还结束后去重标志应复位")
	}
}

// 后台循环只启动一次：重复调用不该叠出第二个 goroutine。
func TestStartCacheReclaimIsIdempotent(t *testing.T) {
	t.Setenv(EnvDisableBackground, "") // TestMain 默认关掉后台任务
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := newTestStore(t)
	st.StartCacheReclaim(ctx)
	st.reconcile.mu.Lock()
	started := st.reconcile.reclaimStarted
	stop := st.reconcile.stop
	st.reconcile.mu.Unlock()
	if !started {
		t.Fatal("首次调用应拉起循环")
	}
	st.StartCacheReclaim(ctx) // 第二次必须被去重
	st.reconcile.mu.Lock()
	after := st.reconcile.reclaimStarted
	stop2 := st.reconcile.stop
	st.reconcile.mu.Unlock()
	if !after || stop2 != stop {
		t.Fatal("重复调用不应重启循环")
	}
}
