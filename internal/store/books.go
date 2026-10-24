package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"norvels/internal/model"
)

// unknownAuthor 是作者缺失时的统一占位名。
const unknownAuthor = "佚名"

// bookSummaryColumns 是 BookSummary 的通用取数列（别名 b=books、a=authors）。
const bookSummaryColumns = "b.id, b.title, b.author_id, COALESCE(a.name, ''), " +
	"b.chapter_count, b.word_count, b.volume_count, b.intro, b.status, b.imported_at, b.updated_at"

// rowScanner 抽象 *sql.Row / *sql.Rows 的 Scan。
type rowScanner interface {
	Scan(dest ...any) error
}

// scanBookSummary 把一行扫描成 BookSummary（时间列为文本，解析失败时留零值）。
func scanBookSummary(sc rowScanner) (BookSummary, error) {
	var (
		bs       BookSummary
		imported string
		updated  string
	)
	err := sc.Scan(&bs.ID, &bs.Title, &bs.AuthorID, &bs.AuthorName, &bs.ChapterCount, &bs.WordCount,
		&bs.VolumeCount, &bs.Intro, &bs.Status, &imported, &updated)
	if err != nil {
		return bs, err
	}
	bs.ImportedAt, _ = parseTime(imported)
	bs.UpdatedAt, _ = parseTime(updated)
	return bs, nil
}

