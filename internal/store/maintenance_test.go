package store

import (
	"context"
	"testing"
)

// StartBackgroundMaintenance 是 serve 的启动入口：三件「不阻塞监听」的后台任务都在这里拉起。
// 契约是「启动即返回、结果自己落库」——启动流程不等待任何一个回填。

func TestStartBackgroundMaintenanceKicksBothBackfills(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedHealthBooks(t, ctx, st)
	backfillBook(t, ctx, st, 1)
	setBookHealthComplete(t, ctx, st, 0)
	requireOK(t, "setCoverageState", st.setCoverageState(ctx, false, 0))
	t.Setenv(EnvDisableBackground, "") // 打开后台任务（TestMain 默认关掉）

	st.StartBackgroundMaintenance(ctx) // 必须立即返回，不等回填
	waitBookHealthComplete(t, ctx, st)
	waitHealthCountsIdle(t, st)
	waitCoverageBackfillIdle(t, st)
	waitRefreshIdle(t, st)

	requireEqual(t, "书级健康计数已回填", bookHealthComplete(t, ctx, st), 1)
	assertBookHealthCountsMatchLive(t, ctx, st)
	assertCoverageMatchesFull(t, ctx, st)
}
