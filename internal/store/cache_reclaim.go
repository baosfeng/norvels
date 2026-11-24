package store

import (
	"context"
	"database/sql"
	"log"
	"sync/atomic"
	"time"

	"norvels/internal/observe"
)

// 本文件负责把 SQLite 页缓存「用过之后还回去」。
//
// 背景：PRAGMA cache_size 设的是上限，SQLite 会一路吃到上限然后一直占着——
// 进程静下来也不会自己缩。叠上 Go runtime 在 Linux 上用 MADV_FREE 惰性归还，
// 高峰吃过的页会长期留在 RSS 里；实测生产进程 HeapAlloc 只有 3.84 MB，
// RSS 却有 470 MB、外加 707 MB 被换进 swap。
//
// 归还手段只有 PRAGMA shrink_memory（判据与代价见 cache.go 的注释）：
// 实测作用域是**整进程的页缓存**，对任一连接执行一次就把 713 MB 缩到 65 MB，
// 因此这里不需要按连接逐个跑。

// reclaimRunning 去重「同一时刻只允许一个归还在跑」。
var reclaimRunning atomic.Bool

// StartCacheReclaim 启动闲置归还循环：每 cacheReclaimInterval 检查一次，
// 数据库闲置超过 cacheReclaimIdle 就归还一次页缓存。
//
// 与 StartContentRefresh 同一套生命周期口径：非阻塞、可重复调用（只有第一次生效）、
// ctx 取消即退出；被 EnvDisableBackground 关掉时整体不启动（离线 CLI 不该带后台循环）。
func (s *Store) StartCacheReclaim(ctx context.Context) {
	if backgroundDisabled() {
		return
	}
	c := &s.reconcile
	c.mu.Lock()
	if c.reclaimStarted {
		c.mu.Unlock()
		return
	}
	c.reclaimStarted = true
	stop := c.stop
	c.mu.Unlock()
	go s.runCacheReclaim(ctx, stop)
}

// runCacheReclaim 是闲置归还的循环体。
//
// stop 与内容对账共用同一个 channel：两者都由 StopContentRefresh 统一收口，
// 不额外造一套停止信号（同一个 Store 的后台能力应该只有一个开关）。
// 若内容对账没启动（stop 为 nil），select 在它上面永远不就绪，循环只由 ctx 退出——
// 这正是想要的：没有统一开关时，跟着进程生命周期走。
func (s *Store) runCacheReclaim(ctx context.Context, stop <-chan struct{}) {
	ticker := time.NewTicker(cacheReclaimInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			s.reclaimIfIdle(observe.IdleFor())
		}
	}
}

// reclaimIfIdle 在闲置够久时归还页缓存；有 SQL 在跑或刚跑过则跳过。
//
// idle 由调用方传入而不是在函数内读时钟：测试要能稳定地分别命中「闲」与「忙」
// 两条分支，不靠 sleep 去赌时间。
func (s *Store) reclaimIfIdle(idle time.Duration) {
	if idle < cacheReclaimIdle {
		return
	}
	if !reclaimRunning.CompareAndSwap(false, true) {
		return
	}
	defer reclaimRunning.Store(false)
	before, _ := observe.RSSMB()
	if err := s.ShrinkMemory(context.Background()); err != nil {
		log.Printf("归还 SQLite 页缓存失败: %v", err)
		return
	}
	after, ok := observe.RSSMB()
	if !ok || before-after < 1 {
		return // 没什么可还的，不刷日志
	}
	log.Printf("数据库闲置，已归还页缓存：RSS %.0f MB → %.0f MB", before, after)
}

// ShrinkMemory 归还本进程的 SQLite 页缓存（可重复调用，幂等）。
//
// 只传主库与正文库就够了：shrink_memory 的作用域是进程级的页缓存池，
// 对任一连接执行一次等于还掉全部（推荐库那份也一起还了，实测见 ShrinkPool）。
//
// 导出是给排障用的：本服务没有常驻的管理端点，运维在容器里也不方便直接跑
// PRAGMA，这个入口让「手动还一次」不必重启服务。
func (s *Store) ShrinkMemory(ctx context.Context) error {
	return ShrinkPool(ctx, s.db, s.cdb)
}

// ShrinkPool 对给定连接池执行一次 PRAGMA shrink_memory。
//
// database/sql 的 Exec 只借用**一条**连接，而 shrink_memory 的作用域是进程级的
// 页缓存池，所以一条就够（实测：8 个独立池全部预热后只 shrink 其中一个，
// RSS 仍从 712 MB 降到 65 MB）。
func ShrinkPool(ctx context.Context, dbs ...*sql.DB) error {
	var firstErr error
	for _, db := range dbs {
		if db == nil {
			continue
		}
		if _, err := db.ExecContext(ctx, "PRAGMA shrink_memory"); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
