package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// unknownAuthor 是作者缺失时的统一占位名。
const unknownAuthor = "佚名"

// bookSummaryColumns 是 BookSummary 的通用取数列（别名 b=books、a=authors）。
//
// 末段 8 列是 0008 加的书籍身份（language…source_format）与 has_cover：
// 列表与详情共用同一份口径，避免两个 SELECT 漂移。
const bookSummaryColumns = "b.id, b.title, b.author_id, COALESCE(a.name, ''), " +
	"b.chapter_count, b.word_count, b.volume_count, b.intro, b.status, b.serial_status, " +
	"b.read_status, b.imported_at, b.updated_at, " +
	"b.language, b.publisher, b.isbn, b.pub_date, b.series, b.series_index, b.has_cover, b.source_format, " +
	"b.rating, b.new_chapters"

// rowScanner 抽象 *sql.Row / *sql.Rows 的 Scan。
type rowScanner interface {
	Scan(dest ...any) error
}

// summaryScanTargets 按 bookSummaryColumns 的顺序返回扫描目标。
//
// 抽成函数是因为详情读取要在同一份列后面追加自己的列：两处共用一套目标，
// 以后加字段只改一处（19 个字段手抄两遍，迟早漏一个）。
func summaryScanTargets(bs *BookSummary, imported, updated *string) []any {
	return []any{
		&bs.ID, &bs.Title, &bs.AuthorID, &bs.AuthorName, &bs.ChapterCount, &bs.WordCount,
		&bs.VolumeCount, &bs.Intro, &bs.Status, &bs.SerialStatus, &bs.ReadStatus, imported, updated,
		&bs.Language, &bs.Publisher, &bs.ISBN, &bs.PubDate, &bs.Series, &bs.SeriesIndex,
		&bs.HasCover, &bs.Format, &bs.Rating, &bs.NewChapters,
	}
}

// scanBookSummary 把一行扫描成 BookSummary（时间列为文本，解析失败时留零值）。
func scanBookSummary(sc rowScanner) (BookSummary, error) {
	var (
		bs       BookSummary
		imported string
		updated  string
	)
	if err := sc.Scan(summaryScanTargets(&bs, &imported, &updated)...); err != nil {
		return bs, err
	}
	applySummaryTimes(&bs, imported, updated)
	return bs, nil
}

// applySummaryTimes 回填两条时间列（解析失败保持零值，不让一条脏数据废掉整页）。
func applySummaryTimes(bs *BookSummary, imported, updated string) {
	bs.ImportedAt, _ = parseTime(imported)
	bs.UpdatedAt, _ = parseTime(updated)
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

// ---------- 元数据编辑（0008 起） ----------

// BookMetaPatch 是一次书籍元数据更新：nil 字段表示「这一项不改」。
//
// 用指针而不是零值判断「有没有提到这个字段」：显式把简介/系列清空与根本没传
// 这两个字段语义完全不同，前者要写空串，后者不能动库。
type BookMetaPatch struct {
	Title *string
	// Author 是单值作者；给出时等价于 Authors=[]string{Author}。
	Author *string
	// Authors 是有序作者（全量替换）：写 books_authors 并把 books.author_id 同步成 [0]。
	// 与 Author 同时给出时 Authors 优先（前端可能同时持有两个字段，不该因此报错）。
	Authors     *[]string
	Intro       *string
	Series      *string
	SeriesIndex *int
	Tags        *[]string
	// SerialStatus 是连载状态（''|ongoing|completed），与解析状态 Status 无关。
	SerialStatus *string
	// ReadStatus 是阅读状态（''|want|reading|done），与解析状态/连载状态互不覆写。
	ReadStatus *string
	// Rating 是评分 0–5（0 = 未评）；越界 ErrInvalidArgument。
	Rating *int
	// 出版信息（P3 管理侧补全）：给出即全量写入，空串表示清空。
	Publisher *string
	PubDate   *string // 只接受 YYYY-MM-DD 或空串
	ISBN      *string
	Language  *string
}

// IsEmpty 报告 patch 是否什么都没包含（空 patch 不写库）。
func (p BookMetaPatch) IsEmpty() bool {
	return p.Title == nil && p.Author == nil && p.Authors == nil && p.Intro == nil &&
		p.Series == nil && p.SeriesIndex == nil && p.Tags == nil &&
		p.SerialStatus == nil && p.ReadStatus == nil && p.Rating == nil &&
		p.Publisher == nil && p.PubDate == nil && p.ISBN == nil && p.Language == nil
}

// authorNames 返回本次补丁要落库的有序作者；nil 表示「作者这一项不改」。
//
// 空名单的处置分两种：显式 Authors 传空由 saveBookAuthorsInTx 报 ErrInvalidArgument
// （api 层先拦成 400）；单值 Author 传空串沿用导入口径落「佚名」，避免 author_id=0。
func (p BookMetaPatch) authorNames() []string {
	if p.Authors != nil {
		return NormalizeAuthors(*p.Authors)
	}
	if p.Author == nil {
		return nil
	}
	names := NormalizeAuthors([]string{*p.Author})
	if len(names) == 0 {
		return []string{unknownAuthor}
	}
	return names
}

// UpdateBookMeta 按 patch 更新书名/作者/简介/系列/系列序号/标签。
//
// 三条不变量：
//   - 空 patch 不产生任何写入（也不刷新 updated_at）；
//   - 作者变更时新旧作者的聚合列都重算（否则 /api/authors 的作品数会漂）；
//   - 标签是全量替换（传空列表即清空），与 PATCH 契约一致。
func (s *Store) UpdateBookMeta(ctx context.Context, id int64, patch BookMetaPatch) error {
	if id <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, id)
	}
	if patch.IsEmpty() {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: update book meta begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作

	if err := updateBookMetaInTx(ctx, tx, id, patch); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: update book meta commit: %w", err)
	}
	return nil
}

