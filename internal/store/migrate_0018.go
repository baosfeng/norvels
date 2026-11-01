package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// 0018 的数据搬迁：把主库的 chapter_blocks 逐书搬进正文库，搬完 DROP 主库的表。
//
// 为什么逐书：全量库 87,626 个块、27.5 GB，一次读进内存会 OOM。逐书 = 一本书的
// 块同时在内存里的只有游标当前那一行（单块最大 8 MB 原文，压缩后约 2 MB），
// 生产实测峰值内存与「单块」同量级。每本书一个正文库事务，中断后重跑代价可控。
//
// 为什么块 id 原样保留：chapters.block_id 是弱引用，搬过去必须还是同一个 id，
// 否则整个 chapters 表都要重写一遍（79.8 亿字节的正文库没有第二份磁盘空间）。

// splitContentBlocksStep 是 0018 的迁移步骤（绑定 Store，才能拿到正文库连接）。
//
// 幂等：主库已经没有 chapter_blocks 表 → 说明这条迁移已经完成，直接返回
// （删除版本记录重跑、或人工重放都不会改动任何数据）。
func splitContentBlocksStep(ctx context.Context, tx *sql.Tx, s *Store) error {
	has, err := tableExistsInTx(ctx, tx, "chapter_blocks")
	if err != nil {
		return err
	}
	if !has {
		return nil // 已搬迁完成
	}
	if err := s.ensureContentSchema(ctx); err != nil {
		return err
	}
	books, err := contentBookIDsInTx(ctx, tx)
	if err != nil {
		return err
	}
	moved := 0
	for _, bookID := range books {
		n, err := copyBookBlocks(ctx, tx, s, bookID)
		if err != nil {
			return fmt.Errorf("book %d: %w", bookID, err)
		}
		moved += n
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE chapter_blocks"); err != nil {
		return fmt.Errorf("drop chapter_blocks: %w", err)
	}
	log.Printf("迁移 0018：正文块搬进正文库 %d 本 / %d 块，主库已 DROP TABLE chapter_blocks",
		len(books), moved)
	return nil
}

// contentBookIDsInTx 返回有块的书 id（升序），主库里逐书搬运的游标。
func contentBookIDsInTx(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT DISTINCT book_id FROM chapter_blocks ORDER BY book_id")
	if err != nil {
		return nil, fmt.Errorf("list books with blocks: %w", err)
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// copyBookBlocks 把一本书的块从主库搬进正文库，返回搬运的块数。
//
// INSERT OR REPLACE 按 id 覆盖：上一次搬运在中间失败时，同一批 id 再写一次即可，
// 不需要先清空正文库（清了反而会在「主库表还在、正文库被清空」的窗口里丢数据）。
func copyBookBlocks(ctx context.Context, tx *sql.Tx, s *Store, bookID int64) (int, error) {
	ct, err := s.cdb.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: 0018 begin content: %w", err)
	}
	defer ct.Rollback() //nolint:errcheck // 提交后回滚是空操作

	n, err := copyBookBlockRows(ctx, tx, ct, bookID)
	if err != nil {
		return 0, err
	}
	if err := ct.Commit(); err != nil {
		return 0, fmt.Errorf("store: 0018 commit content: %w", err)
	}
	return n, nil
}

// copyBookBlockRows 逐行搬运一本书的块（游标一次只持有一行，内存有界）。
func copyBookBlockRows(ctx context.Context, tx, ct *sql.Tx, bookID int64) (int, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT id, book_id, seq, first_idx, last_idx, raw_bytes, packed FROM chapter_blocks "+
			"WHERE book_id = ? ORDER BY seq, id", bookID)
	if err != nil {
		return 0, fmt.Errorf("read old blocks: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄

	n := 0
	for rows.Next() {
		var (
			id, book            int64
			seq, firstIdx, last int
			raw                 int
			packed              []byte
		)
		if err := rows.Scan(&id, &book, &seq, &firstIdx, &last, &raw, &packed); err != nil {
			return 0, fmt.Errorf("scan old block: %w", err)
		}
		if _, err := ct.ExecContext(ctx,
			"INSERT OR REPLACE INTO chapter_blocks(id, book_id, seq, first_idx, last_idx, raw_bytes, packed) "+
				"VALUES (?, ?, ?, ?, ?, ?, ?)",
			id, book, seq, firstIdx, last, raw, packed); err != nil {
			return 0, fmt.Errorf("insert content block %d: %w", id, err)
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate old blocks: %w", err)
	}
	return n, nil
}
