package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// 库健康问题的可下钻类型（契约见 docs/api/api-dashboard.md §库健康明细）。
// 每个 kind 恰好对应 /api/health/report 里的一个数字，供驾驶舱「查看明细」用。
const (
	HealthIssueMissingAuthor = "missing_author"
	HealthIssueMissingIntro  = "missing_intro"
	HealthIssueHugeChapter   = "huge_chapter"
	HealthIssueThinBook      = "thin_book"
)

// 健康判据的 WHERE 片段：报告的 COUNT 与明细列表共用同一份，
// 保证「卡片上是 12 本」和「抽屉里列出 12 条」永远是同一个口径。
const (
	whereMissingAuthor = "author_id = 0 OR author_id NOT IN (SELECT id FROM authors)"
	whereMissingIntro  = "TRIM(intro) = ''"
)

// hugeChapterWhere / thinBookWhere 把阈值拼进 WHERE。
//
// 阈值只有一份来源（health.go 的 hugeChapterWords / thinBookChapters），
// col 传列名：带表别名时写 "c.word_count"，避免和 books.word_count 撞名。
func hugeChapterWhere(col string) string { return col + " >= " + strconv.Itoa(hugeChapterWords) }
func thinBookWhere(col string) string    { return col + " <= " + strconv.Itoa(thinBookChapters) }

// HealthIssueKinds 返回全部 kind（错误提示与文档自检共用），顺序固定。
func HealthIssueKinds() []string {
	return []string{
		HealthIssueMissingAuthor, HealthIssueMissingIntro,
		HealthIssueHugeChapter, HealthIssueThinBook,
	}
}

// HealthIssueItem 是健康明细里的一条。
//
// 书级问题（缺作者 / 缺简介 / 章节不足）只填书字段；章节级问题（超长章节）
// 另外填 ChapterID / ChapterIdx / ChapterTitle，此时 WordCount 是这一章的字数。
type HealthIssueItem struct {
	BookID       int64  `json:"bookId"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	ChapterCount int    `json:"chapterCount"`
	WordCount    int    `json:"wordCount"`
	// ChapterIdx 用指针：0 号章也是有效下标，不能被 omitempty 吃掉。
	ChapterIdx   *int   `json:"chapterIdx,omitempty"`
	ChapterID    int64  `json:"chapterId,omitempty"`
	ChapterTitle string `json:"chapterTitle,omitempty"`
}

// HealthIssues 是某一类健康问题的明细页（与 /api/books 同一套 page/size 口径）。
//
// 不嵌 Page[T]：那样字段名会与外层的分页语义撞车（items 之外还要带 kind），
// 四个字段直接摊开更好读，也更贴接口契约。
type HealthIssues struct {
	Kind  string            `json:"kind"`
	Total int               `json:"total"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
	Items []HealthIssueItem `json:"items"`
}

// healthIssueQuery 是一类问题的两条 SQL（计数 + 取一页）。
type healthIssueQuery struct {
	countSQL string
	listSQL  string
	// chapter 为 true 表示章节级明细（列清单与扫描方式都不同）。
	chapter bool
}

// bookIssueQuery 生成「书级问题」的查询：书名 + 作者 + 章节数 + 字数。
func bookIssueQuery(where string) healthIssueQuery {
	return healthIssueQuery{
		countSQL: "SELECT COUNT(*) FROM books WHERE " + where,
		listSQL: "SELECT b.id, b.title, COALESCE(a.name, ''), b.chapter_count, b.word_count" +
			" FROM books b LEFT JOIN authors a ON a.id = b.author_id" +
			" WHERE " + where + " ORDER BY b.word_count DESC, b.id ASC LIMIT ? OFFSET ?",
	}
}

// chapterIssueQuery 生成「章节级问题」的查询：书 + 章 + 本章字数。
//
// 计数与列表都带别名 c：where 里的列名是 "c.word_count"（books 也有 word_count，
// 不加别名会撞名），两边用同一个别名才谈得上「同一条 WHERE」。
func chapterIssueQuery(where string) healthIssueQuery {
	return healthIssueQuery{
		countSQL: "SELECT COUNT(*) FROM chapters c WHERE " + where,
		listSQL: "SELECT c.book_id, b.title, c.id, c.idx, c.title, c.word_count" +
			" FROM chapters c JOIN books b ON b.id = c.book_id" +
			" WHERE " + where + " ORDER BY c.word_count DESC, c.id ASC LIMIT ? OFFSET ?",
		chapter: true,
	}
}

// healthIssueQueries 是 kind → SQL 的唯一分派表；新增一类问题只在这里加一行。
var healthIssueQueries = map[string]healthIssueQuery{
	HealthIssueMissingAuthor: bookIssueQuery(whereMissingAuthor),
	HealthIssueMissingIntro:  bookIssueQuery(whereMissingIntro),
	HealthIssueThinBook:      bookIssueQuery(thinBookWhere("chapter_count")),
	HealthIssueHugeChapter:   chapterIssueQuery(hugeChapterWhere("c.word_count")),
}

// HealthIssues 返回某一类健康问题的明细（书级或章节级）。
//
// total 与 /api/health/report 里同名指标用同一条 WHERE 算出，两者恒等。
// kind 非法或 page/size 非正 → ErrInvalidArgument（不静默夹紧、不回落默认值）。
// 排序统一「字数多的在前」，让最值得先处理的书排在第一页。
func (s *Store) HealthIssues(ctx context.Context, kind string, page, size int) (HealthIssues, error) {
	q, ok := healthIssueQueries[kind]
	if !ok {
		return HealthIssues{}, fmt.Errorf("%w: kind 必须是 %s", ErrInvalidArgument,
			strings.Join(HealthIssueKinds(), " / "))
	}
	if page < 1 || size < 1 {
		return HealthIssues{}, fmt.Errorf("%w: page 与 size 必须是正整数", ErrInvalidArgument)
	}
	out := HealthIssues{Kind: kind, Page: page, Size: size, Items: []HealthIssueItem{}}
	if err := s.db.QueryRowContext(ctx, q.countSQL).Scan(&out.Total); err != nil {
		return HealthIssues{}, fmt.Errorf("store: health issues count: %w", err)
	}
	items, err := s.healthIssuePage(ctx, q, size, (page-1)*size)
	if err != nil {
		return HealthIssues{}, err
	}
	out.Items = items
	return out, nil
}

// healthIssuePage 取一页明细。
func (s *Store) healthIssuePage(ctx context.Context, q healthIssueQuery, size, offset int) ([]HealthIssueItem, error) {
	rows, err := s.db.QueryContext(ctx, q.listSQL, size, offset)
	if err != nil {
		return nil, fmt.Errorf("store: health issues list: %w", err)
	}
	defer rows.Close()

	items := make([]HealthIssueItem, 0, size)
	for rows.Next() {
		item, scanErr := scanHealthIssue(rows, q.chapter)
		if scanErr != nil {
			return nil, fmt.Errorf("store: scan health issue: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: health issues rows: %w", err)
	}
	return items, nil
}

// scanHealthIssue 按「书级 / 章节级」两种列清单扫描一行。
func scanHealthIssue(rows *sql.Rows, chapter bool) (HealthIssueItem, error) {
	var item HealthIssueItem
	if !chapter {
		err := rows.Scan(&item.BookID, &item.Title, &item.Author, &item.ChapterCount, &item.WordCount)
		return item, err
	}
	var idx int
	if err := rows.Scan(&item.BookID, &item.Title, &item.ChapterID, &idx, &item.ChapterTitle, &item.WordCount); err != nil {
		return item, err
	}
	item.ChapterIdx = &idx
	return item, nil
}
