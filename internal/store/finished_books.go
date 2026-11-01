package store

import (
	"context"
	"fmt"
)

// finishedPercent 是「读完」的判定阈值。
//
// 为什么不再用「有过 read 事件」当已读：读了几分钟就退出是常态，真实数据里几乎
// 每本书都被 read 覆盖过；把这类书一律过滤，候选池会随着使用越来越小，最终推不出书。
// 只有阅读器上报的位置（reading_progress.percent）才能区分「翻了几页」与「读完了」。
const finishedPercent = 0.99

// FinishedBookIDs 返回当前账号**读完**的书集合（进度 ≥ 0.99）。
//
// 账号维度取 ScopeUserID(ctx)：进度按账号隔离，同一本书换个账号就是另一次阅读。
// 空库返回空 map（不是 nil）：调用方在 buildRecommendations 里直接按 map 查，不必再判空。
func (s *Store) FinishedBookIDs(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT book_id FROM reading_progress WHERE user_id = ? AND percent >= ?",
		ScopeUserID(ctx), finishedPercent)
	if err != nil {
		return nil, fmt.Errorf("store: finished books: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]bool, 32)
	for rows.Next() {
		var bookID int64
		if err := rows.Scan(&bookID); err != nil {
			return nil, fmt.Errorf("store: scan finished book: %w", err)
		}
		out[bookID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: finished books rows: %w", err)
	}
	return out, nil
}
