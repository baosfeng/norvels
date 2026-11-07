package main

import (
	"context"
	"log"

	"norvels/internal/store"
)

// startBackgroundWork 启动库级后台任务：聚合值补算 + 健康计数/指纹等维护。
//
// 从 cmdServe 抽出来是为了让单函数保持在门禁 5 的 70 行内；职责也清楚：
// serve 只负责「监听与优雅退出」，这里负责「打开库之后要持续跑的活」。
func startBackgroundWork(ctx context.Context, st *store.Store) {
	// 库级聚合值（store_metrics）缺失或口径落后时后台补算一次：不阻塞启动，
	// 算完前 /api/stats 与 /api/health/report 返回 stale 标记。
	st.OnMetricsRefreshed(func(err error) {
		if err != nil {
			log.Printf("后台重算库级聚合值失败: %v", err)
			return
		}
		log.Print("后台重算库级聚合值完成")
	})
	st.EnsureMetricsAsync(ctx)
	st.StartBackgroundMaintenance(ctx)
}
