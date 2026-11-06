package store

import "context"

// StartBackgroundMaintenance 启动全部「不阻塞监听」的后台维护任务。
//
// 两件事都曾经绑在请求路径上，在 17 万本 / 5125 万章的生产库上把 /api/health/report
// 拖到 21 秒（并发下更长）：
//   - StartContentRefresh：正文对账快照，进程起来先刷一次，之后每 30 分钟一次；
//   - EnsureFingerprintCoverage：0023 只建表，历史指纹靠这里分批回填（可中断续跑）；
//   - EnsureBookHealthCounts：0025 只建表与触发器，存量书的三项健康计数靠这里回填。
//
// 两者都是「启动即返回、失败只记日志」的后台 goroutine，调用方不需要等结果。
func (s *Store) StartBackgroundMaintenance(ctx context.Context) {
	s.StartContentRefresh(ctx)
	s.EnsureFingerprintCoverage(ctx)
	s.EnsureBookHealthCounts()
}
