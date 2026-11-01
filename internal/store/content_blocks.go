package store

import (
	"context"
	"database/sql"
	"fmt"
)

// 本文件是正文库里 chapter_blocks 的**唯一写入口径**：块行怎么插、seq 怎么续、
// 一本书的体积怎么算，都只在这里出现一次。
//
// 读路径（loadBlock / readBlockRow*）留在 blocks.go：它们是 querier 参数化的，
// 0007 迁移要在主库事务里读旧表、运行时要读正文库，同一个实现两边都要能用。

// insertBlock 往正文库写一块，返回块 id。
//
// firstIdx / lastIdx 是这块覆盖的章节**序号**（chapters.idx），不是章节 id：
// 只用于块损坏时的定位文案。seq 是本书内的块序号，UNIQUE(book_id, seq) 保证不重。
// 写入 id 记进 written，主库提交失败时按它精确回滚（不按 book_id 删——回滚的新书
// id 可能被后面的书复用）。
func (c *contentTx) insertBlock(ctx context.Context, bookID int64, seq, firstIdx, lastIdx int,
	raw, packed []byte) (int64, error) {
	res, err := c.tx.ExecContext(ctx,
		"INSERT INTO chapter_blocks(book_id, seq, first_idx, last_idx, raw_bytes, packed) "+
			"VALUES (?, ?, ?, ?, ?, ?)",
		bookID, seq, firstIdx, lastIdx, len(raw), packed)
	if err != nil {
		return 0, fmt.Errorf("store: insert block book=%d seq=%d: %w", bookID, seq, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: insert block id: %w", err)
	}
	c.written = append(c.written, id)
	return id, nil
}

// nextSeq 返回本书下一个可用块序号（0 表示本书还没有块；调用方 +1 使用）。
//
// 合并 / 编辑会留下 seq 空洞：被回收的块不会把序号让出来。seq 只用于块级错误
// 文案里的「第几块」，不参与寻址，取 MAX+1 比维护连续更划算。
func (c *contentTx) nextSeq(ctx context.Context, bookID int64) (int, error) {
	var seq int
	if err := c.tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(seq), 0) FROM chapter_blocks WHERE book_id = ?", bookID).Scan(&seq); err != nil {
		return 0, fmt.Errorf("store: max block seq %d: %w", bookID, err)
	}
	return seq, nil
}

// blockText 解出一块正文（事务内读，能看到本事务刚写入的块）。
func (c *contentTx) blockText(ctx context.Context, blockID, chapterID int64) ([]byte, error) {
	return loadBlock(ctx, c.tx, blockID, chapterID)
}

// blockSum 是一本书在正文库的体积口径快照。
type blockSum struct {
	packed int64 // 压缩后 packed 字节之和
	raw    int64 // 原文逻辑字节之和
}

// sums 求一本书在正文库的体积口径（覆盖写入前要先从聚合里减掉旧值）。
func (c *contentTx) sums(ctx context.Context, bookID int64) (blockSum, error) {
	var s blockSum
	err := c.tx.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(length(packed)), 0), COALESCE(SUM(raw_bytes), 0) "+
			"FROM chapter_blocks WHERE book_id = ?", bookID).
		Scan(&s.packed, &s.raw)
	if err != nil {
		return s, fmt.Errorf("store: block sums %d: %w", bookID, err)
	}
	return s, nil
}

// blockIDs 返回一本书当前的块 id（覆盖写入时把它们挂到「提交后回收」清单）。
func (c *contentTx) blockIDs(ctx context.Context, bookID int64) ([]int64, error) {
	rows, err := c.tx.QueryContext(ctx, "SELECT id FROM chapter_blocks WHERE book_id = ?", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: list block ids %d: %w", bookID, err)
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// blockDB 返回「此刻块表所在的那个库」。
//
// 0018 之后块表在正文库；迁移停在中间（`norvels migrate --to 17`）、
// 或库还是 0018 之前的结构时，块表仍在主库。读路径据此选连接，写入侧不走这里
// （写路径要求正文库表已建好，表不在就直接报错，不会静默写到错的地方）。
func (s *Store) blockDB(ctx context.Context) (*sql.DB, error) {
	ok, err := s.contentTableExists(ctx)
	if err != nil {
		return nil, err
	}
	if ok {
		return s.cdb, nil
	}
	return s.db, nil
}

// loadBlockText 读并解压一个正文块（自动选库）。
func (s *Store) loadBlockText(ctx context.Context, blockID, chapterID int64) ([]byte, error) {
	q, err := s.blockDB(ctx)
	if err != nil {
		return nil, err
	}
	return loadBlock(ctx, q, blockID, chapterID)
}

// blockMeta 只读块的定位元信息（自动选库）。
func (s *Store) blockMeta(ctx context.Context, blockID int64) (blockMeta, error) {
	q, err := s.blockDB(ctx)
	if err != nil {
		return blockMeta{}, err
	}
	return readBlockRowMetaOnly(ctx, q, blockID)
}

// contentTableExists 判断正文库里有没有 chapter_blocks（0018 之前的库没有）。
func (s *Store) contentTableExists(ctx context.Context) (bool, error) {
	var n int
	err := s.cdb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name = 'chapter_blocks'").Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: check content table: %w", err)
	}
	return n > 0, nil
}
