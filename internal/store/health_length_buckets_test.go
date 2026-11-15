package store

import (
	"context"
	"strings"
	"testing"
)

// 长度直方图（0022 预计算）的两条降级口径：
//
//   - 探测缓存说「预计算可用」但表已经读不动（运维手工删表 / 回滚迁移）时，
//     本次直方图给不出来，**必须留下告警**——静默返回空桶会让驾驶舱显示一张
//     全 0 的分布图，比直接报错更难查；
//   - 缓存失效后重新探测会老实退回实时 SQL，数字照常给出，只是慢、并标 stale。

// TestLengthBucketsWarnWhenTableGone 缓存仍标记可用、表却被删：记告警且不谎报空分布。
func TestLengthBucketsWarnWhenTableGone(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)
	st.precomputeReady.Store(true) // 进程启动时探测过，之后表被删掉了
	if _, err := st.DB().ExecContext(ctx, "DROP TABLE IF EXISTS "+tableChapterLengthHist); err != nil {
		t.Fatalf("删表: %v", err)
	}

	r := st.HealthReport(ctx)
	if r.ChapterLengthBuckets != nil {
		t.Errorf("预计算表读不动时直方图 = %v，期望 nil（不给假数据）", r.ChapterLengthBuckets)
	}
	if !hasWarning(r.Warnings, "chapterLengthBuckets") {
		t.Errorf("直方图降级必须留告警，实际 %v", r.Warnings)
	}

	// 重新探测后（与进程重启同效）退回实时 SQL：数字回来了，并标记这份是实时算的。
	st.resetPrecomputeCache()
	live := st.HealthReport(ctx)
	if !live.Stale {
		t.Error("实时算直方图时应标 stale（这次慢必须让运维看见）")
	}
	if len(live.ChapterLengthBuckets) != len(lengthBucketLabels) {
		t.Errorf("实时直方图桶数 = %d，期望 %d", len(live.ChapterLengthBuckets), len(lengthBucketLabels))
	}
}

// hasWarning 判断告警里有没有提到某个统计项（告警文案是「<名字> 统计失败：...」）。
func hasWarning(warnings []string, name string) bool {
	for _, w := range warnings {
		if strings.Contains(w, name) {
			return true
		}
	}
	return false
}
