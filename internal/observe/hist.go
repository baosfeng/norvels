package observe

// BucketEdgesMs 是直方图的分桶上界（毫秒）：桶 i = [BucketEdgesMs[i-1], BucketEdgesMs[i])，
// 首桶 [0, BucketEdgesMs[0])，末桶 >= BucketEdgesMs[len-1]（无上界）。
var BucketEdgesMs = bucketEdges[:]

// bucketEdges 用数组声明：numBuckets 需要 len 是常量表达式（切片做不到）。
var bucketEdges = [...]float64{
	0.5, 1, 2.5, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000,
}

// numBuckets 是桶数 = 边界数 + 1（末桶无上界）。
const numBuckets = len(bucketEdges) + 1

// histogram 是定长桶计数：不存原始样本，内存恒定（17 个 int64 = 136 字节）。
type histogram struct {
	counts [numBuckets]int64
}

func (h *histogram) add(ms float64) { h.counts[bucketIndex(ms)]++ }

// bucketIndex 找到 ms 落在哪个桶：边界只 16 个，线性扫描比二分更快也更好读。
func bucketIndex(ms float64) int {
	if ms <= 0 {
		return 0
	}
	for i, edge := range BucketEdgesMs {
		if ms < edge {
			return i
		}
	}
	return numBuckets - 1
}

// quantile 估算分位值：落在哪一桶就按桶内均匀分布线性插值，末桶用 maxMs 兜底。
//
// 这是「分桶插值」而不是精确分位：误差上限是一个桶宽，换来的是恒定内存与 O(桶数) 查询。
func (h *histogram) quantile(q float64, total int64, maxMs float64) float64 {
	if total <= 0 {
		return 0
	}
	target := q * float64(total)
	var cum int64
	for i, c := range h.counts {
		if c == 0 {
			continue
		}
		if float64(cum+c) >= target {
			// 分位不可能超过实测最大值：单样本落在 [250,500) 时插值会顶到桶上界，
			// 用 maxMs 收口才不会把 258ms 的请求报成 487ms。
			return round2(clampMax(interpolate(i, cum, c, target, maxMs), maxMs))
		}
		cum += c
	}
	return round2(maxMs)
}

// clampMax 把估算值收口到实测最大值（maxMs 为 0 表示还没有样本）。
func clampMax(v, maxMs float64) float64 {
	if maxMs > 0 && v > maxMs {
		return maxMs
	}
	return v
}

// interpolate 在桶 i 的区间内按「落点位置」插值。
func interpolate(i int, cum, count int64, target, maxMs float64) float64 {
	lo := lowerEdge(i)
	hi := upperEdge(i, maxMs)
	if hi <= lo || count <= 0 {
		return lo
	}
	frac := (target - float64(cum)) / float64(count)
	if frac < 0 {
		frac = 0
	} else if frac > 1 {
		frac = 1
	}
	return lo + (hi-lo)*frac
}

// lowerEdge 是桶 i 的下界（首桶 0）。
func lowerEdge(i int) float64 {
	if i <= 0 {
		return 0
	}
	return BucketEdgesMs[i-1]
}

// upperEdge 是桶 i 的上界；末桶无上界时用观测到的最大值兜底。
func upperEdge(i int, maxMs float64) float64 {
	if i < len(BucketEdgesMs) {
		return BucketEdgesMs[i]
	}
	if maxMs <= lowerEdge(i) {
		return lowerEdge(i)
	}
	return maxMs
}

// snapshotBuckets 复制一份桶计数（调用方拿到的是快照，不会被后续写入改掉）。
func (h *histogram) snapshot() []int64 {
	out := make([]int64, numBuckets)
	copy(out, h.counts[:])
	return out
}
