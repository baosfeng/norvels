package parser

import (
	"math"
	"sort"
)

// countSanity 评估「章节数是否落在合理区间」，返回 0~1 的分数。
//
// 依据：中文网络小说的单章长度中位数大约在 1500~8000 字之间。由此反推，
// 一本书合理的章数区间是 [总字数/8000, 总字数/1500]。落在区间内即满分；
// 偏少按比例衰减（越少越可疑），偏多同样衰减（可能是把正文切成碎块）。
//
// 这比旧口径「平均章字数 > 30000 就整体回退」精确得多：15 章 / 4.5 万字的书
// lo=5.6 hi=30，n=15 落在区间内拿满分，本来就不该被回退。
func countSanity(chapters int, totalChars int) float64 {
	if chapters <= 0 || totalChars <= 0 {
		return 0
	}
	lo := float64(totalChars) / 8000.0
	hi := float64(totalChars) / 1500.0
	n := float64(chapters)
	switch {
	case n < lo:
		if lo <= 0 {
			return 1
		}
		return n / lo
	case n > hi:
		if n <= 0 {
			return 0
		}
		return hi / n
	}
	return 1.0
}

// outlierThreshold 是判断「离群章」的两个阈值：
//
//	绝对阈值：超过 maxOutlierChars 个字符一定是切分失败留下的巨块；
//	统计阈值：超过 mean+2σ 且超过 max(2*median, minOutlierChars) 才算离群——
//	单靠统计阈值会在「章长本来就不均匀」的书里误伤。
const (
	maxOutlierChars = 50000
	minOutlierChars = 12000
)

// findOutliers 返回内容明显偏长的章节下标。
func findOutliers(chapters []string) []int {
	if len(chapters) == 0 {
		return nil
	}
	lens := make([]int, len(chapters))
	total := 0
	for i, c := range chapters {
		lens[i] = runeLen(c)
		total += lens[i]
	}
	mean := float64(total) / float64(len(chapters))
	variance := 0.0
	for _, n := range lens {
		d := float64(n) - mean
		variance += d * d
	}
	sd := math.Sqrt(variance / float64(len(chapters)))
	median := medianOf(lens)
	statLimit := mean + 2*sd
	hardLimit := math.Max(float64(2*median), minOutlierChars)

	var out []int
	for i, n := range lens {
		fn := float64(n)
		if fn > maxOutlierChars || (fn > statLimit && fn > hardLimit) {
			out = append(out, i)
		}
	}
	return out
}

// medianOf 返回一组整数的中位数。
func medianOf(v []int) int {
	if len(v) == 0 {
		return 0
	}
	cp := append([]int(nil), v...)
	sort.Ints(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}
