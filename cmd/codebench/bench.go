package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"
)

// 书大小分档（正文原始 UTF-8 字节）：<4MB / 4–16MB / 16–40MB / >40MB。
const (
	binSmall = iota
	binMid
	binLarge
	binHuge
	binsCount
)

// binLabels 是四档的展示名，下标与 bin 常量一致。
var binLabels = [binsCount]string{"<4MB", "4–16MB", "16–40MB", ">40MB"}

// binLimits 是各档上界（左闭右开，最后一档为无上界）。
var binLimits = [binsCount]int64{4 << 20, 16 << 20, 40 << 20, 1 << 62}

// binOf 返回正文字节数所属的档位下标。
func binOf(raw int64) int {
	for i, limit := range binLimits {
		if raw < limit {
			return i
		}
	}
	return binsCount - 1
}

// group 是一组待测方案：块目标字节数（0 = 整本一块）+ 是否先过 textcode 码本。
type group struct {
	key  string
	name string
	blk  int
	code bool
}

// groups 是本次实测的五组方案，顺序即报告顺序。
var groups = []group{
	{"plain_1mb", "① 迁移前基线：纯 1MB 章界块 zstd", blockTarget, false},
	{"code_1mb", "② 码本 + 1MB 章界块 zstd", blockTarget, true},
	{"code_8mb", "③ 码本 + 8MB 章界块 zstd（= 迁移 0017 后的库内格式）", blockTarget * 8, true},
	{"code_book", "④ 码本 + 整本 zstd", 0, true},
	{"plain_book", "⑤ 纯整本 zstd（对照）", 0, false},
}

// bookResult 是一本书在一组方案下的测量：体积、压缩耗时、每次解压延迟。
type bookResult struct {
	bookID  int64
	bin     int
	raw     int64
	packed  int64
	dbBytes int64
	compNs  int64
	ops     []int64
	ok      bool
}

// run 是全流程：开库 → 全库口径 → 抽样 → 逐组实测 → 最大书单独测 → 出报告。
func run(dbPath string, sampleN int, outPath string, biggestID int64) error {
	c, err := newCompressor()
	if err != nil {
		return err
	}
	defer c.Close()

	ctx := context.Background()
	db, err := openReadOnly(dbPath)
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck // 只读句柄

	fmt.Fprintln(os.Stderr, "codebench: 扫描 chapter_blocks 汇总全库口径（约 27 GB BLOB，可能需要几分钟）…")
	lib, sizes, err := loadLibrary(ctx, db)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "codebench: 全库 %d 本 / %d 块 / %d 字节正文 / 库内 BLOB %d 字节 = %.2f%%\n",
		lib.Books, lib.Blocks, lib.RawBytes, lib.PackedBytes, lib.Ratio*100)

	res := &Result{
		GeneratedAt: time.Now().Format(time.RFC3339),
		DBPath:      dbPath,
		Sample:      sampleN,
		Library:     lib,
	}
	ids := pickBooks(sizes, sampleN)
	if len(ids) > 0 {
		if res.Groups, err = benchSample(ctx, db, c, ids, sizes, lib); err != nil {
			return err
		}
	}
	if biggestID > 0 {
		if res.Largest, err = benchLargest(ctx, db, c, biggestID); err != nil {
			return err
		}
	}
	return writeReports(res, outPath)
}

// batchBudgetBytes 是一批同时驻留内存的正文上限。
//
// 为什么要分批：抽样 200 本可能合计数 GB，NAS 只有 7.7 GB 内存且已被生产服务占用，
// 一次性载入会直接进 swap，把吞吐测量变成磁盘测量。
const batchBudgetBytes = 512 << 20

// benchSample 分批载入抽样书、逐组实测，最后汇总成五组统计。
func benchSample(ctx context.Context, db *sql.DB, c *compressor, ids []int64,
	sizes []BookSize, lib Library) ([]GroupStat, error) {
	sizeOf := make(map[int64]int64, len(sizes))
	for _, s := range sizes {
		sizeOf[s.ID] = s.Raw
	}
	perGroup := make([][]bookResult, len(groups))
	for start := 0; start < len(ids); {
		end := batchEnd(ids, sizeOf, start)
		books, err := loadSamples(ctx, db, ids[start:end], c)
		if err != nil {
			return nil, err
		}
		for gi := range groups {
			started := time.Now()
			for _, b := range books {
				perGroup[gi] = append(perGroup[gi], runBook(c, groups[gi], b))
			}
			fmt.Fprintf(os.Stderr, "codebench: %-10s 已测 %d/%d 本（本批 %.0f s）\n",
				groups[gi].key, len(perGroup[gi]), len(ids), time.Since(started).Seconds())
		}
		start = end
	}
	stats := make([]GroupStat, 0, len(groups))
	for gi, g := range groups {
		st := summarize(g, perGroup[gi], lib)
		stats = append(stats, st)
		fmt.Fprintf(os.Stderr,
			"codebench: %-10s 压缩率 %6.2f%% / 加权 %6.2f%% / 吞吐 %6.2f MB/s / P50 %7.2f ms / P95 %7.2f ms / 失败 %d\n",
			g.key, st.Ratio, st.WeightedRatio, st.CompressMBps, st.P50Ms, st.P95Ms, st.Failures)
	}
	return stats, nil
}

