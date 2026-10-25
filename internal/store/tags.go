package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// maxTagRunes 是单个标签名的字符上限：标签是展示字段，超长名字只会在 UI 里撑破布局。
const maxTagRunes = 64

// SetBookTags 全量替换一本书的标签。
//
// 归一化口径：逐项 trim、丢弃空名、按出现顺序去重；重名不报错。全量替换意味着
// 传空列表就等于清空。替换后顺手删掉不再被任何书引用的孤儿标签，避免 /api/tags
// 里堆出一批 bookCount=0 的历史分类。
func (s *Store) SetBookTags(ctx context.Context, bookID int64, names []string) error {
	if bookID <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set book tags begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作

	if err := setBookTagsInTx(ctx, tx, bookID, names); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set book tags commit: %w", err)
	}
	return nil
}

// ListBookTags 返回一本书的标签名（按名称升序，保证前端渲染稳定）。
func (s *Store) ListBookTags(ctx context.Context, bookID int64) ([]string, error) {
	if bookID <= 0 {
		return nil, fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT t.name FROM book_tags bt JOIN tags t ON t.id = bt.tag_id WHERE bt.book_id = ? ORDER BY t.name",
		bookID)
	if err != nil {
		return nil, fmt.Errorf("store: list book tags: %w", err)
	}
	defer rows.Close()

	names := make([]string, 0, 8)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("store: scan book tag: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list book tags rows: %w", err)
	}
	return names, nil
}

// 标签来源（0012 起）：区分「用户手改」与「导入自动打」，
// 自动标签只增不删，绝不覆盖同书的手动标签。
const (
	TagSourceManual = ""     // 手动 / 历史存量
	TagSourceAuto   = "auto" // 词表命中
	TagSourceEPUB   = "epub" // EPUB dc:subject
)

// AddBookTagsWithSource 在事务内**只增不删**地打标签（导入链路用）。
//
// 与 setBookTagsInTx 的区别：不 DELETE 既有标签。原因：自动标签（词表命中 / dc:subject）
// 不能覆盖用户手加的同名标签，覆盖导入也不该清空旧标签；已有同名标签（任意 source）
// 不重复插入，其 source 保持不变。
func addBookTagsWithSourceInTx(ctx context.Context, tx *sql.Tx, bookID int64, names []string, source string) error {
	if err := ensureBookExistsInTx(ctx, tx, bookID); err != nil {
		return err
	}
	for _, name := range normalizeTags(names) {
		if err := linkTagWithSourceInTx(ctx, tx, bookID, name, source); err != nil {
			return err
		}
	}
	return nil
}

// setBookTagsInTx 在事务内全量替换标签；书不存在返回 ErrNotFound。
func setBookTagsInTx(ctx context.Context, tx *sql.Tx, bookID int64, names []string) error {
	if err := ensureBookExistsInTx(ctx, tx, bookID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_tags WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: clear book tags %d: %w", bookID, err)
	}
	for _, name := range normalizeTags(names) {
		if err := linkTagInTx(ctx, tx, bookID, name); err != nil {
			return err
		}
	}
	return pruneOrphanTagsInTx(ctx, tx)
}

// normalizeTags 归一化标签名：trim、去空、按出现顺序去重、按字符数截断。
func normalizeTags(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, raw := range names {
		name := truncateRunes(strings.TrimSpace(raw), maxTagRunes)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// linkTagInTx 建立「书 → 标签」关联（手动来源，source 保持空串）。
func linkTagInTx(ctx context.Context, tx *sql.Tx, bookID int64, name string) error {
	return linkTagWithSourceInTx(ctx, tx, bookID, name, TagSourceManual)
}

// linkTagWithSourceInTx 建立「书 → 标签」关联并记录来源：标签不存在就先建（tags.name 唯一）。
//
// INSERT OR IGNORE 保证同名标签不会被重复插入，也就不会改写已有行的 source——
// 这正是「手动标签不被自动标签覆盖」的落点。
func linkTagWithSourceInTx(ctx context.Context, tx *sql.Tx, bookID int64, name, source string) error {
	if _, err := tx.ExecContext(ctx, "INSERT INTO tags(name) VALUES (?) ON CONFLICT(name) DO NOTHING", name); err != nil {
		return fmt.Errorf("store: ensure tag %q: %w", name, err)
	}
	var tagID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM tags WHERE name = ?", name).Scan(&tagID); err != nil {
		return fmt.Errorf("store: select tag %q: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT OR IGNORE INTO book_tags(book_id, tag_id, source) VALUES (?, ?, ?)",
		bookID, tagID, source); err != nil {
		return fmt.Errorf("store: link tag %q: %w", name, err)
	}
	return nil
}

// pruneOrphanTagsInTx 删除不再被任何书引用的标签（book_tags 有 ON DELETE CASCADE，方向相反不成立）。
func pruneOrphanTagsInTx(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM tags WHERE NOT EXISTS (SELECT 1 FROM book_tags bt WHERE bt.tag_id = tags.id)"); err != nil {
		return fmt.Errorf("store: prune orphan tags: %w", err)
	}
	return nil
}
