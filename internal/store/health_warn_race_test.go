package store

import (
	"context"
	"strings"
	"testing"
)

// healthMetricNames 是 HealthReport 里 7 项并发统计的名字，与 warn() 的第一个参数一一对应。
var healthMetricNames = []string{
	"totalBooks", "thinBooks", "missingAuthor", "missingIntro",
	"hugeChapters", "chapterBuckets", "chapterLengthBuckets",
}

// 回归：并发统计同时失败时，Warnings 的追加必须是线程安全的。
//
// 现场（A 路做并发冒烟时实测）是 HealthReport 里 7 个统计 goroutine 同时写错，
// 而 warn() 里对 warnMu 做的是**懒初始化**：
//
//	if r.warnMu == nil { r.warnMu = &sync.Mutex{} }   // 多个 goroutine 各自建锁
//	r.warnMu.Lock(); r.Warnings = append(...)          // 拿着不同的锁并发 append 同一个 slice
//
// 于是既并发写 r.warnMu 字段，又用不同 mutex 保护同一份 slice——实测直接把进程带走。
// 本用例用「已取消的 ctx」让全部统计同时失败来复现，必须用 -race 跑：
//
//	go test -race -run TestHealthReportConcurrentWarnings ./internal/store/
func TestHealthReportConcurrentWarnings(t *testing.T) {
	st := newTestStore(t)
	// 7 项统计各自失败一次（ctx 已取消），一轮就能让 7 个 goroutine 同时进 warn()。
	// 多跑几轮：竞态窗口很窄，靠轮次而不是靠运气。
	for round := 0; round < 20; round++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立刻取消：所有查询都以 context canceled 失败
		r := st.HealthReport(ctx)
		assertWarnOnce(t, r.Warnings, round)
	}
}

// assertWarnOnce 断言 7 项统计各自**恰好**记了一条告警。
//
// 「恰好一次」是关键：懒初始化 + 并发 append 的另一种表现是丢条目（两个 goroutine 抢同一个
// 底层数组的 len 槽位），只看「不崩」会漏掉这种数据损坏。
func assertWarnOnce(t *testing.T, warnings []string, round int) {
	t.Helper()
	for _, name := range healthMetricNames {
		n := 0
		for _, w := range warnings {
			if strings.HasPrefix(w, name+" 统计失败") {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("第 %d 轮：%s 的告警出现 %d 次（期望 1），warnings=%v", round, name, n, warnings)
		}
	}
}
