package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// 元数据补全的三类待补清单（enrich 子命令的输入）。
//
// 三类共用一个「读 books 标量列」的查询形状，所以放在同一张分派表里；
// 新增一类只在这里加一行，不在 CLI 里另写 SQL。
const (
	EnrichKindPlaceholderAuthor = "placeholder_author"
	EnrichKindMissingIntro      = "missing_intro"
	EnrichKindUnclassified      = "unclassified"
)

// EnrichKinds 返回全部待补类型（顺序固定，CLI 的 --fields 与报告共用）。
func EnrichKinds() []string {
	return []string{EnrichKindPlaceholderAuthor, EnrichKindMissingIntro, EnrichKindUnclassified}
}

// EnrichCandidate 是一本待补的书（只读字段）。
type EnrichCandidate struct {
	BookID     int64
	Title      string
	Author     string
	Intro      string
	CategoryL1 string
	CategoryL2 string
}

// placeholderAuthorNames 是「占位作者」的 SQL 名单，与 internal/enrich 的白名单同源。
//
// 这里必须写死一份：SQL 侧判不全「归一后的等价名」（全角空格、大小写），
// 所以查询只做粗筛（命中已知占位名），真正的判定在 Go 里用 enrich.IsPlaceholderAuthor 复核。
// 粗筛条件比白名单宽是安全的（多查出来几本，复核时会被排除），窄了才会漏书。
var placeholderAuthorNames = []string{
	"佚名", "无名", "未知", "未知作者", "作者未知", "不详", "多人", "多人合集",
	"匿名", "暂无作者", "未署名", "Unknown", "unknown", "N/A", "",
}

// enrichCandidateQueries 是 kind → SQL（只读 books 与 authors，不做 JOIN 聚合）。
var enrichCandidateQueries = map[string]string{
	EnrichKindMissingIntro: "SELECT b.id, b.title, COALESCE(a.name, ''), b.intro," +
		" COALESCE(b.category_l1, ''), COALESCE(b.category_l2, '')" +
		" FROM books b LEFT JOIN authors a ON a.id = b.author_id" +
		" WHERE TRIM(b.intro) = '' AND " + activeBookCond +
		" ORDER BY b.word_count DESC, b.id ASC LIMIT ?",
	EnrichKindUnclassified: "SELECT b.id, b.title, COALESCE(a.name, ''), b.intro," +
		" COALESCE(b.category_l1, ''), COALESCE(b.category_l2, '')" +
		" FROM books b LEFT JOIN authors a ON a.id = b.author_id" +
		" WHERE (b.category_l1 = '' OR b.category_l1 = '未分类' OR b.category_l1 IS NULL) AND " + activeBookCond +
		" ORDER BY b.word_count DESC, b.id ASC LIMIT ?",
}

// EnrichCandidates 取一类待补书（limit ≤ 0 表示不限制）。
//
// 占位作者那一类走单独实现：它要按「作者名在不在占位名单里」筛，
// SQL 只能做粗筛，最终判定在 enricher 侧用 enrich.IsPlaceholderAuthor 复核。
func (s *Store) EnrichCandidates(ctx context.Context, kind string, limit int) ([]EnrichCandidate, error) {
	return s.EnrichCandidatesSince(ctx, kind, limit, time.Time{})
}

// EnrichCandidatesSince 取一类待补书（limit ≤ 0 表示不限制；since 非零时只取 imported_at >= since 的书）。
//
// 占位作者那一类走单独实现：它要按「作者名在不在占位名单里」筛，
// SQL 只能做粗筛，最终判定在 enricher 侧用 enrich.IsPlaceholderAuthor 复核。
func (s *Store) EnrichCandidatesSince(ctx context.Context, kind string, limit int, since time.Time) ([]EnrichCandidate, error) {
	hasSince := !since.IsZero()
	sinceStr := ""
	if hasSince {
		sinceStr = since.UTC().Format(time.RFC3339)
	}
	if kind == EnrichKindPlaceholderAuthor {
		return s.placeholderAuthorCandidates(ctx, limit, sinceStr, hasSince)
	}
	sqlText, ok := enrichCandidateQueries[kind]
	if !ok {
		return nil, fmt.Errorf("%w: kind 必须是 %s 之一", ErrInvalidArgument, joinKinds())
	}
	if hasSince {
		sqlText = strings.Replace(sqlText, " ORDER BY", " AND b.imported_at >= ? ORDER BY", 1)
	}
	return s.queryEnrichCandidates(ctx, sqlText, limit, sinceStr, hasSince)
}

