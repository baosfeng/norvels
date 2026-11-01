package store

import (
	"testing"
	"time"
)

// 本文件锁死版本择优的判定规则（设计 §5）：规则表写在 versionpick.go 的文档注释里，
// 这里逐条给出「新版更优 / 新版更差 / 持平」三例与每个阈值的边界，防止口径被改而没人发现。
//
// 语料全是合成的数值，不碰真实书。用例表放在包级（数据不是逻辑，函数体才受行数门禁约束）。

// qualityOf 造一份「已计算」的质量分（章数与时间戳都填上，否则按未计算处理）。
func qualityOf(noise, garbage, structure float64) BookQuality {
	return BookQuality{
		NoiseRatio: noise, GarbageRatio: garbage, StructureScore: structure,
		DuplicateRatio: 0.05, Chapters: 20, Chars: 120000,
		ComputedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}
}

// versionCase 是一条「新旧质量分对比」的期望。
type versionCase struct {
	name    string
	oldQ    BookQuality
	newQ    BookQuality
	upgrade bool
	better  []string
	worse   []string
}

// versionCases 是「新版更优 / 新版更差 / 持平」与各项边界的完整期望表。
var versionCases = []versionCase{
	{
		name: "新版更优（噪声+乱码+结构三项）",
		oldQ: qualityOf(0.04, 0.02, 0.50), newQ: qualityOf(0.004, 0.001, 0.90),
		upgrade: true, better: []string{QualityDimNoise, QualityDimGarbage, QualityDimStructure},
	},
	{
		name: "新版更差（噪声升、乱码升）",
		oldQ: qualityOf(0.004, 0.001, 0.90), newQ: qualityOf(0.04, 0.02, 0.90),
		upgrade: false, worse: []string{QualityDimNoise, QualityDimGarbage},
	},
	{
		name: "持平（数值完全相同）",
		oldQ: qualityOf(0.02, 0.01, 0.80), newQ: qualityOf(0.02, 0.01, 0.80),
		upgrade: false,
	},
	{
		name: "恰好两项更优（噪声+乱码，结构持平）",
		oldQ: qualityOf(0.04, 0.02, 0.50), newQ: qualityOf(0.004, 0.001, 0.50),
		upgrade: true, better: []string{QualityDimNoise, QualityDimGarbage},
	},
	{
		name: "只有一项更优（噪声）",
		oldQ: qualityOf(0.04, 0.02, 0.50), newQ: qualityOf(0.004, 0.02, 0.50),
		upgrade: false, better: []string{QualityDimNoise},
	},
	{
		name: "两项更优但结构显著更差",
		oldQ: qualityOf(0.04, 0.02, 0.90), newQ: qualityOf(0.004, 0.001, 0.60),
		upgrade: false, better: []string{QualityDimNoise, QualityDimGarbage},
		worse: []string{QualityDimStructure},
	},
	{
		name: "旧值噪声为 0 时持平不算更差",
		oldQ: qualityOf(0, 0.01, 0.80), newQ: qualityOf(0, 0.01, 0.80),
		upgrade: false,
	},
	{
		name: "旧值噪声为 0 时新版本有噪声即算更差",
		oldQ: qualityOf(0, 0.01, 0.80), newQ: qualityOf(0.001, 0.001, 0.80),
		upgrade: false, better: []string{QualityDimGarbage}, worse: []string{QualityDimNoise},
	},
}

// thresholdCase 是一条「刚好达标 / 差一点」的阈值边界期望。
type thresholdCase struct {
	name   string
	dim    string
	oldQ   BookQuality
	newQ   BookQuality
	better bool
}

// thresholdCases 是三个维度各自「刚好达标」与「差一点」的边界表。
var thresholdCases = []thresholdCase{
	{name: "噪声降到正好 50%", dim: QualityDimNoise, oldQ: qualityOf(0.04, 0, 0.8), newQ: qualityOf(0.02, 0, 0.8), better: true},
	{name: "噪声只降 49%", dim: QualityDimNoise, oldQ: qualityOf(0.04, 0, 0.8), newQ: qualityOf(0.0204, 0, 0.8)},
	{name: "乱码降到正好 20%（降 80%）", dim: QualityDimGarbage, oldQ: qualityOf(0, 0.02, 0.8), newQ: qualityOf(0, 0.004, 0.8), better: true},
	{name: "乱码只降 79%", dim: QualityDimGarbage, oldQ: qualityOf(0, 0.02, 0.8), newQ: qualityOf(0, 0.0042, 0.8)},
	{name: "结构升正好 0.10", dim: QualityDimStructure, oldQ: qualityOf(0, 0, 0.5), newQ: qualityOf(0, 0, 0.6), better: true},
	{name: "结构只升 0.09", dim: QualityDimStructure, oldQ: qualityOf(0, 0, 0.5), newQ: qualityOf(0, 0, 0.59)},
}

