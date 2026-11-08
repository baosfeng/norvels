package main

// 推荐库的打开与定期清理：与主流程解耦的辅助能力，失败只降级不阻塞。

import (
	"context"
	"log"
	"time"

	"norvels/internal/recstore"
)

// openRecStore 打开推荐库；失败按降级处理（返回 nil → 推荐端点 503）。
//
// 推荐是辅助能力：推荐库打不开（磁盘满、权限、或库版本比程序新）时服务照常启动，
// 只是 /api/events 与 /api/recommend 返回 503。不能因为一个可重建的辅助库
// 让整个阅读站起不来——那会把「发现页不可用」放大成「书也读不了」。
func openRecStore(path string) *recstore.Store {
	rec, err := recstore.Open(context.Background(), path)
	if err != nil {
		log.Printf("WARN: 推荐库打开失败（%s），/api/recommend 与 /api/events 将返回 503: %v", path, err)
		return nil
	}
	return rec
}

// setupRecommendation 打开推荐库并挂上「定期清理过期明细」的后台任务。
//
// 打开失败返回 nil（端点降级 503）而不是报错：推荐是辅助能力，
// 缺库不该阻塞启动。清理任务随 ctx 取消退出，不与 HTTP 服务生命周期耦合。
func setupRecommendation(ctx context.Context, dbPath string) *recstore.Store {
	rec := openRecStore(recstore.RecDBPath(dbPath))
	if rec == nil {
		return nil
	}
	go pruneRecStore(ctx, rec)
	return rec
}

// pruneRecommendationInterval 是推荐库历史明细的清理间隔。
//
// 每天一次而不是挂在请求上顺手清：清理是一次全表范围的条件删除，
// 挂在请求路径上等于让某个倒霉的请求替全库承担它；而每天一次已足够让
// events 稳定在保留窗口内（窗口 90 天、日增几百行，更勤的清理没有收益）。
const pruneRecommendationInterval = 24 * time.Hour

// pruneRecStore 定期清理推荐库的历史明细：启动先跑一次，之后每 24h 一次，
// ctx 取消（SIGTERM）即退出。
//
// 为什么要单独一个 goroutine：一次清理可能要几秒（几十万行），同步跑就是
// 「启动慢几秒」，而清理晚几秒没有任何副作用。
func pruneRecStore(ctx context.Context, rec *recstore.Store) {
	if rec == nil {
		return
	}
	pruneOnce(ctx, rec)
	ticker := time.NewTicker(pruneRecommendationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pruneOnce(ctx, rec)
		}
	}
}

// pruneOnce 执行一次清理，失败只记 WARN（下一个 tick 会重试）。
func pruneOnce(ctx context.Context, rec *recstore.Store) {
	n, err := rec.PruneExpired(ctx, time.Now().UTC())
	if err != nil {
		log.Printf("WARN: 推荐库历史明细清理失败（下次重试）: %v", err)
		return
	}
	if n > 0 {
		log.Printf("推荐库已清理 %d 条过期埋点明细", n)
	}
}
