package main

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// block 是一条待统计的块（packed 是「标记 + 载荷」的完整字节）。
type block struct {
	id     int64
	packed []byte
}

// progressEvery 每处理多少块打一行进度：全库约 8 万块，5000 块约十几秒。
const progressEvery = 5000

// countAll 单连接顺序读块、多 worker 并行解压计数。
//
// 读是 IO 串行（SQLite 单连接）、解压是 CPU 并行，流水线重叠后接近满核；
// 块在交给 worker 前已拷贝，worker 之间不共享缓冲。
func countAll(db *sql.DB, p plan, workers int) (*counter, error) {
	jobs := make(chan block, workers*2)
	results := make(chan *counter, workers)
	var wg sync.WaitGroup
	var done int64
	start := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- scanWorker(jobs, &done, p.blocks, start)
		}()
	}

	feedErr := feed(db, p, jobs)
	close(jobs)
	wg.Wait()
	close(results)

	agg := newCounter()
	var firstErr error
	for c := range results {
		if c.err != nil && firstErr == nil {
			firstErr = c.err
		}
		agg.merge(c)
	}
	if feedErr != nil {
		return nil, feedErr
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return agg, nil
}

// scanWorker 消费块队列直到关闭，返回本 worker 的计数桶。
func scanWorker(jobs <-chan block, done *int64, total int, start time.Time) *counter {
	c := newCounter()
	for b := range jobs {
		if err := c.addBlock(b); err != nil && c.err == nil {
			c.err = err
		}
		if n := atomic.AddInt64(done, 1); total > 0 && n%progressEvery == 0 {
			fmt.Fprintf(os.Stderr, "corpusstats: 已处理 %d/%d 块（%.0fs）\n",
				n, total, time.Since(start).Seconds())
		}
	}
	return c
}

// feed 按计划把块送进队列。
func feed(db *sql.DB, p plan, jobs chan<- block) error {
	for _, q := range p.queries {
		if err := feedQuery(db, q, jobs); err != nil {
			return err
		}
	}
	return nil
}

// feedQuery 跑一条查询并把每块的字节拷贝后入队。
//
// 必须拷贝：database/sql 交给我们的 []byte 在下次 Next() 后即失效。
func feedQuery(db *sql.DB, q query, jobs chan<- block) error {
	rows, err := db.Query(q.sql, q.args...)
	if err != nil {
		return fmt.Errorf("查询块失败: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var packed []byte
		if err := rows.Scan(&id, &packed); err != nil {
			return fmt.Errorf("扫块失败: %w", err)
		}
		raw := make([]byte, len(packed))
		copy(raw, packed)
		jobs <- block{id: id, packed: raw}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历块失败: %w", err)
	}
	return nil
}
