package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Result 是一次完整跑分的结果，也是 -out 同名 .json 的文件结构。
type Result struct {
	GeneratedAt string       `json:"generated_at"`
	DBPath      string       `json:"db_path"`
	Sample      int          `json:"sample"`
	Library     Library      `json:"library"`
	Groups      []GroupStat  `json:"groups"`
	Largest     *LargestStat `json:"largest,omitempty"`
}

// GroupStat 是一组方案的汇总（总体 + 四档分档）。
type GroupStat struct {
	Key           string    `json:"key"`
	Name          string    `json:"name"`
	Books         int       `json:"books"`
	RawBytes      int64     `json:"raw_bytes"`
	PackedBytes   int64     `json:"packed_bytes"`
	Ratio         float64   `json:"ratio"`
	WeightedRatio float64   `json:"weighted_ratio"`
	CompressMBps  float64   `json:"compress_mbps"`
	CompressNs    int64     `json:"compress_ns"`
	Ops           int       `json:"decode_ops"`
	P50Ms         float64   `json:"decode_p50_ms"`
	P95Ms         float64   `json:"decode_p95_ms"`
	Failures      int       `json:"roundtrip_failures"`
	DBBytes       int64     `json:"db_blob_bytes"`
	DBDeltaPct    float64   `json:"db_delta_pct"`
	Bins          []BinStat `json:"bins"`
}

// BinStat 是一档书大小内的汇总。
type BinStat struct {
	Label        string  `json:"label"`
	Books        int     `json:"books"`
	RawBytes     int64   `json:"raw_bytes"`
	PackedBytes  int64   `json:"packed_bytes"`
	Ratio        float64 `json:"ratio"`
	CompressMBps float64 `json:"compress_mbps"`
	P50Ms        float64 `json:"decode_p50_ms"`
	P95Ms        float64 `json:"decode_p95_ms"`
	dbBytes      int64
	compNs       int64
	ops          []int64
}

// LargestStat 是最大书在「码本 + 整本 zstd」下的整本指标。
type LargestStat struct {
	BookID          int64     `json:"book_id"`
	Chapters        int       `json:"chapters"`
	RawBytes        int64     `json:"raw_bytes"`
	PackedBytes     int64     `json:"packed_bytes"`
	DBBytes         int64     `json:"db_blob_bytes"`
	Ratio           float64   `json:"ratio"`
	EncodeMs        float64   `json:"encode_ms"`
	DecodeMs        []float64 `json:"decode_ms"`
	SteadyMs        float64   `json:"steady_ms"`
	HeapAllocPeak   uint64    `json:"heap_alloc_peak_bytes"`
	TotalAllocBytes uint64    `json:"total_alloc_bytes"`
	RSSPeakBytes    uint64    `json:"rss_peak_bytes"`
	RoundTrip       bool      `json:"round_trip_ok"`
}

// newBins 造一组带档位名的空累加器。
func newBins() []BinStat {
	bins := make([]BinStat, binsCount)
	for i := range bins {
		bins[i].Label = binLabels[i]
	}
	return bins
}

// add 把一本书的结果并进所在档。
func (b *BinStat) add(r bookResult) {
	b.Books++
	b.RawBytes += r.raw
	b.PackedBytes += r.packed
	b.dbBytes += r.dbBytes
	b.compNs += r.compNs
	b.ops = append(b.ops, r.ops...)
}

// summarize 把逐本结果汇总成一组方案的统计。
func summarize(g group, results []bookResult, lib Library) GroupStat {
	st := GroupStat{Key: g.key, Name: g.name, Bins: newBins()}
	for _, r := range results {
		if !r.ok {
			st.Failures++
			continue
		}
		st.Bins[r.bin].add(r)
	}
	st.finish(lib)
	return st
}