// placeholderAuthorCandidates 取作者名为占位值的书（粗筛 + 原名返回，复核由调用方做）。
func (s *Store) placeholderAuthorCandidates(ctx context.Context, limit int, sinceStr string, hasSince bool) ([]EnrichCandidate, error) {
	sqlText := "SELECT b.id, b.title, COALESCE(a.name, ''), b.intro," +
		" COALESCE(b.category_l1, ''), COALESCE(b.category_l2, '')" +
		" FROM books b LEFT JOIN authors a ON a.id = b.author_id" +
		" WHERE a.name IN (" + placeholders(len(placeholderAuthorNames)) + ") AND " + activeBookCond
	if hasSince {
		sqlText += " AND b.imported_at >= ?"
	}
	sqlText += " ORDER BY b.word_count DESC, b.id ASC"
	args := make([]any, 0, len(placeholderAuthorNames)+2)
	for _, n := range placeholderAuthorNames {
		args = append(args, n)
	}
	if hasSince {
		args = append(args, sinceStr)
	}
	if limit > 0 {
		sqlText += " LIMIT ?"
		args = append(args, limit)
	}
	return s.scanEnrichCandidates(ctx, sqlText, args...)
}

// queryEnrichCandidates 执行一条带 LIMIT 占位的候选查询。
func (s *Store) queryEnrichCandidates(ctx context.Context, sqlText string, limit int, sinceStr string, hasSince bool) ([]EnrichCandidate, error) {
	args := make([]any, 0, 2)
	if hasSince {
		args = append(args, sinceStr)
	}
	if limit <= 0 {
		args = append(args, 1_000_000)
	} else {
		args = append(args, limit)
	}
	return s.scanEnrichCandidates(ctx, sqlText, args...)
}

// scanEnrichCandidates 扫描候选行。
func (s *Store) scanEnrichCandidates(ctx context.Context, sqlText string, args ...any) ([]EnrichCandidate, error) {
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("store: enrich candidates: %w", err)
	}
	defer rows.Close()
	out := make([]EnrichCandidate, 0, 256)
	for rows.Next() {
		var c EnrichCandidate
		if err := rows.Scan(&c.BookID, &c.Title, &c.Author, &c.Intro, &c.CategoryL1, &c.CategoryL2); err != nil {
			return nil, fmt.Errorf("store: scan enrich candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: enrich candidates rows: %w", err)
	}
	return out, nil
}

// joinKinds 把三类拼成错误提示。
func joinKinds() string {
	return EnrichKindPlaceholderAuthor + " / " + EnrichKindMissingIntro + " / " + EnrichKindUnclassified
}

// HugeChapter 是一个超长章节（切分命令的输入）。
type HugeChapter struct {
	BookID    int64
	ChapterID int64
	Index     int
	Title     string
	WordCount int
}

// ListHugeChapters 取字数 ≥ minWords 的章节（按字数降序；limit ≤ 0 表示不限制）。
//
// 只返回当前版本的书（activeBookCond）：被取代的旧版本不进任何列表与统计。
func (s *Store) ListHugeChapters(ctx context.Context, minWords, limit int) ([]HugeChapter, error) {
	if minWords <= 0 {
		return nil, fmt.Errorf("%w: minWords 必须是正整数", ErrInvalidArgument)
	}
	sqlText := "SELECT c.book_id, c.id, c.idx, c.title, c.word_count" +
		" FROM chapters c JOIN books b ON b.id = c.book_id" +
		" WHERE c.word_count >= ? AND " + activeBookCond +
		" ORDER BY c.word_count DESC, c.id ASC"
	args := []any{minWords}
	if limit > 0 {
		sqlText += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list huge chapters: %w", err)
	}
	defer rows.Close()
	out := make([]HugeChapter, 0, 128)
	for rows.Next() {
		var h HugeChapter
		if err := rows.Scan(&h.BookID, &h.ChapterID, &h.Index, &h.Title, &h.WordCount); err != nil {
			return nil, fmt.Errorf("store: scan huge chapter: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: huge chapters rows: %w", err)
	}
	return out, nil
}
