package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// 系列聚合（P3 §系列聚合页）。
//
// series 是 books 上的自由文本列（0008 起，EPUB 的丛书名或用户手填），
// 没有独立的 series 表：系列是一本书的属性，聚合按 TRIM(series) 分组即可，
// 不引入「系列实体」也就没有改名、合并、删除这一整套接口。
//
// 口径：只统计 TRIM(series) <> '' 的书；名字比较一律用 SQL 的 TRIM，
// 让老库里带首尾空格的存量值也能被正确分组，而 Go 侧不做第二次口径。

// SeriesSummary 是一个系列的聚合行。
type SeriesSummary struct {
	Name      string `json:"name"`
	BookCount int    `json:"bookCount"`
	WordCount int    `json:"wordCount"`
	// LastUpdatedAt 是系列内书籍 updated_at 的最大值（ISO-8601 文本）。
	LastUpdatedAt string `json:"lastUpdatedAt"`
	// CoverBookID 是有封面的书里 id 最大的一本；封面全缺时退化为 id 最小的书，
	// 前端据此渲染一张真实封面，而不是系列的合成图。
	CoverBookID int64 `json:"coverBookId"`
}

// BookBrief 是轻量列表里的一本书（系列详情、随机一本）：不含简介与目录。
type BookBrief struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	AuthorID     int64     `json:"authorId"`
	AuthorName   string    `json:"authorName"`
	ChapterCount int       `json:"chapterCount"`
	WordCount    int       `json:"wordCount"`
	HasCover     bool      `json:"hasCover"`
	Rating       int       `json:"rating"`
	NewChapters  int       `json:"newChapters"`
	ReadStatus   string    `json:"readStatus,omitempty"`
	Series       string    `json:"series,omitempty"`
	SeriesIndex  int       `json:"seriesIndex,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// ListSeries 分页列出系列，按最近更新倒序；q 非空时按系列名模糊匹配。
func (s *Store) ListSeries(ctx context.Context, q string, page, size int) ([]SeriesSummary, int, error) {
	page, size = normPage(page, size)
	where, args := seriesWhere(q)
	total, err := s.countSeries(ctx, where, args)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.loadSeries(ctx, where, args, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// seriesWhere 编译系列筛选：空系列永不出现在列表里。
func seriesWhere(q string) ([]string, []any) {
	where := []string{"TRIM(b.series) <> ''"}
	args := make([]any, 0, 1)
	if kw := strings.TrimSpace(q); kw != "" {
		where = append(where, "TRIM(b.series) LIKE ? ESCAPE '!'")
		args = append(args, "%"+escapeLike(kw)+"%")
	}
	return where, args
}

// countSeries 数「不同的系列名」而不是书：分页总数必须与列表行数同口径。
func (s *Store) countSeries(ctx context.Context, where []string, args []any) (int, error) {
	var total int
	q := "SELECT COUNT(DISTINCT TRIM(b.series)) FROM books b WHERE " + strings.Join(where, " AND ")
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("store: count series: %w", err)
	}
	return total, nil
}

// loadSeries 读取一页系列聚合；排序按最近更新倒序、同名兜底。
func (s *Store) loadSeries(ctx context.Context, where []string, args []any, limit, offset int) ([]SeriesSummary, error) {
	q := "SELECT TRIM(b.series), COUNT(*), COALESCE(SUM(b.word_count), 0), " +
		"COALESCE(MAX(b.updated_at), ''), " +
		"COALESCE(MAX(CASE WHEN b.has_cover = 1 THEN b.id END), MIN(b.id)) " +
		"FROM books b WHERE " + strings.Join(where, " AND ") +
		" GROUP BY TRIM(b.series) " +
		"ORDER BY COALESCE(MAX(b.updated_at), '') DESC, TRIM(b.series) ASC LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, q, append(args, limit, offset)...)
	if err != nil {
		return nil, fmt.Errorf("store: list series: %w", err)
	}
	defer rows.Close()

	items := make([]SeriesSummary, 0, limit)
	for rows.Next() {
		var it SeriesSummary
		if err := rows.Scan(&it.Name, &it.BookCount, &it.WordCount,
			&it.LastUpdatedAt, &it.CoverBookID); err != nil {
			return nil, fmt.Errorf("store: scan series: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list series rows: %w", err)
	}
	return items, nil
}

// bookBriefColumns 是 BookBrief 的取数列（别名 b=books、a=authors）。
const bookBriefColumns = "b.id, b.title, b.author_id, COALESCE(a.name, ''), " +
	"b.chapter_count, b.word_count, b.has_cover, b.rating, b.new_chapters, " +
	"b.read_status, b.series, b.series_index, b.updated_at"

// SeriesBooks 返回某个系列下的书，按系列内序号升序（同序号按最近更新）。
//
// 名字为空/系列不存在都返回 ErrNotFound（api 映射 404）：空名字不是一个系列，
// 而「这个系列一本书都没有」与「系列不存在」对调用方是同一件事。
func (s *Store) SeriesBooks(ctx context.Context, name string) ([]BookBrief, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: 系列名不能为空", ErrInvalidArgument)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+bookBriefColumns+" FROM books b LEFT JOIN authors a ON a.id = b.author_id "+
			"WHERE TRIM(b.series) = ? ORDER BY b.series_index ASC, b.updated_at DESC, b.id ASC", trimmed)
	if err != nil {
		return nil, fmt.Errorf("store: list series books: %w", err)
	}
	defer rows.Close()

	items := make([]BookBrief, 0, 8)
	for rows.Next() {
		item, err := scanBookBrief(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list series books rows: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: 系列 %q", ErrNotFound, trimmed)
	}
	return items, nil
}

// scanBookBrief 扫描一行 BookBrief（updated_at 为文本，解析失败留零值）。
func scanBookBrief(sc rowScanner) (BookBrief, error) {
	var (
		item    BookBrief
		updated string
	)
	if err := sc.Scan(&item.ID, &item.Title, &item.AuthorID, &item.AuthorName,
		&item.ChapterCount, &item.WordCount, &item.HasCover, &item.Rating, &item.NewChapters,
		&item.ReadStatus, &item.Series, &item.SeriesIndex, &updated); err != nil {
		return item, fmt.Errorf("store: scan book brief: %w", err)
	}
	item.UpdatedAt, _ = parseTime(updated)
	return item, nil
}
