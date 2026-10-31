package main

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// syntheticBook 造一本合成书：raw 是正文，sizes 是逐章字节数（和必须等于 len(raw)）。
func syntheticBook(id int64) bookSample {
	raw := []byte(strings.Repeat("第一章 正文内容，用来测压缩率与往返；abcdefghij", 120))
	return bookSample{id: id, raw: raw, sizes: []int{10, 10, len(raw) - 20}, dbBlobBytes: 4096}
}

// TestRunBookAllGroups 用合成正文跑完五组方案：往返必须通过，解压次数必须等于块数。
func TestRunBookAllGroups(t *testing.T) {
	c, err := newCompressor()
	if err != nil {
		t.Fatalf("建压缩器失败: %v", err)
	}
	defer c.Close()

	b := syntheticBook(7)
	for _, g := range groups {
		res := runBook(c, g, b)
		if !res.ok {
			t.Fatalf("%s: 往返校验失败", g.key)
		}
		if res.raw != int64(len(b.raw)) {
			t.Fatalf("%s: 原始字节 %d，期望 %d", g.key, res.raw, len(b.raw))
		}
		if res.packed <= 0 || res.compNs <= 0 {
			t.Fatalf("%s: 压缩体积/耗时未测到：packed=%d compNs=%d", g.key, res.packed, res.compNs)
		}
		if want := len(blockSpans(b.sizes, g.blk)); len(res.ops) != want {
			t.Fatalf("%s: 解压次数 %d，期望块数 %d", g.key, len(res.ops), want)
		}
		if res.bin != binOf(res.raw) {
			t.Fatalf("%s: 档位 %d 与正文字节数不匹配", g.key, res.bin)
		}
	}
}

// TestRunBookDetectsHashMismatch 锁住整本 SHA-256 校验：块覆盖不全时必须判失败。
func TestRunBookDetectsHashMismatch(t *testing.T) {
	c, err := newCompressor()
	if err != nil {
		t.Fatalf("建压缩器失败: %v", err)
	}
	defer c.Close()

	b := syntheticBook(8)
	b.sizes = []int{len(b.raw) - 1} // 少一字节 → 拼回来的正文与原文不等
	if res := runBook(c, groups[1], b); res.ok {
		t.Fatal("块覆盖不全时往返校验必须失败，否则缺字的书会静默通过")
	}
}

// TestBatchEnd 校验分批预算：按字节切批，单本超预算时独占一批。
func TestBatchEnd(t *testing.T) {
	ids := []int64{1, 2, 3}
	sizeOf := map[int64]int64{1: 300 << 20, 2: 300 << 20, 3: 10}
	if got := batchEnd(ids, sizeOf, 0); got != 2 {
		t.Fatalf("300MB + 300MB 应在第 2 本后切批，得到 %d", got)
	}
	big := map[int64]int64{1: 900 << 20}
	if got := batchEnd([]int64{1}, big, 0); got != 1 {
		t.Fatalf("单本超预算应独占一批，得到 %d", got)
	}
	if got := batchEnd(ids, sizeOf, len(ids)); got != len(ids) {
		t.Fatalf("越界起点应原地返回，得到 %d", got)
	}
}

// TestSampleHeap 校验 2 ms 采样真的能抓到堆峰值（抓不到就没法报内存指标）。
func TestSampleHeap(t *testing.T) {
	peak := sampleHeap(func() {
		buf := make([]byte, 8<<20)
		for i := range buf {
			buf[i] = byte(i)
		}
		runtime.KeepAlive(buf)
		time.Sleep(12 * time.Millisecond)
	})
	if peak == 0 {
		t.Fatal("采样期间有 8 MB 分配，HeapAlloc 峰值不该是 0")
	}
}

// TestAverageMsAndElapsed 校验毫秒聚合与计时换算。
func TestAverageMsAndElapsed(t *testing.T) {
	if got := averageMs(nil); got != 0 {
		t.Fatalf("空切片均值应为 0，得到 %v", got)
	}
	if got := averageMs([]float64{1, 3}); got != 2 {
		t.Fatalf("均值应为 2，得到 %v", got)
	}
	got := elapsedMs(time.Now().Add(-50 * time.Millisecond))
	if got < 40 || got > 5000 {
		t.Fatalf("elapsedMs 口径异常: %v", got)
	}
}

// TestRssPeakNeverPanics 校验 RSS 探测在无 /proc 的平台安全降级（返回 0 即可）。
func TestRssPeakNeverPanics(t *testing.T) {
	if got := rssPeak(); got > 1<<46 {
		t.Fatalf("RSS 峰值明显不合理: %d", got)
	}
}
