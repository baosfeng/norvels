package store

import (
	"context"
	"fmt"
)

// 本文件是回填需要的两件小工具：章节定位信息（不拉正文）与按 id 分页取书。

// chapterRef 是一章在正文库里的定位信息（chapters 表的弱引用三列）。
type chapterRef struct {
	id      int64 // 章节 id（块级报错定位用）
	blockID int64
	off     int
	length  int
}

// bookChapterRefs 按章节顺序读出定位信息（不碰正文块，几十万章也只是很小的切片）。
func (s *Store) bookChapterRefs(ctx context.Context, bookID int64) ([]chapterRef, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, block_id, block_off, block_len FROM chapters WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: list chapter refs book=%d: %w", bookID, err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	out := make([]chapterRef, 0, 64)
	for rows.Next() {
		var r chapterRef
		if err := rows.Scan(&r.id, &r.blockID, &r.off, &r.length); err != nil {
			return nil, fmt.Errorf("store: scan chapter ref book=%d: %w", bookID, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list chapter refs book=%d: %w", bookID, err)
	}
	return out, nil
}

// bookCount 返回主库书数（回填的进度分母）。
func (s *Store) bookCount(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM books").Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count books: %w", err)
	}
	return n, nil
}

// bookIDsAfter 按 id 升序取一批书 id（keyset 分页：两万本书也不把 id 全读进内存）。
func (s *Store) bookIDsAfter(ctx context.Context, after int64, limit int) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id FROM books WHERE id > ? ORDER BY id LIMIT ?", after, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list book ids: %w", err)
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// fingerprintedBookIDs 返回已有指纹的书 id 集合（回填按它做「整本跳过」）。
//
// 一次性读进内存：一行 int64，两万本约 160 KB，比每本书查一次 COUNT 少两万次往返。
func (s *Store) fingerprintedBookIDs(ctx context.Context) (map[int64]struct{}, error) {
	rows, err := s.cdb.QueryContext(ctx, "SELECT DISTINCT book_id FROM chapter_fingerprints")
	if err != nil {
		return nil, fmt.Errorf("store: list fingerprinted books: %w", err)
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}