// updateBookMetaInTx 是 UpdateBookMeta 的事务体：作者 → 标量列 → 标签 → 作者聚合。
func updateBookMetaInTx(ctx context.Context, tx *sql.Tx, id int64, patch BookMetaPatch) error {
	oldAuthorID, err := bookAuthorIDInTx(ctx, tx, id)
	if err != nil {
		return err
	}
	newAuthorID := oldAuthorID
	if names := patch.authorNames(); names != nil {
		if newAuthorID, err = saveBookAuthorsInTx(ctx, tx, id, names); err != nil {
			return err
		}
	}
	if err := applyColumnPatch(ctx, tx, id, patch); err != nil {
		return err
	}
	if patch.Tags != nil {
		if err := setBookTagsInTx(ctx, tx, id, *patch.Tags); err != nil {
			return err
		}
	}
	return refreshBookAuthors(ctx, tx, newAuthorID, oldAuthorID)
}

// bookAuthorIDInTx 读当前作者 id；书不存在返回 ErrNotFound。
func bookAuthorIDInTx(ctx context.Context, tx *sql.Tx, id int64) (int64, error) {
	var authorID int64
	err := tx.QueryRowContext(ctx, "SELECT author_id FROM books WHERE id = ?", id).Scan(&authorID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: book %d", ErrNotFound, id)
	}
	if err != nil {
		return 0, fmt.Errorf("store: book author lookup %d: %w", id, err)
	}
	return authorID, nil
}

// applyColumnPatch 写 books 上的标量列（文本/状态/出版信息/评分）并刷新 updated_at。
//
// 具体拼 SET 的逻辑按字段类别拆到多个 helper：加一类字段只改一处，
// 不会把任何单个函数推过复杂度阈值。
func applyColumnPatch(ctx context.Context, tx *sql.Tx, id int64, patch BookMetaPatch) error {
	sets, args, err := buildColumnPatchSets(patch)
	if err != nil {
		return err
	}
	args = append(args, id)
	if _, err := tx.ExecContext(ctx, "UPDATE books SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil {
		return fmt.Errorf("store: update book meta columns %d: %w", id, err)
	}
	return nil
}

// buildColumnPatchSets 把 patch 编译成 SET 片段与参数，并在拼接时完成全部标量校验。
//
// 抽出来是为了让批量入口在开事务前先验一遍：单本 PATCH 与批量 PATCH 共用这一份
// 校验，错误文案一致，不会出现「批量时每本书各失败一次、单本时直接 400」的口径漂移。
func buildColumnPatchSets(patch BookMetaPatch) ([]string, []any, error) {
	sets := []string{"updated_at = ?"}
	args := []any{formatTime(nowUTC())}
	var err error
	if sets, args, err = textColumnSets(sets, args, patch); err != nil {
		return nil, nil, err
	}
	if sets, args, err = identityColumnSets(sets, args, patch); err != nil {
		return nil, nil, err
	}
	if sets, args, err = statusColumnSets(sets, args, patch); err != nil {
		return nil, nil, err
	}
	return ratingColumnSet(sets, args, patch)
}