// finish 把逐档累加收敛成组级统计：体积、吞吐、分位延迟、加权估计。
func (s *GroupStat) finish(lib Library) {
	var ops []int64
	for i := range s.Bins {
		b := &s.Bins[i]
		b.Ratio = ratioOf(b.PackedBytes, b.RawBytes)
		b.CompressMBps = throughputMBps(b.RawBytes, b.compNs)
		sort.Slice(b.ops, func(i, j int) bool { return b.ops[i] < b.ops[j] })
		b.P50Ms = percentile(b.ops, 0.50)
		b.P95Ms = percentile(b.ops, 0.95)
		ops = append(ops, b.ops...)
		s.Books += b.Books
		s.RawBytes += b.RawBytes
		s.PackedBytes += b.PackedBytes
		s.CompressNs += b.compNs
		s.DBBytes += b.dbBytes
		s.Ops += len(b.ops)
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i] < ops[j] })
	s.Ratio = ratioOf(s.PackedBytes, s.RawBytes)
	s.CompressMBps = throughputMBps(s.RawBytes, s.CompressNs)
	s.P50Ms = percentile(ops, 0.50)
	s.P95Ms = percentile(ops, 0.95)
	s.WeightedRatio = weightedRatio(s.Bins, lib)
	if s.DBBytes > 0 {
		s.DBDeltaPct = float64(s.PackedBytes-s.DBBytes) / float64(s.DBBytes) * 100
	}
}

// ratioOf 返回「压缩后 / 原始」的百分比数值（0–100）。
func ratioOf(packed, raw int64) float64 {
	if raw <= 0 {
		return 0
	}
	return float64(packed) / float64(raw) * 100
}

// throughputMBps 返回单线程压缩吞吐（按 1 MB = 2^20 字节）。
func throughputMBps(raw int64, ns int64) float64 {
	if ns <= 0 {
		return 0
	}
	return float64(raw) / (1 << 20) / (float64(ns) / 1e9)
}

// percentile 取最近秩分位（p ∈ [0,1]），输入必须已升序，单位纳秒 → 毫秒。
func percentile(sorted []int64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	idx = min(max(idx, 0), len(sorted)-1)
	return float64(sorted[idx]) / 1e6
}

// weightedRatio 按库内各档书数占比给逐档压缩率加权，得到「全库按此方案重压」的估计。
func weightedRatio(bins []BinStat, lib Library) float64 {
	if lib.Books <= 0 || len(lib.BinBooks) < len(bins) {
		return 0
	}
	sum, share := 0.0, 0.0
	for i := range bins {
		if bins[i].Books == 0 {
			continue
		}
		w := float64(lib.BinBooks[i]) / float64(lib.Books)
		sum += w * bins[i].Ratio
		share += w
	}
	if share <= 0 {
		return 0
	}
	return sum / share
}

// writeReports 写 markdown 数字表与同名 .json 明细（两份都不入库）。
func writeReports(res *Result, outPath string) error {
	if err := os.WriteFile(outPath, []byte(markdown(res)), 0o644); err != nil {
		return fmt.Errorf("写 markdown 结果: %w", err)
	}
	blob, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化结果: %w", err)
	}
	jsonPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".json"
	if err := os.WriteFile(jsonPath, blob, 0o644); err != nil {
		return fmt.Errorf("写 JSON 结果: %w", err)
	}
	fmt.Fprintf(os.Stderr, "codebench: 结果已写入 %s 与 %s\n", outPath, jsonPath)
	return nil
}