// SaveBook 幂等写入一本书及其卷、章节，并同步维护 FTS 索引。返回 BookID。
//
// 幂等语义：source_path 或 file_sha256 命中既有记录时返回 Skipped；
// opt.Force 为 true 时保留原 id 覆盖更新（先删旧卷/章节，触发器同步清理 FTS）。
func (s *Store) SaveBook(ctx context.Context, b *model.ParsedBook, opt SaveOptions) (SaveResult, error) {
	if b == nil {
		return SaveResult{}, fmt.Errorf("%w: nil book", ErrInvalidArgument)
	}
	sourcePath := strings.TrimSpace(b.SourcePath)
	if sourcePath == "" {
		return SaveResult{}, fmt.Errorf("%w: empty source path", ErrInvalidArgument)
	}
	sha := strings.ToLower(strings.TrimSpace(b.SHA256))
	if sha == "" {
		return SaveResult{}, fmt.Errorf("%w: empty sha256", ErrInvalidArgument)
	}

	title := strings.TrimSpace(b.Title)
	if title == "" {
		title = strings.TrimSpace(b.SourceName)
	}
	if title == "" {
		title = "未命名"
	}
	sourceName := strings.TrimSpace(b.SourceName)
	if sourceName == "" {
		sourceName = filepath.Base(sourcePath)
	}
	author := strings.TrimSpace(b.Author)
	if author == "" {
		// 解析器抽不到作者时统一落「佚名」，避免 author_id=0 导致按作者筛选丢书。
		author = unknownAuthor
	}
	intro := truncateRunes(strings.TrimSpace(b.Intro), 500)

	wordCount := b.WordCount
	if wordCount <= 0 {
		wordCount = 0
		for i := range b.Chapters {
			wc := b.Chapters[i].WordCount
			if wc <= 0 {
				wc = countWords(b.Chapters[i].Content)
			}
			wordCount += wc
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SaveResult{}, fmt.Errorf("store: save book begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作

	var (
		existingID  int64
		oldAuthorID int64
	)
	err = tx.QueryRowContext(ctx,
		"SELECT id, author_id FROM books WHERE source_path = ? OR file_sha256 = ? ORDER BY id LIMIT 1",
		sourcePath, sha).Scan(&existingID, &oldAuthorID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		existingID, oldAuthorID = 0, 0
	case err != nil:
		return SaveResult{}, fmt.Errorf("store: save book lookup: %w", err)
	}
	if existingID != 0 && !opt.Force {
		return SaveResult{BookID: existingID, Skipped: true}, nil
	}

	authorID, err := ensureAuthor(ctx, tx, author)
	if err != nil {
		return SaveResult{}, err
	}

	usedVols := b.VolumeIdxSet()
	sort.Ints(usedVols)
	volMap := make(map[int]int64, len(usedVols))

	nowText := formatTime(nowUTC())
	chapterCount := len(b.Chapters)
	volumeCount := len(usedVols)

	var bookID int64
	updated := existingID != 0
	if !updated {
		res, err := tx.ExecContext(ctx,
			"INSERT INTO books(title, author_id, source_path, source_name, file_size, file_sha256, "+
				"encoding, word_count, chapter_count, volume_count, intro, status, multi_book, imported_at, updated_at) "+
				"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			title, authorID, sourcePath, sourceName, b.FileSize, sha, b.Encoding,
			wordCount, chapterCount, volumeCount, intro, "parsed", boolToInt(b.MultiBook), nowText, nowText)
		if err != nil {
			return SaveResult{}, fmt.Errorf("store: save book insert: %w", err)
		}
		if bookID, err = res.LastInsertId(); err != nil {
			return SaveResult{}, fmt.Errorf("store: save book id: %w", err)
		}
	} else {
		bookID = existingID
		// 先清旧章节（触发器同步删 FTS 行）再清旧卷。
		if _, err := tx.ExecContext(ctx, "DELETE FROM chapters WHERE book_id = ?", bookID); err != nil {
			return SaveResult{}, fmt.Errorf("store: save book clear chapters: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM volumes WHERE book_id = ?", bookID); err != nil {
			return SaveResult{}, fmt.Errorf("store: save book clear volumes: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE books SET title = ?, author_id = ?, source_path = ?, source_name = ?, file_size = ?, "+
				"file_sha256 = ?, encoding = ?, word_count = ?, chapter_count = ?, volume_count = ?, intro = ?, "+
				"status = ?, multi_book = ?, imported_at = ?, updated_at = ? WHERE id = ?",
			title, authorID, sourcePath, sourceName, b.FileSize, sha, b.Encoding,
			wordCount, chapterCount, volumeCount, intro, "parsed", boolToInt(b.MultiBook), nowText, nowText, bookID); err != nil {
			return SaveResult{}, fmt.Errorf("store: save book update: %w", err)
		}
	}

	for _, vi := range usedVols {
		if vi < 0 || vi >= len(b.Volumes) {
			continue
		}
		v := b.Volumes[vi]
		res, err := tx.ExecContext(ctx, "INSERT INTO volumes(book_id, idx, title) VALUES (?, ?, ?)",
			bookID, v.Idx, strings.TrimSpace(v.Title))
		if err != nil {
			return SaveResult{}, fmt.Errorf("store: save volume %d: %w", v.Idx, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return SaveResult{}, fmt.Errorf("store: save volume id: %w", err)
		}
		volMap[vi] = id
	}

	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO chapters(book_id, volume_id, idx, title, title_seg, content, content_seg, word_count) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return SaveResult{}, fmt.Errorf("store: save chapter prepare: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄

	for i := range b.Chapters {
		c := &b.Chapters[i]
		var volID int64
		if c.VolumeIdx >= 0 {
			volID = volMap[c.VolumeIdx]
		}
		wc := c.WordCount
		if wc <= 0 {
			wc = countWords(c.Content)
		}
		if _, err := stmt.ExecContext(ctx, bookID, volID, c.Idx, c.Title,
			Segment(c.Title), c.Content, Segment(c.Content), wc); err != nil {
			return SaveResult{}, fmt.Errorf("store: save chapter idx=%d: %w", c.Idx, err)
		}
	}

	if err := refreshAuthorStats(ctx, tx, authorID); err != nil {
		return SaveResult{}, err
	}
	if oldAuthorID != 0 && oldAuthorID != authorID {
		if err := refreshAuthorStats(ctx, tx, oldAuthorID); err != nil {
			return SaveResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return SaveResult{}, fmt.Errorf("store: save book commit: %w", err)
	}
	return SaveResult{BookID: bookID, Updated: updated}, nil
}

// ensureAuthor 按名字取作者 id，不存在则创建；空名返回 0（未知作者）。
func ensureAuthor(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	if name == "" {
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO authors(name) VALUES (?) ON CONFLICT(name) DO NOTHING", name); err != nil {
		return 0, fmt.Errorf("store: ensure author: %w", err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM authors WHERE name = ?", name).Scan(&id); err != nil {
		return 0, fmt.Errorf("store: select author: %w", err)
	}
	return id, nil
}

// refreshAuthorStats 重算单个作者的聚合列（books 表只有千级行，成本可忽略）。
func refreshAuthorStats(ctx context.Context, tx *sql.Tx, authorID int64) error {
	if authorID == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		"UPDATE authors SET "+
			"book_count = (SELECT COUNT(*) FROM books WHERE author_id = authors.id), "+
			"word_count = (SELECT COALESCE(SUM(word_count), 0) FROM books WHERE author_id = authors.id) "+
			"WHERE id = ?", authorID)
	if err != nil {
		return fmt.Errorf("store: refresh author stats: %w", err)
	}
	return nil
}

// BookExistsBySHA256 判断文件哈希是否已入库。
func (s *Store) BookExistsBySHA256(ctx context.Context, sha string) (bool, error) {
	sha = strings.ToLower(strings.TrimSpace(sha))
	if sha == "" {
		return false, nil
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM books WHERE file_sha256 = ?)", sha).Scan(&exists); err != nil {
		return false, fmt.Errorf("store: exists by sha: %w", err)
	}
	return exists, nil
}

// bookOrder 把排序键映射为 ORDER BY 子句。
func bookOrder(sortKey string) string {
	switch sortKey {
	case "title":
		return "b.title COLLATE NOCASE ASC, b.id ASC"
	case "chapters":
		return "b.chapter_count DESC, b.id DESC"
	case "words":
		return "b.word_count DESC, b.id DESC"
	case "imported":
		return "b.imported_at DESC, b.id DESC"
	default: // updated
		return "b.updated_at DESC, b.id DESC"
	}
}

// bookFilter 把 BookQuery 编译成 WHERE 片段与参数。
func bookFilter(q BookQuery) ([]string, []any) {
	var (
		where []string
		args  []any
	)
	if s := strings.TrimSpace(q.Q); s != "" {
		pat := "%" + escapeLike(s) + "%"
		where = append(where, "(b.title LIKE ? ESCAPE '!' OR a.name LIKE ? ESCAPE '!')")
		args = append(args, pat, pat)
	}
	if q.AuthorID != 0 {
		where = append(where, "b.author_id = ?")
		args = append(args, q.AuthorID)
	}
	if t := strings.TrimSpace(q.Tag); t != "" {
		where = append(where, "EXISTS (SELECT 1 FROM book_tags bt JOIN tags t ON t.id = bt.tag_id "+
			"WHERE bt.book_id = b.id AND t.name = ?)")
		args = append(args, t)
	}
	return where, args
}

func (s *Store) countBooks(ctx context.Context, where []string, args []any) (int, error) {
	q := "SELECT COUNT(*) FROM books b LEFT JOIN authors a ON a.id = b.author_id"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("store: count books: %w", err)
	}
	return total, nil
}

// loadBooks 执行列表查询；limit <= 0 表示不分页。
func (s *Store) loadBooks(ctx context.Context, where []string, args []any, order string, limit, offset int) ([]BookSummary, error) {
	q := "SELECT " + bookSummaryColumns + " FROM books b LEFT JOIN authors a ON a.id = b.author_id"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY " + order
	qargs := make([]any, 0, len(args)+2)
	qargs = append(qargs, args...)
	if limit > 0 {
		q += " LIMIT ? OFFSET ?"
		if offset < 0 {
			offset = 0
		}
		qargs = append(qargs, limit, offset)
	}
	rows, err := s.db.QueryContext(ctx, q, qargs...)
	if err != nil {
		return nil, fmt.Errorf("store: list books: %w", err)
	}
	defer rows.Close()

	items := make([]BookSummary, 0, 20)
	for rows.Next() {
		bs, err := scanBookSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan book: %w", err)
		}
		items = append(items, bs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list books rows: %w", err)
	}
	return items, nil
}

// ListBooks 分页列出书籍。
func (s *Store) ListBooks(ctx context.Context, q BookQuery) (Page[BookSummary], error) {
	page, size := normPage(q.Page, q.Size)
	where, args := bookFilter(q)
	total, err := s.countBooks(ctx, where, args)
	if err != nil {
		return Page[BookSummary]{}, err
	}
	items, err := s.loadBooks(ctx, where, args, bookOrder(q.Sort), size, (page-1)*size)
	if err != nil {
		return Page[BookSummary]{}, err
	}
	return Page[BookSummary]{Items: items, Total: total, Page: page, Size: size}, nil
}

// GetBook 返回书籍详情（含完整目录树）。未归卷章节包装成伪卷 id=0/idx=-1。
func (s *Store) GetBook(ctx context.Context, id int64) (*BookDetail, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: book id %d", ErrInvalidArgument, id)
	}
	row := s.db.QueryRowContext(ctx,
		"SELECT "+bookSummaryColumns+", b.encoding, b.source_name, b.file_size "+
			"FROM books b LEFT JOIN authors a ON a.id = b.author_id WHERE b.id = ?", id)

	var (
		bs       BookSummary
		imported string
		updated  string
		encoding string
		srcName  string
		fileSize int64
	)
	err := row.Scan(&bs.ID, &bs.Title, &bs.AuthorID, &bs.AuthorName, &bs.ChapterCount, &bs.WordCount,
		&bs.VolumeCount, &bs.Intro, &bs.Status, &imported, &updated, &encoding, &srcName, &fileSize)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: book %d", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get book: %w", err)
	}
	bs.ImportedAt, _ = parseTime(imported)
	bs.UpdatedAt, _ = parseTime(updated)

	detail := &BookDetail{
		BookSummary: bs,
		Encoding:    encoding,
		SourceName:  srcName,
		FileSize:    fileSize,
		Volumes:     make([]VolumeNode, 0, 8),
	}

	volRows, err := s.db.QueryContext(ctx,
		"SELECT id, idx, title FROM volumes WHERE book_id = ? ORDER BY idx, id", id)
	if err != nil {
		return nil, fmt.Errorf("store: get book volumes: %w", err)
	}
	volIndex := make(map[int64]int, 8)
	for volRows.Next() {
		var v VolumeNode
		if err := volRows.Scan(&v.ID, &v.Idx, &v.Title); err != nil {
			volRows.Close()
			return nil, fmt.Errorf("store: scan volume: %w", err)
		}
		v.Chapters = make([]ChapterBrief, 0, 32)
		volIndex[v.ID] = len(detail.Volumes)
		detail.Volumes = append(detail.Volumes, v)
	}
	if err := volRows.Err(); err != nil {
		volRows.Close()
		return nil, fmt.Errorf("store: get book volumes rows: %w", err)
	}
	volRows.Close()

	chRows, err := s.db.QueryContext(ctx,
		"SELECT id, idx, title, volume_id, word_count FROM chapters WHERE book_id = ? ORDER BY idx, id", id)
	if err != nil {
		return nil, fmt.Errorf("store: get book chapters: %w", err)
	}
	defer chRows.Close()

	loose := make([]ChapterBrief, 0, 8)
	for chRows.Next() {
		var c ChapterBrief
		if err := chRows.Scan(&c.ID, &c.Idx, &c.Title, &c.VolumeID, &c.WordCount); err != nil {
			return nil, fmt.Errorf("store: scan chapter brief: %w", err)
		}
		if idx, ok := volIndex[c.VolumeID]; ok {
			detail.Volumes[idx].Chapters = append(detail.Volumes[idx].Chapters, c)
			continue
		}
		loose = append(loose, c)
	}
	if err := chRows.Err(); err != nil {
		return nil, fmt.Errorf("store: get book chapters rows: %w", err)
	}
	if len(loose) > 0 {
		detail.Volumes = append(detail.Volumes, VolumeNode{
			ID:       0,
			Idx:      -1,
			Title:    "未分卷",
			Chapters: loose,
		})
	}
	return detail, nil
}

// truncateRunes 按字符数截断（用于简介等展示字段）。
func truncateRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
