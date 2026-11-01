package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（CGO-free，便于交叉编译到 NAS）
)

// dsnCandidates 是只读打开库的两条 DSN。
//
// 第一条走标准只读；NAS 上库文件属主是容器用户（65532），普通用户改不动 -shm，
// 此时 SQLite 打不开 WAL 库的只读连接，退回 immutable=1（不加锁、不读 WAL）。
// 两条都带 query_only，任何写语句都会被驱动层直接拒绝。
func dsnCandidates(path string) []string {
	return []string{
		"file:" + path + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(10000)",
		"file:" + path + "?mode=ro&immutable=1&_pragma=query_only(1)",
	}
}

// openReadOnly 以只读方式打开库并返回连接池。
func openReadOnly(path string) (*sql.DB, error) {
	var lastErr error
	for _, dsn := range dsnCandidates(path) {
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			lastErr = err
			continue
		}
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(2)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err == nil {
			fmt.Fprintf(os.Stderr, "codebench: 以只读方式打开库：%s\n", dsn)
			return db, nil
		}
		_ = db.Close()
		lastErr = err
	}
	return nil, fmt.Errorf("只读打开库失败：%w", lastErr)
}

// Library 是全库口径（不抽样）：书数、块数、原始正文字节、库内压缩块字节。
type Library struct {
	Books       int     `json:"books"`
	Blocks      int     `json:"blocks"`
	RawBytes    int64   `json:"raw_bytes"`
	PackedBytes int64   `json:"packed_bytes"`
	Ratio       float64 `json:"ratio"`
	BinBooks    []int   `json:"bin_books"`
}

// BookSize 是一本书的原始正文总字节数（其各块 raw_bytes 之和）。
type BookSize struct {
	ID  int64
	Raw int64
}

