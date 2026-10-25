package store

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"
)

// TestRealDBMetricsProbe 是真实库性能探针，仅在设置 NORVELS_REAL_DB 时运行。
// 输出 Stats / HealthReport 各 10 次的 P50 / P95（毫秒）。
func TestRealDBMetricsProbe(t *testing.T) {
	dbPath := os.Getenv("NORVELS_REAL_DB")
	if dbPath == "" {
		t.Skip("设置 NORVELS_REAL_DB 指向真实全量库再运行")
	}
	ctx := context.Background()
	st, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	if os.Getenv("NORVELS_PROBE_REFRESH") == "1" {
		t0 := time.Now()
		if err := st.RefreshMetrics(ctx); err != nil {
			t.Fatalf("RefreshMetrics: %v", err)
		}
		t.Logf("RefreshMetrics 耗时 %v", time.Since(t0))
	}

	statsMs := make([]float64, 0, 12)
	var last Stats
	for i := 0; i < 12; i++ {
		t0 := time.Now()
		s, err := st.Stats(ctx)
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		statsMs = append(statsMs, ms(time.Since(t0)))
		last = s
	}
	t.Logf("Stats 10 次（去掉前 2 次冷启动）: %s", summarize(statsMs[2:]))
	t.Logf("  books=%d chapters=%d text=%d index=%d other=%d db=%d estimated=%v",
		last.Books, last.Chapters, last.TextBytes, last.IndexBytes, last.OtherBytes, last.DBBytes, last.CompositionEstimated)

	healthMs := make([]float64, 0, 12)
	var lastReport HealthReport
	for i := 0; i < 12; i++ {
		t0 := time.Now()
		lastReport = st.HealthReport(ctx)
		healthMs = append(healthMs, ms(time.Since(t0)))
	}
	t.Logf("HealthReport 10 次（去掉前 2 次冷启动）: %s", summarize(healthMs[2:]))
	t.Logf("  chapterBuckets=%v", lastReport.ChapterBuckets)
	t.Logf("  lengthBuckets=%v", lastReport.ChapterLengthBuckets)
	t.Logf("  stale=%v warnings=%v", lastReport.Stale, lastReport.Warnings)

	snap, _ := st.Metrics(ctx)
	t.Logf("metrics: text=%d words=%d index=%d stale=%v", snap.TextBytes, snap.ChapterWordSum, snap.IndexBytes, snap.Stale)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// summarize 返回「P50 / P95 / 最大」的毫秒描述。
func summarize(v []float64) string {
	sorted := append([]float64(nil), v...)
	sort.Float64s(sorted)
	p50 := sorted[len(sorted)/2]
	p95 := sorted[int(float64(len(sorted)-1)*0.95)]
	max := sorted[len(sorted)-1]
	return "P50=" + ftoa(p50) + "ms P95=" + ftoa(p95) + "ms max=" + ftoa(max) + "ms"
}

// ftoa 保留两位小数的字符串化。
func ftoa(v float64) string {
	whole := int64(v)
	frac := int64((v - float64(whole)) * 100)
	return itoa(int(whole)) + "." + string(rune('0'+frac/10)) + string(rune('0'+frac%10))
}
