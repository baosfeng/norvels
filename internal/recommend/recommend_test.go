package recommend

import (
	"math"
	"testing"
	"time"
)

func TestDefaultWeightsMatchDesign(t *testing.T) {
	want := Weights{
		InterestL1: 3.0, InterestL2: 1.5, InterestAuthor: 2.5,
		InterestTag: 2.0, InterestSeries: 1.5, Resume: 2.0, Similar: 2.5,
		Cooccur: 2.0, HotReads: 1.5, HotClicks: 0.5, Freshness: 1.0,
		ReadPenalty: 1.0, RecentShownPenalty: 0.5,
		JitterBase: 0.8, JitterSpan: 0.4, Explore: 0.6,
	}
	if got := DefaultWeights(); got != want {
		t.Fatalf("默认权重 = %+v，期望 %+v", got, want)
	}
}

func TestWeightsResolve(t *testing.T) {
	if got := (Weights{}).resolve(); got != DefaultWeights() {
		t.Fatalf("零值 Weights 应用默认权重，得到 %+v", got)
	}
	// 显式只留热度（冷启动）：不能被「逐字段补默认值」悄悄加回兴趣权重。
	custom := Weights{HotReads: 2}
	if got := custom.resolve(); got != custom {
		t.Fatalf("显式权重被改写: %+v", got)
	}
}

func TestFreshness(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	if got := Freshness(time.Time{}, now); got != 0 {
		t.Fatalf("零值入库时间新鲜度 = %v，期望 0", got)
	}
	if got := Freshness(now.Add(time.Hour), now); got != 1 {
		t.Fatalf("未来时间新鲜度 = %v，期望夹到 1", got)
	}
	if got := Freshness(now, now); got != 1 {
		t.Fatalf("刚入库新鲜度 = %v，期望 1", got)
	}
	half := Freshness(now.Add(-14*24*time.Hour), now)
	if math.Abs(half-math.Exp(-1)) > 1e-9 {
		t.Fatalf("14 天前新鲜度 = %v，期望 ≈ e⁻¹", half)
	}
	old := Freshness(now.Add(-365*24*time.Hour), now)
	if old >= 0.01 || old <= 0 {
		t.Fatalf("一年前新鲜度 = %v，期望接近 0 且为正", old)
	}
}