// markdown 把结果渲染成一张可贴进回报的数字表。
func markdown(res *Result) string {
	var b strings.Builder
	lib := res.Library
	fmt.Fprintf(&b, "# codebench：真实库压缩方案实测\n\n")
	fmt.Fprintf(&b, "- 库：%s\n", res.DBPath)
	fmt.Fprintf(&b, "- 全库口径：%d 本 / %d 块 / %d 字节 UTF-8 正文 / %d 字节库内 BLOB = %.2f%%\n",
		lib.Books, lib.Blocks, lib.RawBytes, lib.PackedBytes, lib.Ratio*100)
	fmt.Fprintf(&b, "- 抽样：%d 本（四档等额、档内按 book_id 分位）\n\n", res.Sample)

	if len(res.Groups) > 0 {
		b.WriteString("## 总体\n\n")
		b.WriteString("| 方案 | 压缩率 | 加权估计 | 压缩吞吐 MB/s | 解压 P50 ms | 解压 P95 ms | 解压次数 | 往返校验 |\n")
		b.WriteString("|---|---|---|---|---|---|---|---|\n")
		for _, g := range res.Groups {
			fmt.Fprintf(&b, "| %s | %.2f%% | %.2f%% | %.2f | %.2f | %.2f | %d | %s |\n",
				g.Name, g.Ratio, g.WeightedRatio, g.CompressMBps, g.P50Ms, g.P95Ms, g.Ops,
				verdict(g.Failures))
		}
		for _, g := range res.Groups {
			if g.Key == "plain_1mb" && g.DBBytes > 0 {
				fmt.Fprintf(&b, "\n① 与库内同书 BLOB 体积对比：重压 %d 字节 vs 库内 %d 字节 = %+.2f%%（库内已是码本格式，① 只作迁移前口径参考）\n",
					g.PackedBytes, g.DBBytes, g.DBDeltaPct)
			}
		}
		b.WriteString("\n## 按书大小分档\n\n")
		b.WriteString("| 方案 | 档位 | 书数 | 压缩率 | 压缩吞吐 MB/s | 解压 P50 ms | 解压 P95 ms |\n")
		b.WriteString("|---|---|---|---|---|---|---|\n")
		for _, g := range res.Groups {
			for _, bin := range g.Bins {
				if bin.Books == 0 {
					continue
				}
				fmt.Fprintf(&b, "| %s | %s | %d | %.2f%% | %.2f | %.2f | %.2f |\n",
					g.Key, bin.Label, bin.Books, bin.Ratio, bin.CompressMBps, bin.P50Ms, bin.P95Ms)
			}
		}
	}
	if res.Largest != nil {
		b.WriteString(markdownLargest(res.Largest))
	}
	return b.String()
}

// markdownLargest 渲染最大书的整本指标段落。
func markdownLargest(st *LargestStat) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n## 最大书（book_id=%d，%d 章）在「码本 + 整本 zstd」下\n\n", st.BookID, st.Chapters)
	fmt.Fprintf(&b, "- 正文 %d → 压缩后 %d 字节 = %.2f%%（库内块合计 %d 字节）\n",
		st.RawBytes, st.PackedBytes, st.Ratio, st.DBBytes)
	fmt.Fprintf(&b, "- 整本压缩一次：%.2f ms\n", st.EncodeMs)
	fmt.Fprintf(&b, "- 连续解压 %d 次：%s\n", len(st.DecodeMs), formatMss(st.DecodeMs))
	fmt.Fprintf(&b, "- 稳态（后 %d 次均）：%.2f ms\n", max(len(st.DecodeMs)-1, 0), st.SteadyMs)
	fmt.Fprintf(&b, "- HeapAlloc 峰值（2 ms 采样）：%.1f MB；本次总分配 %.1f MB；进程 RSS 峰值 %.1f MB\n",
		float64(st.HeapAllocPeak)/(1<<20), float64(st.TotalAllocBytes)/(1<<20), float64(st.RSSPeakBytes)/(1<<20))
	fmt.Fprintf(&b, "- 往返 SHA-256：%s\n", verdictBool(st.RoundTrip))
	return b.String()
}

// formatMss 把一组毫秒值渲染成「a / b / c」。
func formatMss(values []float64) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("%.2f ms", v)
	}
	return strings.Join(parts, " / ")
}

// verdict 把失败计数换成「全部通过 / 失败 N 本」。
func verdict(failures int) string {
	if failures == 0 {
		return "全部通过"
	}
	return fmt.Sprintf("失败 %d 本", failures)
}

// verdictBool 把布尔校验结果换成中文结论。
func verdictBool(ok bool) string {
	if ok {
		return "逐字节相等，全部通过"
	}
	return "不一致"
}