// TestCompareVersionQualityThreeCases 三例（更优 / 更差 / 持平）与组合情形的结论。
func TestCompareVersionQualityThreeCases(t *testing.T) {
	for _, tc := range versionCases {
		t.Run(tc.name, func(t *testing.T) { assertVersionCase(t, tc) })
	}
}

// assertVersionCase 断言一条对比期望：结论 + 更优维度 + 更差维度。
func assertVersionCase(t *testing.T, tc versionCase) {
	t.Helper()
	pick := compareVersionQuality(tc.oldQ, tc.newQ)
	if pick.Upgrade != tc.upgrade {
		t.Fatalf("Upgrade = %v，期望 %v（更优 %v / 更差 %v）",
			pick.Upgrade, tc.upgrade, dimensionKeys(pick.Better), dimensionKeys(pick.Worse))
	}
	if got := dimensionKeys(pick.Better); !equalKeys(got, tc.better) {
		t.Errorf("更优维度 = %v，期望 %v", got, tc.better)
	}
	if got := dimensionKeys(pick.Worse); !equalKeys(got, tc.worse) {
		t.Errorf("更差维度 = %v，期望 %v", got, tc.worse)
	}
}

// dimensionKeys 取一组维度的键（顺序敏感：维度顺序固定为 噪声 → 乱码 → 结构）。
func dimensionKeys(dims []QualityDimension) []string {
	out := make([]string, 0, len(dims))
	for _, d := range dims {
		out = append(out, d.Key)
	}
	return out
}

// equalKeys 比较两个键列表（顺序敏感）。
func equalKeys(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestCompareVersionQualityThresholds 三个阈值各测「刚好达标」与「差一点」。
func TestCompareVersionQualityThresholds(t *testing.T) {
	for _, tc := range thresholdCases {
		t.Run(tc.name, func(t *testing.T) {
			pick := compareVersionQuality(tc.oldQ, tc.newQ)
			if hit := hasDimension(pick.Better, tc.dim); hit != tc.better {
				t.Fatalf("维度 %s 的「显著更优」= %v，期望 %v", tc.dim, hit, tc.better)
			}
		})
	}
}

// hasDimension 报告一组维度里有没有某个键。
func hasDimension(dims []QualityDimension, key string) bool {
	for _, d := range dims {
		if d.Key == key {
			return true
		}
	}
	return false
}

// TestCompareVersionQualityConservativePaths 任一侧没有质量分时走保守路径（判重复 + 说明原因）。
func TestCompareVersionQualityConservativePaths(t *testing.T) {
	computed := qualityOf(0.02, 0.01, 0.8)
	cases := []struct {
		name string
		oldQ BookQuality
		newQ BookQuality
		note string
	}{
		{name: "旧版本无质量分", oldQ: BookQuality{}, newQ: computed, note: VersionNoteOldUnknown},
		{name: "新版本无质量分", oldQ: computed, newQ: BookQuality{}, note: VersionNoteNewUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pick := compareVersionQuality(tc.oldQ, tc.newQ)
			if pick.Upgrade || pick.Note != tc.note {
				t.Fatalf("结论 = (upgrade=%v, note=%q)，期望 (false, %q)", pick.Upgrade, pick.Note, tc.note)
			}
			if len(pick.Better) != 0 || len(pick.Worse) != 0 {
				t.Errorf("保守路径不该给出任何维度结论：更优 %v / 更差 %v",
					dimensionKeys(pick.Better), dimensionKeys(pick.Worse))
			}
		})
	}
}

// TestBookQualityComputed 判定「算过没有」的两条依据都要成立。
func TestBookQualityComputed(t *testing.T) {
	cases := []struct {
		name string
		q    BookQuality
		want bool
	}{
		{name: "完整", q: qualityOf(0.02, 0.01, 0.8), want: true},
		{name: "零值", q: BookQuality{}, want: false},
		{name: "只有章数没有时间戳", q: BookQuality{Chapters: 5}, want: false},
		{name: "只有时间戳没有章数", q: BookQuality{ComputedAt: time.Now()}, want: false},
	}
	for _, tc := range cases {
		if got := tc.q.Computed(); got != tc.want {
			t.Errorf("%s: Computed() = %v，期望 %v", tc.name, got, tc.want)
		}
	}
}
