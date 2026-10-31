package main

import "testing"

// TestBlockSpans 覆盖按章节边界切块的规则（与 store.splitBlocks 同口径）。
func TestBlockSpans(t *testing.T) {
	cases := []struct {
		name   string
		sizes  []int
		target int
		want   [][2]int
	}{
		{"整本一块", []int{10, 10, 10}, 0, [][2]int{{0, 2}}},
		{"1MB 目标", []int{10, 10, 10}, 25, [][2]int{{0, 1}, {2, 2}}},
		{"单章超目标", []int{30, 5}, 20, [][2]int{{0, 0}, {1, 1}}},
		{"空输入", nil, 100, nil},
	}
	for _, c := range cases {
		got := blockSpans(c.sizes, c.target)
		if len(got) != len(c.want) {
			t.Fatalf("%s: 块数 %d，期望 %d", c.name, len(got), len(c.want))
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s: 第 %d 块 %v，期望 %v", c.name, i, got[i], c.want[i])
			}
		}
	}
}

// TestOffsetsOf 校验前缀和：最后一个偏移必须等于总字节数。
func TestOffsetsOf(t *testing.T) {
	offs := offsetsOf([]int{3, 0, 5})
	if len(offs) != 4 || offs[3] != 8 || offs[2] != 3 {
		t.Fatalf("前缀和错误: %v", offs)
	}
}

// TestBinOf 校验四档边界（左闭右开）。
func TestBinOf(t *testing.T) {
	cases := map[int64]int{
		1 << 20:        binSmall,
		(4 << 20) - 1:  binSmall,
		4 << 20:        binMid,
		(16 << 20) - 1: binMid,
		16 << 20:       binLarge,
		(40 << 20) - 1: binLarge,
		40 << 20:       binHuge,
		75 << 20:       binHuge,
	}
	for raw, want := range cases {
		if got := binOf(raw); got != want {
			t.Fatalf("%d 字节落在档 %d，期望 %d", raw, got, want)
		}
	}
}

// TestPickQuantile 校验分位抽样：确定性、均匀、不越界。
func TestPickQuantile(t *testing.T) {
	ids := make([]int64, 10)
	for i := range ids {
		ids[i] = int64(1000 + i)
	}
	got := pickQuantile(ids, 4)
	if len(got) != 4 || got[0] != 1000 || got[3] != 1007 {
		t.Fatalf("分位抽样结果异常: %v", got)
	}
	if all := pickQuantile(ids, 20); len(all) != len(ids) {
		t.Fatalf("k > len 时应返回全部 %d 本，得到 %d 本", len(ids), len(all))
	}
}

// TestQuotas 校验名额分配：总数恰好为 n，且不超过各档书数。
func TestQuotas(t *testing.T) {
	counts := []int{1000, 400, 3, 0}
	q := quotas(counts, 200)
	sum := 0
	for i, v := range q {
		if v > counts[i] {
			t.Fatalf("档 %d 分了 %d 本，只有 %d 本可用", i, v, counts[i])
		}
		sum += v
	}
	if sum != 200 {
		t.Fatalf("名额合计 %d，期望 200", sum)
	}
	if q[3] != 0 {
		t.Fatalf("空档不该分名额: %v", q)
	}
}

// TestPackRoundTrip 校验「码本 + zstd」的往返：含非法 UTF-8 字节也必须逐字节还原。
func TestPackRoundTrip(t *testing.T) {
	c, err := newCompressor()
	if err != nil {
		t.Fatalf("建压缩器失败: %v", err)
	}
	defer c.Close()

	raw := []byte("第一章 风起\n他说道：「走吧。」\n" + string([]byte{0xFF, 0xFE, 0x41}) + "abc")
	blob, err := packOne(c, raw, true)
	if err != nil {
		t.Fatalf("打包失败: %v", err)
	}
	if blob[0] != storedCodebook {
		t.Fatalf("标记应为码本流，得到 0x%02x", blob[0])
	}
	out, err := unpackOne(c, blob)
	if err != nil {
		t.Fatalf("解包失败: %v", err)
	}
	if string(out) != string(raw) {
		t.Fatalf("往返不一致: %q != %q", out, raw)
	}
}

// TestRatioAndPercentile 校验压缩率、吞吐与分位口径。
func TestRatioAndPercentile(t *testing.T) {
	if got := ratioOf(200, 1000); got != 20 {
		t.Fatalf("压缩率应为 20%%，得到 %v", got)
	}
	if got := throughputMBps(1<<20, int64(1e9)); got != 1 {
		t.Fatalf("1 MB / 1 s 应为 1 MB/s，得到 %v", got)
	}
	sorted := []int64{1e6, 2e6, 3e6, 4e6}
	if got := percentile(sorted, 0.5); got != 2 {
		t.Fatalf("P50 应为 2 ms，得到 %v", got)
	}
	if got := percentile(sorted, 0.95); got != 4 {
		t.Fatalf("P95 应为 4 ms，得到 %v", got)
	}
}
