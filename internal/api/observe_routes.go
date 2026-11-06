package api

import (
	"net/http"

	"norvels/internal/observe"
)

// applyObserveEnv 在 server 装配时读一次观测开关（见 internal/observe.ConfigureFromEnv）：
// NORVELS_METRICS=off 关掉全部采集（Observe* 变成空操作），
// NORVELS_SLOW_MS / NORVELS_SQL_SLOW_MS 覆盖默认阈值（500 / 100 毫秒）。
func applyObserveEnv() { observe.ConfigureFromEnv() }

// handleMetrics 返回本刻的性能快照（GET /api/metrics）。
//
// 鉴权沿用 authMiddleware 的默认策略：它不在 authPublicPath 里，启用口令后同样要求登录。
// 响应含真实请求路径与 SQL 模板，是运维诊断面，不要加成免鉴权路径。
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, observe.SnapshotNow())
}
