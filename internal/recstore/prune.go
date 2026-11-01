package recstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// events 明细的保留窗口（设计 §2 的资源护栏）。
//
// 为什么明细要过期而聚合不用：events 是「兴趣权重怎么来的」的账本，只对最近
// 一两个月的兴趣有意义；聚合表（book_stats / book_daily / user_interest / cooccur）
// 是固定行数的汇总，不随事件数增长。没有窗口的事件表每天几百行、一年就是十几 MB
// 且永不回落——「资源曲线平稳」这条就不成立了。
//
// 90 天不是拍脑袋：兴趣权重按 14 天衰减，90 天 ≈ 6 个半衰期（权重剩约 1.2%），
// 更早的明细对排序已无影响，只对「考古」有价值——那属于备份的职责。
const retentionDays = 90

// Prune 删除 before 之前的 events 明细与 recent_items 曝光记录，返回删除的事件行数。
//
// 两个删除在同一事务里：明细与曝光记录是同一条时间线，只清一半会让
// 「近期已曝光」比事件表活得更久，出现「早就没有记录的书还被当成刚推过」。
func (s *Store) Prune(ctx context.Context, before time.Time) (int, error) {
	if before.IsZero() {
		// 零值时间会被格式化成 0001-01-01，等价于「清空全表」：
		// 定期清理的调用方一旦漏传参数就会静默抹掉全部历史，这里直接拒绝。
		return 0, fmt.Errorf("%w: 清理时间不能为零值", ErrInvalidArgument)
	}
	cut := formatTime(before)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("recstore: 清理事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	deleted, err := deleteBefore(ctx, tx, "events", cut)
	if err != nil {
		return 0, err
	}
	if _, err := deleteBefore(ctx, tx, "recent_items", cut); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("recstore: 提交清理: %w", err)
	}
	return deleted, nil
}

// PruneExpired 按内置保留窗口清理（now − retentionDays）。
//
// 窗口只保留在这一个地方：调用方各自算「现在减 90 天」，写错一天就是
// 「多删/少删一整天」，而这种偏差在线上几乎察觉不到。
func (s *Store) PruneExpired(ctx context.Context, now time.Time) (int, error) {
	return s.Prune(ctx, now.AddDate(0, 0, -retentionDays))
}

// deleteBefore 删除某张表里早于 cut 的行。
//
// table 只来自本文件的常量字面量，不存在注入面；集中成一个函数是为了让
// 「两张表同一个时间口径」这件事只有一个实现。
func deleteBefore(ctx context.Context, tx *sql.Tx, table, cut string) (int, error) {
	res, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE ts < ?", cut)
	if err != nil {
		return 0, fmt.Errorf("recstore: 清理 %s: %w", table, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("recstore: 清理 %s 行数: %w", table, err)
	}
	return int(n), nil
}
