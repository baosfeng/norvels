package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeResults 造三条逐本结果：两条通过（分别落 <4MB 与 4–16MB 档），一条失败。
func fakeResults() []bookResult {
	return []bookResult{
		{bookID: 11, bin: binOf(1 << 10), raw: 1 << 10, packed: 300, dbBytes: 320,
			compNs: 1e6, ops: []int64{1e6, 2e6}, ok: true},
		{bookID: 12, bin: binOf(8 << 20), raw: 8 << 20, packed: 2 << 20, dbBytes: 3 << 20,
			compNs: 2e6, ops: []int64{5e6}, ok: true},
		{bookID: 13, bin: binOf(8 << 20), ok: false},
	}
}

// wantEqual 是断言小工具：失败时带上字段名，便于从门禁日志直接定位。
func wantEqual[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %v，期望 %v", name, got, want)
	}
}

// summarizeFixture 用固定的三条结果跑一次汇总，供下面两个用例共用。
func summarizeFixture() GroupStat {
	return summarize(group{key: "k", name: "方案"}, fakeResults(), Library{Books: 10, BinBooks: []int{5, 5, 0, 0}})
}

// TestSummarizeCountsAndBins 校验分档聚合与失败计数：失败的书不进任何档。
func TestSummarizeCountsAndBins(t *testing.T) {
	st := summarizeFixture()
	wantEqual(t, "通过书数", st.Books, 2)
	wantEqual(t, "失败书数", st.Failures, 1)
	wantEqual(t, "原始字节", st.RawBytes, int64((1<<10)+(8<<20)))
	wantEqual(t, "解压次数", st.Ops, 3)
	wantEqual(t, "小档书数", st.Bins[binSmall].Books, 1)
	wantEqual(t, "中档书数", st.Bins[binMid].Books, 1)
	wantEqual(t, "大档书数", st.Bins[binHuge].Books, 0)
}

// TestSummarizeLatencyAndRatio 校验分位时延、加权估计与库内体积偏差。
func TestSummarizeLatencyAndRatio(t *testing.T) {
	st := summarizeFixture()
	wantEqual(t, "P50 毫秒", st.P50Ms, 2.0) // 1 / 2 / 5 ms 的中位数
	wantEqual(t, "P95 毫秒", st.P95Ms, 5.0)
	if st.WeightedRatio <= 0 || st.WeightedRatio > 100 {
		t.Fatalf("加权估计应在 0–100 之间，得到 %v", st.WeightedRatio)
	}
	if st.DBDeltaPct >= 0 {
		t.Fatalf("重压体积小于库内 BLOB，偏差应为负，得到 %v", st.DBDeltaPct)
	}
}

// TestPercentileClamps 校验最近秩分位的边界（空输入 / p=0 / 超大 p）。
func TestPercentileClamps(t *testing.T) {
	if got := percentile(nil, 0.5); got != 0 {
		t.Fatalf("空切片应返回 0，得到 %v", got)
	}
	sorted := []int64{1e6, 2e6, 3e6}
	if got := percentile(sorted, 0); got != 1 {
		t.Fatalf("p=0 应取最小，得到 %v", got)
	}
	if got := percentile(sorted, 1); got != 3 {
		t.Fatalf("p=1 应取最大，得到 %v", got)
	}
}

// TestRatioAndThroughputGuards 校验除零口径：无原文返回 0，无耗时返回 0。
func TestRatioAndThroughputGuards(t *testing.T) {
	if got := ratioOf(10, 0); got != 0 {
		t.Fatalf("无原文时压缩率应为 0，得到 %v", got)
	}
	if got := throughputMBps(1<<20, 0); got != 0 {
		t.Fatalf("无耗时时吞吐应为 0，得到 %v", got)
	}
}

