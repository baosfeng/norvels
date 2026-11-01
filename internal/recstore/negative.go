package recstore

import (
	"context"
	"database/sql"
	"fmt"
)

// 负反馈（dislike）：一本书被标记「不感兴趣」后，召回阶段整本剔除。

const upsertNegativeSQL = "INSERT INTO negative_items(user_id, book_id, ts) VALUES(?,?,?) " +
	"ON CONFLICT(user_id, book_id) DO UPDATE SET ts = excluded.ts"

// NegativeItems 返回该账号标记过「不感兴趣」的书集合。
//
// 与 ReadBooks 同层使用：已读不推、讨厌也不推，两者都是「不该出现在结果里」的硬过滤。
// 不返回时间：调用方只关心「是否拉黑」，时间只在库里留着，供将来做「负反馈过期」兜底。
func (s *Store) NegativeItems(ctx context.Context, userID int64) (map[int64]bool, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("%w: user_id %d", ErrInvalidArgument, userID)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT book_id FROM negative_items WHERE user_id = ?", userID)
	if err != nil {
		return nil, fmt.Errorf("recstore: 查负反馈: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]bool, 16)
	for rows.Next() {
		var bookID int64
		if err := rows.Scan(&bookID); err != nil {
			return nil, fmt.Errorf("recstore: 扫描负反馈: %w", err)
		}
		out[bookID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recstore: 遍历负反馈: %w", err)
	}
	return out, nil
}

// upsertNegativeItem 记录一次「不感兴趣」（重复标记只更新时间）。
func upsertNegativeItem(ctx context.Context, tx *sql.Tx, userID, bookID int64, ts string) error {
	if _, err := tx.ExecContext(ctx, upsertNegativeSQL, userID, bookID, ts); err != nil {
		return fmt.Errorf("recstore: 写负反馈: %w", err)
	}
	return nil
}