// 出版信息四列的写入上限：展示型字段，超长只会在详情页里撑破布局。
const (
	maxPublisherRunes = 120
	maxISBNRunes      = 32
	maxLanguageRunes  = 32
)

// identityColumnSets 拼接出版信息四列：publisher / isbn / language 截断即写入。
func identityColumnSets(sets []string, args []any, patch BookMetaPatch) ([]string, []any, error) {
	cols := []struct {
		column string
		value  *string
		limit  int
	}{
		{"publisher", patch.Publisher, maxPublisherRunes},
		{"isbn", patch.ISBN, maxISBNRunes},
		{"language", patch.Language, maxLanguageRunes},
	}
	for _, c := range cols {
		if c.value == nil {
			continue
		}
		sets, args = append(sets, c.column+" = ?"),
			append(args, truncateRunes(strings.TrimSpace(*c.value), c.limit))
	}
	if patch.PubDate == nil {
		return sets, args, nil
	}
	date, err := normalizePubDate(*patch.PubDate)
	if err != nil {
		return nil, nil, err
	}
	return append(sets, "pub_date = ?"), append(args, date), nil
}

// normalizePubDate 校验出版日期：空串合法（清空），其余必须是 YYYY-MM-DD。
func normalizePubDate(raw string) (string, error) {
	date := strings.TrimSpace(raw)
	if date == "" {
		return "", nil
	}
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", fmt.Errorf("%w: pubDate %q 不是 YYYY-MM-DD", ErrInvalidArgument, date)
	}
	return parsed.Format("2006-01-02"), nil
}

// ratingColumnSet 拼接评分列：0 = 未评，越界即 ErrInvalidArgument（不静默截断）。
func ratingColumnSet(sets []string, args []any, patch BookMetaPatch) ([]string, []any, error) {
	if patch.Rating == nil {
		return sets, args, nil
	}
	if *patch.Rating < 0 || *patch.Rating > maxBookRating {
		return nil, nil, fmt.Errorf("%w: rating %d 必须在 0~%d 之间",
			ErrInvalidArgument, *patch.Rating, maxBookRating)
	}
	return append(sets, "rating = ?"), append(args, *patch.Rating), nil
}

// textColumnSets 把文本类标量列拼进 SET 片段（title/intro/series/seriesIndex）。
func textColumnSets(sets []string, args []any, patch BookMetaPatch) ([]string, []any, error) {
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return nil, nil, fmt.Errorf("%w: empty title", ErrInvalidArgument)
		}
		sets, args = append(sets, "title = ?"), append(args, title)
	}
	if patch.Intro != nil {
		sets, args = append(sets, "intro = ?"), append(args, truncateRunes(strings.TrimSpace(*patch.Intro), 500))
	}
	if patch.Series != nil {
		sets, args = append(sets, "series = ?"), append(args, truncateRunes(strings.TrimSpace(*patch.Series), 120))
	}
	if patch.SeriesIndex != nil {
		sets, args = append(sets, "series_index = ?"), append(args, maxInt(*patch.SeriesIndex, 0))
	}
	return sets, args, nil
}

// statusColumnSets 校验并拼接枚举类状态列（serial_status / read_status）。
//
// 表驱动而不是写两遍 if：两组状态的校验口径完全一样（去空白 → 白名单 → 非法即 400），
// 以后再加同类状态列只往表里添一行。
func statusColumnSets(sets []string, args []any, patch BookMetaPatch) ([]string, []any, error) {
	cols := []struct {
		column string
		value  *string
		valid  func(string) bool
		label  string
	}{
		{"serial_status", patch.SerialStatus, ValidSerialStatus, "serial status"},
		{"read_status", patch.ReadStatus, ValidReadStatus, "read status"},
	}
	for _, c := range cols {
		if c.value == nil {
			continue
		}
		v := strings.TrimSpace(*c.value)
		if !c.valid(v) {
			return nil, nil, fmt.Errorf("%w: %s %q", ErrInvalidArgument, c.label, v)
		}
		sets, args = append(sets, c.column+" = ?"), append(args, v)
	}
	return sets, args, nil
}

// ensureBookExistsInTx 是写子表（book_covers/book_tags）前的存在性守卫：
// 子表外键只保证「book_id 有效」，调用方需要的是「这本书在不在」这个明确答案。
func ensureBookExistsInTx(ctx context.Context, tx *sql.Tx, bookID int64) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM books WHERE id = ?)", bookID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check book %d: %w", bookID, err)
	}
	if !exists {
		return fmt.Errorf("%w: book %d", ErrNotFound, bookID)
	}
	return nil
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