// TestWeightedRatioGuards 校验加权估计的三条兜底：库为空、分档统计缺失、样本覆盖不到任何档。
func TestWeightedRatioGuards(t *testing.T) {
	bins := newBins()
	if got := weightedRatio(bins, Library{Books: 0}); got != 0 {
		t.Fatalf("库为空应返回 0，得到 %v", got)
	}
	if got := weightedRatio(bins, Library{Books: 10}); got != 0 {
		t.Fatalf("BinBooks 缺失应返回 0，得到 %v", got)
	}
	lib := Library{Books: 10, BinBooks: []int{10, 0, 0, 0}}
	if got := weightedRatio(bins, lib); got != 0 {
		t.Fatalf("样本没覆盖任何档应返回 0，得到 %v", got)
	}
}

// TestMarkdownAndLargest 校验数字表渲染：表头、分档行、失败提示与最大书段落都在。
func TestMarkdownAndLargest(t *testing.T) {
	lib := Library{Books: 10, Blocks: 20, RawBytes: 1000, PackedBytes: 340, Ratio: 34, BinBooks: []int{5, 5, 0, 0}}
	res := &Result{
		DBPath:  "/tmp/x.db",
		Sample:  2,
		Library: lib,
		Groups:  []GroupStat{summarize(groups[0], fakeResults(), lib)},
		Largest: &LargestStat{BookID: 9, Chapters: 3, RawBytes: 100, PackedBytes: 30,
			Ratio: 30, EncodeMs: 1.5, DecodeMs: []float64{1, 2}, SteadyMs: 2,
			HeapAllocPeak: 1 << 20, RoundTrip: true},
	}
	md := markdown(res)
	for _, want := range []string{"## 总体", "## 按书大小分档", "失败 1 本", "book_id=9", "HeapAlloc 峰值", "逐字节相等"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown 缺少 %q：\n%s", want, md)
		}
	}
}

// TestWriteReports 校验两份产物真的落盘且 JSON 可解析（这是 leader 取数的唯一入口）。
func TestWriteReports(t *testing.T) {
	out := filepath.Join(t.TempDir(), "cb.md")
	res := &Result{DBPath: "/tmp/x.db", Sample: 0, Library: Library{Books: 1, BinBooks: []int{1, 0, 0, 0}}}
	if err := writeReports(res, out); err != nil {
		t.Fatalf("写报告失败: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("markdown 未落盘: %v", err)
	}
	blob, err := os.ReadFile(strings.TrimSuffix(out, ".md") + ".json")
	if err != nil {
		t.Fatalf("JSON 未落盘: %v", err)
	}
	var back Result
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("JSON 不可解析: %v", err)
	}
	if back.Library.Books != 1 {
		t.Fatalf("JSON 回读内容错误: %+v", back.Library)
	}
}

// TestVerdictAndFormat 校验结论串与毫秒列表的渲染。
func TestVerdictAndFormat(t *testing.T) {
	if got := verdict(0); got != "全部通过" {
		t.Fatalf("零失败应写「全部通过」，得到 %q", got)
	}
	if got := verdict(3); got != "失败 3 本" {
		t.Fatalf("失败计数渲染错误: %q", got)
	}
	if got := verdictBool(false); got != "不一致" {
		t.Fatalf("校验失败应写「不一致」，得到 %q", got)
	}
	if got := formatMss([]float64{1, 2}); got != "1.00 ms / 2.00 ms" {
		t.Fatalf("毫秒列表渲染错误: %q", got)
	}
}

// TestNewBinsLabels 校验档位名与下标一一对应（表格行靠这个对号入座）。
func TestNewBinsLabels(t *testing.T) {
	bins := newBins()
	if len(bins) != binsCount {
		t.Fatalf("档位数 %d，期望 %d", len(bins), binsCount)
	}
	for i := range bins {
		if bins[i].Label != binLabels[i] {
			t.Fatalf("第 %d 档名为 %q，期望 %q（表格行靠下标对号入座）", i, bins[i].Label, binLabels[i])
		}
	}
}
