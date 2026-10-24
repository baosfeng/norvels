package store

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// minSearchRunes 是字级分词方案可检索的最小字符数（单字无法构成有意义的 phrase）。
const minSearchRunes = 2

// dq 是 FTS5 查询串里的双引号。
const dq = string(rune(34))

// buildFTSQuery 把用户输入转成 FTS5 查询串：
// 每个空白分隔的词先经 Segment 规整（汉字逐字加空格），再用双引号包成 phrase，
// 词与词之间是 AND 语义。整串都走字面量，不做前缀/通配解释。
func buildFTSQuery(q string) string {
	fields := strings.Fields(q)
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		seg := strings.TrimSpace(Segment(f))
		if seg == "" {
			continue
		}
		parts = append(parts, quoteFTSPhrase(seg))
	}
	return strings.Join(parts, " ")
}

// quoteFTSPhrase 用双引号包裹 phrase，内部双引号按 FTS5 规则双写转义。
func quoteFTSPhrase(s string) string {
	return dq + strings.ReplaceAll(s, dq, dq+dq) + dq
}

// hasSearchToken 判断关键词是否含可被 unicode61 切出 token 的字符（字母/数字/CJK），
// 纯标点或空白的查询直接短路，避免构造出空 phrase 触发 FTS5 语法错误。
func hasSearchToken(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// Search 全文/书名检索。
//
// chapters 走 chapters_fts（字级分词，unicode61），books 走书名/作者的 LIKE。
// 关键词少于 2 个字符时直接返回空结果并给出 Notice（全表 LIKE 在 4.4GB 下不可接受）。
func (s *Store) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	page, size := normPage(q.Page, q.Size)
	query := strings.TrimSpace(q.Q)

	out := &SearchResult{
		Q:        query,
		Books:    Page[BookSummary]{Items: []BookSummary{}, Total: 0, Page: page, Size: size},
		Chapters: Page[ChapterHit]{Items: []ChapterHit{}, Total: 0, Page: page, Size: size},
	}
	if query == "" {
		out.Notice = "请输入搜索关键词"
		return out, nil
	}
	if utf8.RuneCountInString(query) < minSearchRunes {
		out.Notice = "关键词至少 2 个字（当前库使用字级全文索引）"
		return out, nil
	}
	if !hasSearchToken(query) {
		out.Notice = "关键词需要包含文字或数字"
		return out, nil
	}

	typ := strings.ToLower(strings.TrimSpace(q.Type))
	if typ != "book" && typ != "chapter" {
		typ = "all"
	}

	if typ == "book" || typ == "all" {
		books, err := s.ListBooks(ctx, BookQuery{Q: query, Page: page, Size: size, Sort: "updated"})
		if err != nil {
			return nil, err
		}
		out.Books = books
	}
	if typ == "chapter" || typ == "all" {
		chapters, capped, err := s.searchChapters(ctx, query, q.BookID, page, size)
		if err != nil {
			return nil, err
		}
		out.Chapters = chapters
		if capped {
			out.Notice = searchTotalCapNotice
		}
	}
	return out, nil
}

// searchTotalCap 是全文检索 total 的统计上限：超过后只报上限并提示，
// 避免高频词（如「他的」）在大库上为算精确总数而全量扫描。
const searchTotalCap = 1000

// searchTotalCapNotice 是触发 total 截断时写进 SearchResult.Notice 的提示。
const searchTotalCapNotice = "命中过多，仅统计前 1000 条"

// searchChapters 用 FTS5 MATCH 检索章节正文/标题，返回 snippet 与 bm25 得分。
// 第二个返回值表示 total 是否被截断到 searchTotalCap。
func (s *Store) searchChapters(ctx context.Context, query string, bookID int64, page, size int) (Page[ChapterHit], bool, error) {
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return Page[ChapterHit]{Items: []ChapterHit{}, Page: page, Size: size}, false, nil
	}

	where := "chapters_fts MATCH ?"
	args := []any{ftsQuery}
	if bookID > 0 {
		where += " AND c.book_id = ?"
		args = append(args, bookID)
	}

	// total 只数到 searchTotalCap+1 为止；分页数据查询不受影响，page>50 依然有结果。
	probeSQL := "SELECT COUNT(*) FROM (SELECT chapters_fts.rowid FROM chapters_fts " +
		"JOIN chapters c ON c.id = chapters_fts.rowid WHERE " + where + " LIMIT ?)"
	probeArgs := append(append([]any{}, args...), searchTotalCap+1)
	var probe int
	if err := s.db.QueryRowContext(ctx, probeSQL, probeArgs...).Scan(&probe); err != nil {
		return Page[ChapterHit]{}, false, fmt.Errorf("store: search chapters count: %w", err)
	}
	total := probe
	capped := false
	if probe > searchTotalCap {
		total = searchTotalCap
		capped = true
	}

	dataSQL := "SELECT c.book_id, b.title, c.id, c.idx, c.title, " +
		"snippet(chapters_fts, 1, char(1), char(2), '…', 40), bm25(chapters_fts) " +
		"FROM chapters_fts JOIN chapters c ON c.id = chapters_fts.rowid " +
		"JOIN books b ON b.id = c.book_id WHERE " + where +
		" ORDER BY bm25(chapters_fts) ASC LIMIT ? OFFSET ?"
	dataArgs := append(append([]any{}, args...), size, (page-1)*size)

	rows, err := s.db.QueryContext(ctx, dataSQL, dataArgs...)
	if err != nil {
		return Page[ChapterHit]{}, false, fmt.Errorf("store: search chapters: %w", err)
	}
	defer rows.Close()

	items := make([]ChapterHit, 0, size)
	for rows.Next() {
		var (
			hit  ChapterHit
			snip string
			rank float64
		)
		if err := rows.Scan(&hit.BookID, &hit.BookTitle, &hit.ChapterID, &hit.ChapterIdx,
			&hit.ChapterTitle, &snip, &rank); err != nil {
			return Page[ChapterHit]{}, false, fmt.Errorf("store: scan chapter hit: %w", err)
		}
		hit.Snippet = renderSnippet(snip)
		hit.Score = -rank
		items = append(items, hit)
	}
	if err := rows.Err(); err != nil {
		return Page[ChapterHit]{}, false, fmt.Errorf("store: search chapters rows: %w", err)
	}
	return Page[ChapterHit]{Items: items, Total: total, Page: page, Size: size}, capped, nil
}
