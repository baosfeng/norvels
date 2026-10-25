package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
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

// ---------- 标签 CRUD（P3 §管理侧补全） ----------
//
// 标签是扁平分类：名字全库唯一（0001 的 tags.name UNIQUE），改名只动 tags.name，
// 书与标签的关联（book_tags）不受影响；删除标签要全库解绑，而不是留下悬空关联。

// ListTagsWithCount 是 P3 契约里的显式入口，口径与既有的 ListTags 完全一致
// （按书籍数倒序、同名升序），实现只保留一份，避免两个 SELECT 漂移。
func (s *Store) ListTagsWithCount(ctx context.Context) ([]TagSummary, error) {
	return s.ListTags(ctx)
}

// CreateTag 新建标签；同名（去首尾空白后）返回已有标签且 Created=false。
//
// 契约把「同名」当成功（POST /api/tags 201/200）：标签是分类词，重复点一次
// 不该报错；调用方按返回的 Created 决定状态码。
func (s *Store) CreateTag(ctx context.Context, name string) (TagSummary, error) {
	clean, err := cleanTagName(name)
	if err != nil {
		return TagSummary{}, err
	}
	out := TagSummary{Name: clean}
	err = s.inTx(ctx, "create tag", func(tx *sql.Tx) error {
		existing, ok, err := findTagInTx(ctx, tx, clean)
		if err != nil {
			return err
		}
		if ok {
			out = existing
			return nil
		}
		return insertTagInTx(ctx, tx, clean, &out)
	})
	if err != nil {
		return TagSummary{}, err
	}
	return out, nil
}

// insertTagInTx 插入标签并把 id 写进 out；并发撞 UNIQUE 时回读已有标签。
//
// 查重与插入之间没有锁：另一个连接同时建同名标签时，唯一索引是兜底，
// 失败方要拿到「已存在的那一行」而不是 500（与 volumeTitleConflict 同一取舍）。
func insertTagInTx(ctx context.Context, tx *sql.Tx, name string, out *TagSummary) error {
	res, err := tx.ExecContext(ctx, "INSERT INTO tags(name) VALUES (?)", name)
	if err != nil {
		if !isTagNameConflict(err) {
			return fmt.Errorf("store: create tag %q: %w", name, err)
		}
		existing, ok, ferr := findTagInTx(ctx, tx, name)
		if ferr != nil {
			return ferr
		}
		if !ok {
			return fmt.Errorf("store: create tag %q: %w", name, err)
		}
		*out = existing
		return nil
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("store: tag id: %w", err)
	}
	out.ID, out.Created = id, true
	return nil
}

// RenameTag 改名；重名 ErrConflict（409），标签不存在 ErrNotFound（404）。
func (s *Store) RenameTag(ctx context.Context, id int64, name string) error {
	if id <= 0 {
		return fmt.Errorf("%w: 标签 id %d", ErrInvalidArgument, id)
	}
	clean, err := cleanTagName(name)
	if err != nil {
		return err
	}
	return s.inTx(ctx, "rename tag", func(tx *sql.Tx) error {
		if err := requireTagInTx(ctx, tx, id); err != nil {
			return err
		}
		if err := ensureTagNameFreeInTx(ctx, tx, clean, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE tags SET name = ? WHERE id = ?", clean, id); err != nil {
			if isTagNameConflict(err) {
				return fmt.Errorf("%w: 标签名 %q 已存在", ErrConflict, clean)
			}
			return fmt.Errorf("store: rename tag %d: %w", id, err)
		}
		return nil
	})
}

// DeleteTag 删除标签并全库解绑（书本身不动）。
//
// 显式删 book_tags 而不是只靠 ON DELETE CASCADE：删除不可逆，
// 「删一个标签到底动了哪些表」应该在这条写路径上直接可读（与 delete.go 同一取舍）。
func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: 标签 id %d", ErrInvalidArgument, id)
	}
	return s.inTx(ctx, "delete tag", func(tx *sql.Tx) error {
		if err := requireTagInTx(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM book_tags WHERE tag_id = ?", id); err != nil {
			return fmt.Errorf("store: unbind tag %d: %w", id, err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM tags WHERE id = ?", id); err != nil {
			return fmt.Errorf("store: delete tag %d: %w", id, err)
		}
		return nil
	})
}

// cleanTagName 归一化并校验标签名：去首尾空白后非空、不超上限（不静默截断）。
//
// 与 normalizeTags（导入/关联路径）的差别：那里对超长名截断后照写，
// 因为自动标签不该让整本书导入失败；这里是人手输入，超长应当报错让他改。
func cleanTagName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("%w: 标签名不能为空", ErrInvalidArgument)
	}
	if utf8.RuneCountInString(name) > maxTagRunes {
		return "", fmt.Errorf("%w: 标签名超过 %d 字", ErrInvalidArgument, maxTagRunes)
	}
	return name, nil
}

// findTagInTx 按名字找标签（含书籍数）；未命中返回 ok=false。
func findTagInTx(ctx context.Context, tx *sql.Tx, name string) (TagSummary, bool, error) {
	var t TagSummary
	err := tx.QueryRowContext(ctx,
		"SELECT t.id, t.name, (SELECT COUNT(*) FROM book_tags bt WHERE bt.tag_id = t.id) "+
			"FROM tags t WHERE t.name = ?", name).Scan(&t.ID, &t.Name, &t.BookCount)
	if errors.Is(err, sql.ErrNoRows) {
		return TagSummary{}, false, nil
	}
	if err != nil {
		return TagSummary{}, false, fmt.Errorf("store: find tag %q: %w", name, err)
	}
	return t, true, nil
}

// requireTagInTx 确认标签存在；不存在返回 ErrNotFound。
func requireTagInTx(ctx context.Context, tx *sql.Tx, id int64) error {
	var got int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM tags WHERE id = ?", id).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: 标签 %d", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("store: check tag %d: %w", id, err)
	}
	return nil
}

// ensureTagNameFreeInTx 事务内查重；excludeID 用于改名时跳过自己。
func ensureTagNameFreeInTx(ctx context.Context, tx *sql.Tx, name string, excludeID int64) error {
	owner, ok, err := findTagInTx(ctx, tx, name)
	if err != nil {
		return err
	}
	if ok && owner.ID != excludeID {
		return fmt.Errorf("%w: 标签名 %q 已存在", ErrConflict, name)
	}
	return nil
}

// isTagNameConflict 判断是否撞了 tags.name 的唯一索引（并发兜底用）。
func isTagNameConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: tags.name")
}
