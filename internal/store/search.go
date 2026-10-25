package store

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// minSearchRunes 是可检索的最小字符数。单字在标题上命中范围过宽
// （「的」「一」几乎命中全书），对读者没有区分度，因此直接拒绝。
const minSearchRunes = 2

// 搜索关键词的拒绝提示（搜不出结果时直接告诉用户原因，不跑全表 LIKE）。
const (
	noticeEmptyQuery   = "请输入搜索关键词"
	noticeTooShort     = "关键词至少 2 个字"
	noticeNoSearchChar = "关键词需要包含文字或数字"
)

// hasSearchToken 判断关键词是否含可用于 LIKE 的字符（字母/数字/CJK）。
// 纯标点或空白的查询直接短路，避免构造出 "%%。" 这类必然命中又毫无意义的模式。
func hasSearchToken(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// Search 按「书名 / 作者 / 章节标题」检索。
//
// 检索范围是**元数据**，不含正文：正文只做阅读，不做倒排索引。
// 三类命中都走 LIKE（书名与作者见 books.go 的 bookFilter，章节标题见 searchChapters），
// 因此没有索引可走，但匹配的是千级 books 行与百万级 chapters 标题列，
// 典型查询在毫秒到数十毫秒级完成。
func (s *Store) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	page, size := normPage(q.Page, q.Size)
	query := strings.TrimSpace(q.Q)
	out := newEmptySearchResult(query, page, size)

	if notice := validateSearchQuery(query); notice != "" {
		out.Notice = notice
		return out, nil
	}

	if err := s.fillSearchHits(ctx, out, q, page, size); err != nil {
		return nil, err
	}
	return out, nil
}

// newEmptySearchResult 构造空结果骨架（分页字段先填好，命中数为 0）。
func newEmptySearchResult(query string, page, size int) *SearchResult {
	return &SearchResult{
		Q:        query,
		Books:    Page[BookSummary]{Items: []BookSummary{}, Total: 0, Page: page, Size: size},
		Chapters: Page[ChapterHit]{Items: []ChapterHit{}, Total: 0, Page: page, Size: size},
	}
}

// validateSearchQuery 检查关键词是否可检索；返回空串表示通过，否则是给前端的提示语。
func validateSearchQuery(query string) string {
	switch {
	case query == "":
		return noticeEmptyQuery
	case utf8.RuneCountInString(query) < minSearchRunes:
		return noticeTooShort
	case !hasSearchToken(query):
		return noticeNoSearchChar
	}
	return ""
}

// searchTypes 是 SearchQuery.Type 的几种检索维度（"all" 表示都查）。
const (
	searchTypeAll     = "all"
	searchTypeBook    = "book"
	searchTypeChapter = "chapter"
)

// searchType 归一化检索类型：无法识别的取值按 "all" 处理。
func searchType(t string) string {
	switch typ := strings.ToLower(strings.TrimSpace(t)); typ {
	case searchTypeBook, searchTypeChapter:
		return typ
	default:
		return searchTypeAll
	}
}

// fillSearchHits 按检索类型填充书名命中与章节标题命中。
func (s *Store) fillSearchHits(ctx context.Context, out *SearchResult, q SearchQuery, page, size int) error {
	typ := searchType(q.Type)
	if typ == searchTypeBook || typ == searchTypeAll {
		books, err := s.ListBooks(ctx, BookQuery{Q: out.Q, Page: page, Size: size, Sort: "updated"})
		if err != nil {
			return err
		}
		out.Books = books
	}
	if typ == searchTypeChapter || typ == searchTypeAll {
		chapters, err := s.searchChapters(ctx, out.Q, q.BookID, page, size)
		if err != nil {
			return err
		}
		out.Chapters = chapters
	}
	return nil
}

// searchChapters 按**章节标题**做 LIKE 匹配，可限定单本书。
//
// Snippet 填「第 N 章 · 共 M 章」：正文检索已下线，无法给正文片段，
// 但标题命中没有上下文时，用户看不出这一章在书里的位置。
func (s *Store) searchChapters(ctx context.Context, query string, bookID int64, page, size int) (Page[ChapterHit], error) {
	pat := "%" + escapeLike(query) + "%"
	where := "c.title LIKE ? ESCAPE '!'"
	args := []any{pat}
	if bookID > 0 {
		where += " AND c.book_id = ?"
		args = append(args, bookID)
	}

	var total int
	countSQL := "SELECT COUNT(*) FROM chapters c WHERE " + where
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return Page[ChapterHit]{}, fmt.Errorf("store: search chapters count: %w", err)
	}
	if total == 0 {
		return Page[ChapterHit]{Items: []ChapterHit{}, Page: page, Size: size}, nil
	}

	// 排序口径：先按所属书名、再按章节序号，保证同一本书内命中顺序稳定；
	// 正文检索时代的 bm25 得分已不存在，Score 恒为 0。
	dataSQL := "SELECT c.book_id, b.title, c.id, c.idx, c.title, " +
		"(SELECT COUNT(*) FROM chapters x WHERE x.book_id = c.book_id) " +
		"FROM chapters c JOIN books b ON b.id = c.book_id WHERE " + where +
		" ORDER BY b.title COLLATE NOCASE ASC, c.book_id ASC, c.idx ASC, c.id LIMIT ? OFFSET ?"
	dataArgs := append(append([]any{}, args...), size, (page-1)*size)

	rows, err := s.db.QueryContext(ctx, dataSQL, dataArgs...)
	if err != nil {
		return Page[ChapterHit]{}, fmt.Errorf("store: search chapters: %w", err)
	}
	defer rows.Close()

	items := make([]ChapterHit, 0, size)
	for rows.Next() {
		var (
			hit           ChapterHit
			totalChapters int
		)
		if err := rows.Scan(&hit.BookID, &hit.BookTitle, &hit.ChapterID, &hit.ChapterIdx,
			&hit.ChapterTitle, &totalChapters); err != nil {
			return Page[ChapterHit]{}, fmt.Errorf("store: scan chapter hit: %w", err)
		}
		hit.Snippet = chapterContext(hit.ChapterIdx, totalChapters)
		items = append(items, hit)
	}
	if err := rows.Err(); err != nil {
		return Page[ChapterHit]{}, fmt.Errorf("store: search chapters rows: %w", err)
	}
	return Page[ChapterHit]{Items: items, Total: total, Page: page, Size: size}, nil
}

// chapterContext 给标题命中补一句位置上下文（正文检索已下线，只能给章序）。
func chapterContext(idx, total int) string {
	if total <= 0 {
		return ""
	}
	return fmt.Sprintf("第 %d 章 · 共 %d 章", idx+1, total)
}
