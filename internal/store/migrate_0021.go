package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// 0021 的主库步骤：建 AI 审核表 + 给 books 补 superseded_by（设计 §6 / §7）。
//
// 为什么整段放 Go 而不写进迁移 SQL：SQLite 没有 ADD COLUMN IF NOT EXISTS，而
// 「删掉版本号重跑」是排查迁移问题的常规手段，撞 duplicate column 会让整次迁移失败；
// 建表与建索引本身是 IF NOT EXISTS，但加列必须先探测。两条都跑在 0021 自己的事务里，
// 失败整体回滚，不会留下「表建了、列没加」的半迁移库。
//
// 存量书 superseded_by = 0（= 当前版本）：0021 之前不存在「版本对」的概念，
// 所有存量书都是可见的当前版本，不回填、不猜。
func createImportReviewsStep(ctx context.Context, tx *sql.Tx) error {
	if err := addColumnIfMissing(ctx, tx, "books", "superseded_by", "superseded_by INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, importReviewsDDL); err != nil {
		return fmt.Errorf("create import_reviews: %w", err)
	}
	if _, err := tx.ExecContext(ctx, importReviewsIndexDDL); err != nil {
		return fmt.Errorf("create import_reviews index: %w", err)
	}
	log.Printf("迁移 0021：已建 import_reviews 表并给 books 补 superseded_by 列（存量书按当前版本处理）")
	return nil
}

// importReviewsDDL 是审核表结构，与 internal/store/reviews.go 的列清单一一对应。
//
//   - status：pending / ready / needs_manual / confirmed_keep / confirmed_replace / rolled_back；
//   - ai_winner：new / old / equal（空串 = 没有 AI 结论）；
//   - ai_reasons / rule_reasons：JSON 字符串数组（AI 依据 / 规则对比文案）；
//   - decided_at / decided_by：最后一次决策的时间与账号（空串 = 还没决策）。
const importReviewsDDL = `CREATE TABLE IF NOT EXISTS import_reviews (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	job_id INTEGER NOT NULL DEFAULT 0,
	new_book_id INTEGER NOT NULL DEFAULT 0,
	old_book_id INTEGER NOT NULL DEFAULT 0,
	hit_ratio REAL NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'pending',
	ai_winner TEXT NOT NULL DEFAULT '',
	ai_confidence REAL NOT NULL DEFAULT 0,
	ai_reasons TEXT NOT NULL DEFAULT '[]',
	rule_reasons TEXT NOT NULL DEFAULT '[]',
	created_at TEXT NOT NULL,
	decided_at TEXT NOT NULL DEFAULT '',
	decided_by TEXT NOT NULL DEFAULT ''
)`

// importReviewsIndexDDL 是「按状态取待办」的索引：worker 每轮只查 pending，
// 列表页按状态筛选，两个查询都走 (status, created_at)。
const importReviewsIndexDDL = "CREATE INDEX IF NOT EXISTS idx_import_reviews_status_created " +
	"ON import_reviews(status, created_at)"
