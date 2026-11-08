package store

import (
	"context"
	"database/sql"
	"fmt"
)

// 0027 的全部内容：enrich_changes（元数据补全的变更日志）已在 0027_enrich_changes.sql 建好，
// 本步骤只补 SQL 文件做不到的两件事：确认 books 具备回滚要写的列，缺列时整步跳过。
//
// 为什么缺列就跳过而不是报错：老库（v1）的 books 没有 category_l1/category_l2，
// 报错会让整次升级卡死；跳过则 CLI 的 enrich 子命令在启动时按「表不存在」拒绝执行，
// 用户看到的是明确的「先升级到支持分类的版本」，而不是迁移失败。
func createEnrichChangesStep(ctx context.Context, tx *sql.Tx, _ *Store) error {
	ok, err := booksReadyForEnrich(ctx, tx)
	if err != nil || !ok {
		return err
	}
	return nil
}

// booksReadyForEnrich 判断 books 是否具备补全链路要读写的列。
func booksReadyForEnrich(ctx context.Context, tx *sql.Tx) (bool, error) {
	for _, col := range []string{"author_id", "intro", "category_l1", "category_l2"} {
		has, err := columnExistsInTx(ctx, tx, "books", col)
		if err != nil || !has {
			return false, err
		}
	}
	return true, nil
}

// EnrichChange 是一条字段改动（与 enrich_changes 表一行对应）。
type EnrichChange struct {
	BookID     int64
	Field      string // author / intro / category / chapter_split
	OldValue   string
	NewValue   string
	SourceID   string
	Confidence float64
}

// 变更字段的合法取值（与 0027 的 CHECK 约束一致）。
const (
	EnrichFieldAuthor       = "author"
	EnrichFieldIntro        = "intro"
	EnrichFieldCategory     = "category"
	EnrichFieldChapterSplit = "chapter_split"
)

// RecordEnrichChanges 在一个事务里追加一批变更记录。
//
// rows 为空是合法调用（本次没有改动），直接返回 nil，不产生事务。
func (s *Store) RecordEnrichChanges(ctx context.Context, batchID string, rows []EnrichChange) error {
	if len(rows) == 0 {
		return nil
	}
	if batchID == "" {
		return fmt.Errorf("%w: batchID 不能为空", ErrInvalidArgument)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: record enrich changes begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO enrich_changes(batch_id, book_id, field, old_value, new_value, source_id, confidence)"+
			" VALUES(?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("store: record enrich changes prepare: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄
	for _, r := range rows {
		if r.Field == "" {
			return fmt.Errorf("%w: 变更字段不能为空", ErrInvalidArgument)
		}
		if _, err := stmt.ExecContext(ctx, batchID, r.BookID, r.Field, r.OldValue, r.NewValue, r.SourceID, r.Confidence); err != nil {
			return fmt.Errorf("store: record enrich change book=%d field=%s: %w", r.BookID, r.Field, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: record enrich changes commit: %w", err)
	}
	return nil
}

// EnrichBatchChanges 按批次取回全部变更（回滚与报告共用）。
func (s *Store) EnrichBatchChanges(ctx context.Context, batchID string) ([]EnrichChange, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT book_id, field, old_value, new_value, source_id, confidence FROM enrich_changes"+
			" WHERE batch_id = ? ORDER BY id", batchID)
	if err != nil {
		return nil, fmt.Errorf("store: list enrich changes: %w", err)
	}
	defer rows.Close()
	out := make([]EnrichChange, 0, 64)
	for rows.Next() {
		var c EnrichChange
		if err := rows.Scan(&c.BookID, &c.Field, &c.OldValue, &c.NewValue, &c.SourceID, &c.Confidence); err != nil {
			return nil, fmt.Errorf("store: scan enrich change: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: enrich changes rows: %w", err)
	}
	return out, nil
}