// batchEnd 从 start 起按预算切一批，至少含一本（单本超预算时独占一批）。
func batchEnd(ids []int64, sizeOf map[int64]int64, start int) int {
	end := start
	var acc int64
	for end < len(ids) {
		acc += sizeOf[ids[end]]
		end++
		if acc >= batchBudgetBytes {
			break
		}
	}
	return end
}

// runBook 测一本书在一组方案下的压缩体积、压缩耗时、逐块解压延迟与往返一致性。
//
// 往返校验两道：逐块 bytes.Equal（保证每一块都不丢字节），以及整本解码结果的
// SHA-256 与原文比对（挡住「切块漏章 / 顺序错」这类结构性错误）。
func runBook(c *compressor, g group, b bookSample) bookResult {
	res := bookResult{
		bookID:  b.id,
		bin:     binOf(int64(len(b.raw))),
		raw:     int64(len(b.raw)),
		dbBytes: b.dbBlobBytes,
		ok:      true,
	}
	offs := offsetsOf(b.sizes)
	hasher := sha256.New()
	for _, sp := range blockSpans(b.sizes, g.blk) {
		if sp[0] < 0 || sp[1] >= len(offs)-1 {
			return failBook(res, fmt.Errorf("book %d: 块区间 [%d,%d] 越界", b.id, sp[0], sp[1]))
		}
		src := b.raw[offs[sp[0]]:offs[sp[1]+1]]

		t0 := time.Now()
		blob, cerr := packOne(c, src, g.code)
		res.compNs += time.Since(t0).Nanoseconds()
		if cerr != nil {
			return failBook(res, cerr)
		}
		res.packed += int64(len(blob))

		t1 := time.Now()
		dec, uerr := unpackOne(c, blob)
		res.ops = append(res.ops, time.Since(t1).Nanoseconds())
		if uerr != nil {
			return failBook(res, uerr)
		}
		if !bytes.Equal(dec, src) {
			return failBook(res, fmt.Errorf("book %d: 块 [%d,%d] 往返不一致", b.id, sp[0], sp[1]))
		}
		hasher.Write(dec)
	}
	if !bytes.Equal(hasher.Sum(nil), sha256Sum(b.raw)) {
		return failBook(res, fmt.Errorf("book %d: 整本 SHA-256 不一致", b.id))
	}
	return res
}

// failBook 把一次失败的测量标成不通过并带上原因。
func failBook(res bookResult, err error) bookResult {
	res.ok = false
	fmt.Fprintf(os.Stderr, "codebench: 往返校验失败: %v\n", err)
	return res
}

// sha256Sum 返回一段字节的 SHA-256。
func sha256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

// largestRuns 是对最大书连续解压的次数（第 1 次含首次分配，后 3 次算稳态）。
const largestRuns = 4

// benchLargest 单独测最大书在「码本 + 整本 zstd」下的整本耗时与内存峰值。
func benchLargest(ctx context.Context, db *sql.DB, c *compressor, id int64) (*LargestStat, error) {
	b, err := loadSample(ctx, db, id, c)
	if err != nil {
		return nil, err
	}
	st := &LargestStat{
		BookID:   id,
		Chapters: len(b.sizes),
		RawBytes: int64(len(b.raw)),
		DBBytes:  b.dbBlobBytes,
	}
	started := time.Now()
	blob, err := packOne(c, b.raw, true)
	st.EncodeMs = elapsedMs(started)
	if err != nil {
		return nil, err
	}
	st.PackedBytes = int64(len(blob))
	st.Ratio = ratioOf(st.PackedBytes, st.RawBytes)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	var peak uint64
	okAll := true
	for i := 0; i < largestRuns; i++ {
		t0 := time.Now()
		var dec []byte
		var derr error
		if i == 0 {
			peak = sampleHeap(func() { dec, derr = unpackOne(c, blob) })
		} else {
			dec, derr = unpackOne(c, blob)
		}
		st.DecodeMs = append(st.DecodeMs, elapsedMs(t0))
		if derr != nil || !bytes.Equal(dec, b.raw) {
			okAll = false
			fmt.Fprintf(os.Stderr, "codebench: 最大书解压校验失败: %v\n", derr)
		}
	}
	runtime.ReadMemStats(&after)
	st.HeapAllocPeak = peak
	st.TotalAllocBytes = after.TotalAlloc - before.TotalAlloc
	st.RSSPeakBytes = rssPeak()
	st.RoundTrip = okAll
	st.SteadyMs = averageMs(st.DecodeMs[1:])
	return st, nil
}

// sampleHeap 在 fn 执行期间按 2 ms 采样 HeapAlloc，返回峰值字节数。
func sampleHeap(fn func()) uint64 {
	stop := make(chan struct{})
	done := make(chan uint64, 1)
	go func() {
		var peak uint64
		var ms runtime.MemStats
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				done <- peak
				return
			case <-ticker.C:
				runtime.ReadMemStats(&ms)
				peak = max(peak, ms.HeapAlloc)
			}
		}
	}()
	fn()
	close(stop)
	return <-done
}

// rssPeak 读 /proc/self/status 的 VmHWM（Linux 峰值 RSS）；读不到就返回 0。
func rssPeak() uint64 {
	blob, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range bytes.Split(blob, []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("VmHWM:")) {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, perr := strconv.ParseUint(string(fields[1]), 10, 64)
		if perr != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}

// elapsedMs 把一段耗时换成毫秒。
func elapsedMs(start time.Time) float64 {
	return float64(time.Since(start).Nanoseconds()) / 1e6
}

// averageMs 求一组毫秒值的均值（空切片返回 0）。
func averageMs(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