// loadLibrary 一次扫描**正文库**的 chapter_blocks 得到全库口径与每本书的原始正文字节数。
//
// 为什么用一条 GROUP BY 而不是两条聚合：库里有 87,600 块 / 约 27 GB BLOB，
// 每多扫一遍就多几十秒到几分钟的 NAS I/O，聚合成每本一次拿全。
//
// 0018 起块表只在正文库里：拿主库连接扫这张表会直接报 no such table。
func loadLibrary(ctx context.Context, db *sql.DB) (Library, []BookSize, error) {
	lib := Library{BinBooks: make([]int, binsCount)}
	rows, err := db.QueryContext(ctx,
		"SELECT book_id, COUNT(*), COALESCE(SUM(raw_bytes), 0), COALESCE(SUM(length(packed)), 0) "+
			"FROM chapter_blocks GROUP BY book_id ORDER BY book_id")
	if err != nil {
		return lib, nil, fmt.Errorf("扫描 chapter_blocks: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄

	sizes := make([]BookSize, 0, 21000)
	for rows.Next() {
		var (
			id     int64
			blocks int
			raw    int64
			packed int64
		)
		if err := rows.Scan(&id, &blocks, &raw, &packed); err != nil {
			return lib, nil, fmt.Errorf("扫描 chapter_blocks 行: %w", err)
		}
		lib.Books++
		lib.Blocks += blocks
		lib.RawBytes += raw
		lib.PackedBytes += packed
		lib.BinBooks[binOf(raw)]++
		sizes = append(sizes, BookSize{ID: id, Raw: raw})
	}
	if err := rows.Err(); err != nil {
		return lib, nil, fmt.Errorf("遍历 chapter_blocks: %w", err)
	}
	if lib.RawBytes > 0 {
		lib.Ratio = float64(lib.PackedBytes) / float64(lib.RawBytes)
	}
	return lib, sizes, nil
}

// pickBooks 按四档书大小等额抽样，档内按 book_id 分位取。
//
// 为什么不取 id 最小的若干本：库里有「乱码书」，字符异常集中，按 id 头部抽会
// 严重高估 3 字节兜底比例。sizes 已按 book_id 升序，档内等距取即是 id 分位。
func pickBooks(sizes []BookSize, n int) []int64 {
	if n <= 0 || len(sizes) == 0 {
		return nil
	}
	perBin := make([][]int64, binsCount)
	for _, s := range sizes {
		b := binOf(s.Raw)
		perBin[b] = append(perBin[b], s.ID)
	}
	counts := make([]int, binsCount)
	for i := range perBin {
		counts[i] = len(perBin[i])
	}
	quota := quotas(counts, n)
	ids := make([]int64, 0, n)
	for i := range perBin {
		ids = append(ids, pickQuantile(perBin[i], quota[i])...)
	}
	return ids
}

// quotas 把 n 个名额分给四档：先均分，某档书不够时把缺额轮转补给还有余量的档。
func quotas(counts []int, n int) []int {
	q := make([]int, len(counts))
	base := n / len(counts)
	used := 0
	for i := range q {
		q[i] = min(base, counts[i])
		used += q[i]
	}
	for used < n {
		grew := false
		for i := range q {
			if used < n && q[i] < counts[i] {
				q[i]++
				used++
				grew = true
			}
		}
		if !grew {
			break
		}
	}
	return q
}

// pickQuantile 在按 id 升序的列表里均匀取 k 个（确定性分位抽样，重跑结果一致）。
func pickQuantile(ids []int64, k int) []int64 {
	if k <= 0 || len(ids) == 0 {
		return nil
	}
	if k >= len(ids) {
		return append([]int64(nil), ids...)
	}
	out := make([]int64, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, ids[i*len(ids)/k])
	}
	return out
}

// bookSample 是一本已还原的书：原始 UTF-8 正文 + 每章字节数 + 库内块体积。
type bookSample struct {
	id          int64
	raw         []byte
	sizes       []int
	dbBlobBytes int64
}

// loadSamples 逐本读出正文（这段 I/O 与解压不计入任何方案的计时）。
//
// db 是主库（章节字节数），cdb 是正文库（packed BLOB）。
func loadSamples(ctx context.Context, db, cdb *sql.DB, ids []int64, c *compressor) ([]bookSample, error) {
	books := make([]bookSample, 0, len(ids))
	var total int64
	for i, id := range ids {
		b, err := loadSample(ctx, db, cdb, id, c)
		if err != nil {
			return nil, err
		}
		total += int64(len(b.raw))
		books = append(books, b)
		if (i+1)%20 == 0 || i+1 == len(ids) {
			fmt.Fprintf(os.Stderr, "codebench: 已载入 %d/%d 本，累计 %.1f MB 正文\n",
				i+1, len(ids), float64(total)/(1<<20))
		}
	}
	return books, nil
}

// loadSample 从正文库读一本书的全部块、解压拼成正文，再从主库取每章字节数
// （章节只存块寻址，正文以正文库的块为准）。
func loadSample(ctx context.Context, db, cdb *sql.DB, id int64, c *compressor) (bookSample, error) {
	b := bookSample{id: id}
	rows, err := cdb.QueryContext(ctx,
		"SELECT raw_bytes, packed FROM chapter_blocks WHERE book_id = ? ORDER BY seq", id)
	if err != nil {
		return b, fmt.Errorf("book %d: 读块: %w", id, err)
	}
	for rows.Next() {
		var (
			rawBytes int64
			packed   []byte
		)
		if err := rows.Scan(&rawBytes, &packed); err != nil {
			_ = rows.Close()
			return b, fmt.Errorf("book %d: 扫描块: %w", id, err)
		}
		blk, derr := decompressStored(c, packed)
		if derr != nil {
			_ = rows.Close()
			return b, fmt.Errorf("book %d: %w", id, derr)
		}
		if int64(len(blk)) != rawBytes {
			_ = rows.Close()
			return b, fmt.Errorf("book %d: 块解压出 %d 字节，raw_bytes 记 %d", id, len(blk), rawBytes)
		}
		b.raw = append(b.raw, blk...)
		b.dbBlobBytes += int64(len(packed))
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return b, fmt.Errorf("book %d: 遍历块: %w", id, err)
	}
	_ = rows.Close()

	if b.sizes, err = loadChapterSizes(ctx, db, id); err != nil {
		return b, err
	}
	sum := 0
	for _, n := range b.sizes {
		sum += n
	}
	if sum != len(b.raw) {
		return b, fmt.Errorf("book %d: 章节字节和 %d != 块正文 %d（块与章节记录不一致）", id, sum, len(b.raw))
	}
	return b, nil
}

// loadChapterSizes 从**主库**取一本书每章的字节数（chapters.block_len，按 idx 排序），
// 供「按章节边界切块」复现库内的分块规则。空正文的章（block_id = 0）block_len 为 0。
func loadChapterSizes(ctx context.Context, db *sql.DB, id int64) ([]int, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT block_len FROM chapters WHERE book_id = ? ORDER BY idx, id", id)
	if err != nil {
		return nil, fmt.Errorf("book %d: 读章节尺寸: %w", id, err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	sizes := make([]int, 0, 256)
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("book %d: 扫描章节尺寸: %w", id, err)
		}
		sizes = append(sizes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("book %d: 遍历章节尺寸: %w", id, err)
	}
	return sizes, nil
}
