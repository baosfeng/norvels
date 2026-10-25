package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Bucket 是一个直方图分桶（label 是给前端直接显示的中文/区间文案）。
type Bucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// HealthReport 是驾驶舱「库健康」状态卡与两张直方图的数据。
//
// 指针字段为 nil 表示该单项统计失败（已写入 Warnings），接口整体仍返回 200。
type HealthReport struct {
	TotalBooks           *int     `json:"totalBooks"`
	ThinBooks            *int     `json:"thinBooks"`
	MissingAuthor        *int     `json:"missingAuthor"`
	MissingIntro         *int     `json:"missingIntro"`
	HugeChapters         *int     `json:"hugeChapters"`
	ChapterBuckets       []Bucket `json:"chapterBuckets"`
	ChapterLengthBuckets []Bucket `json:"chapterLengthBuckets"`
	Warnings             []string `json:"warnings"`
	// Stale 为 true 表示全库聚合值（store_metrics）尚未算好，数字可能为 0；
	// 后台补算完成后自动转为 false。属于增量字段，前端可忽略。
	Stale bool `json:"stale,omitempty"`

	// warnMu 串行化并发统计时的 Warnings 追加（指针以免 HealthReport 被复制时带走锁）。
	warnMu *sync.Mutex
}

// 分桶边界（写死在代码里，改动需同步 docs/api.md §8）：
//
//	章节数分桶：1 / 2-9 / 10-49 / 50-199 / 200-999 / 1000+
//	  用途：识别「超短篇」与「超大合集」，一本书一章的书通常是碎片文件。
//	章节长度分桶（按章字数）：<1千字 / 1千-5千 / 5千-2万 / 2万-10万 / 10万+
//	  用途：识别切章失败（整本挤进一章）与碎章（每章十几字）。
var (
	chapterBucketLabels = []string{"1", "2-9", "10-49", "50-199", "200-999", "1000+"}
	chapterBucketEdges  = []int{1, 2, 10, 50, 200, 1000} // 第 i 桶下界
	lengthBucketLabels  = []string{"<1千字", "1千-5千", "5千-2万", "2万-10万", "10万+"}
	lengthBucketEdges   = []int{0, 1000, 5000, 20000, 100000}
)

// 健康判据阈值。
const (
	thinBookChapters = 1      // 章节数 ≤ 1 视为「超薄」
	hugeChapterWords = 100000 // 单章字数 ≥ 10 万视为「超大章」（切章可能失败）
)

// HealthReport 汇总库健康指标：每项独立查询，单项失败返回 nil 并记 Warnings。
//
// 七项统计相互独立，这里并发发起：连接池有 8 个连接，串行会白等 IO。
// 真实库（42 万章）实测串行 138 ms、并发约 40 ms。
func (s *Store) HealthReport(ctx context.Context) HealthReport {
	r := HealthReport{Warnings: []string{}}
	var wg sync.WaitGroup
	wg.Add(7)
	go func() {
		defer wg.Done()
		r.TotalBooks = s.intMetric(ctx, &r, "totalBooks", "SELECT COUNT(*) FROM books")
	}()
	go func() {
		defer wg.Done()
		r.ThinBooks = s.intMetric(ctx, &r, "thinBooks",
			"SELECT COUNT(*) FROM books WHERE chapter_count <= ?", thinBookChapters)
	}()
	go func() {
		defer wg.Done()
		r.MissingAuthor = s.intMetric(ctx, &r, "missingAuthor",
			"SELECT COUNT(*) FROM books WHERE author_id = 0 OR author_id NOT IN (SELECT id FROM authors)")
	}()
	go func() {
		defer wg.Done()
		r.MissingIntro = s.intMetric(ctx, &r, "missingIntro",
			"SELECT COUNT(*) FROM books WHERE TRIM(intro) = ''")
	}()
	go func() {
		defer wg.Done()
		r.HugeChapters = s.intMetric(ctx, &r, "hugeChapters",
			"SELECT COUNT(*) FROM chapters WHERE word_count >= ?", hugeChapterWords)
	}()
	go func() {
		defer wg.Done()
		r.ChapterBuckets = s.bucketMetric(ctx, &r, "chapterBuckets",
			chapterBucketLabels, chapterBucketEdges, "chapter_count", "books")
	}()
	go func() {
		defer wg.Done()
		r.ChapterLengthBuckets = s.bucketMetric(ctx, &r, "chapterLengthBuckets",
			lengthBucketLabels, lengthBucketEdges, "word_count", "chapters")
	}()
	wg.Wait()

	if snap, err := s.Metrics(ctx); err == nil {
		r.Stale = snap.Stale
	}
	return r
}

// intMetric 执行单个标量统计；失败时记 Warnings 并返回 nil。
// 并发调用时用 warn 串行化 Warnings 追加（append 非并发安全）。
func (s *Store) intMetric(ctx context.Context, r *HealthReport, name, query string, args ...any) *int {
	var n int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		r.warn(name, err)
		return nil
	}
	return &n
}

// warn 在线程安全的前提下追加一条 Warnings。
func (r *HealthReport) warn(name string, err error) {
	if r.warnMu == nil {
		r.warnMu = &sync.Mutex{}
	}
	r.warnMu.Lock()
	defer r.warnMu.Unlock()
	r.Warnings = append(r.Warnings, fmt.Sprintf("%s 统计失败：%v", name, err))
}

// bucketMetric 把一个数值列按 edges 分桶计数；失败时记 Warnings 并返回 nil。
//
// 桶语义：第 i 桶统计 [edges[i], edges[i+1])，最后一桶为 [edges[last], +∞)。
//
// 实现是「一条 SQL 里的 N 个区间 COUNT 子查询」，每个子查询都能走 idx_chapters_wc
// 的覆盖索引做区间 SEARCH，只读 word_count 这一列。
//
// 为什么不用 CASE WHEN + GROUP BY：它要对索引做一次全量扫描（真实库 42 万行），
// 纯 Go CLI 下 0.09 s、go 驱动下 143 ms；而区间 SEARCH 只走索引边界，实测 0.01 s。
// 也不能把 42 万行拉进 Go 逐行计数：那要先读完整行记录（含数 GB 正文），实测 73.6 s。
func (s *Store) bucketMetric(ctx context.Context, r *HealthReport, name string,
	labels []string, edges []int, column, table string) []Bucket {
	counts, err := s.bucketCounts(ctx, column, table, edges)
	if err != nil {
		r.warn(name, err)
		return nil
	}
	out := make([]Bucket, len(labels))
	for i := range labels {
		out[i] = Bucket{Label: labels[i], Count: counts[i]}
	}
	return out
}

// bucketCounts 用一个 SELECT 拿到各桶计数（第 i 个结果列 = 第 i 桶）。
func (s *Store) bucketCounts(ctx context.Context, column, table string, edges []int) ([]int, error) {
	args := make([]any, 0, len(edges)*2)
	for i := range edges {
		args = append(args, edges[i])
		if i+1 < len(edges) {
			args = append(args, edges[i+1])
		}
	}
	counts := make([]int, len(edges))
	row := s.db.QueryRowContext(ctx, bucketRangeSQL(column, table, edges), args...)
	dest := make([]any, len(edges))
	for i := range counts {
		dest[i] = &counts[i]
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return counts, nil
}

// bucketRangeSQL 生成「一条 SELECT 里 N 个区间 COUNT」的语句。
//
// 第 i 桶 = [edges[i], edges[i+1])，最后一桶 = [edges[last], +∞)。
// 每个子查询都是 column 上的开区间 SEARCH，能命中 (column) 覆盖索引。
func bucketRangeSQL(column, table string, edges []int) string {
	parts := make([]string, 0, len(edges))
	for i := range edges {
		switch {
		case i+1 < len(edges):
			parts = append(parts, fmt.Sprintf(
				"(SELECT COUNT(*) FROM %s WHERE %s >= ? AND %s < ?)", table, column, column))
		default:
			parts = append(parts, fmt.Sprintf(
				"(SELECT COUNT(*) FROM %s WHERE %s >= ?)", table, column))
		}
	}
	return "SELECT " + strings.Join(parts, ", ")
}

// bucketHistogramSQL 生成「CASE WHEN 分桶 + COUNT(*)」的聚合语句（保留给对照测试）。
func bucketHistogramSQL(column, table string, edges []int) string {
	return "SELECT " + bucketCaseExpr(column, edges) + " AS bucket, COUNT(*) FROM " + table + " GROUP BY bucket"
}

// bucketCaseExpr 生成把 column 映射成桶号的 CASE 表达式。
//
// 第 i 桶的语义是 [edges[i], edges[i+1])，最后一桶是 [edges[last], +∞)。
// 因此按「边界从大到小」平铺 WHEN：第一个命中的就是答案（越大的边界优先级越高），
// 都不命中落到 0 号桶。平铺而非嵌套，是为了让生成的 SQL 与 bucketIndex 的循环一眼对得上。
func bucketCaseExpr(column string, edges []int) string {
	if len(edges) <= 1 {
		return "0"
	}
	var b strings.Builder
	b.WriteString("CASE")
	for i := len(edges) - 1; i >= 1; i-- {
		fmt.Fprintf(&b, " WHEN %s >= %d THEN %d", column, edges[i], i)
	}
	b.WriteString(" ELSE 0 END")
	return b.String()
}

// bucketIndex 返回数值 v 落在 edges 定义的第几个桶。
func bucketIndex(edges []int, v int) int {
	idx := 0
	for i := 1; i < len(edges); i++ {
		if v >= edges[i] {
			idx = i
			continue
		}
		break
	}
	return idx
}

// ImportTrendPoint 是导入趋势里的一天（无数据日期补 0）。
type ImportTrendPoint struct {
	Date     string `json:"date"` // YYYY-MM-DD（本地时区）
	Books    int    `json:"books"`
	Chapters int    `json:"chapters"`
}

// ImportTrend 返回最近 days 天（含今天）每天的导入书数与章节数。
//
// 章节按所属书的 imported_at 归日（章节本身没有导入时间），
// 没有数据的日期补 0，保证前端 x 轴连续。
func (s *Store) ImportTrend(ctx context.Context, days int) ([]ImportTrendPoint, error) {
	if days <= 0 {
		days = 14
	}
	if days > maxTrendDays {
		days = maxTrendDays
	}
	start := trendStartDay(days)
	out := make([]ImportTrendPoint, 0, days)
	for i := 0; i < days; i++ {
		out = append(out, ImportTrendPoint{Date: start.AddDate(0, 0, i).Format(dateLayout)})
	}
	books, chapters, err := s.trendCounts(ctx, start)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Books = books[out[i].Date]
		out[i].Chapters = chapters[out[i].Date]
	}
	return out, nil
}

const (
	// dateLayout 是趋势图使用的本地日期键。
	dateLayout = "2006-01-02"
	// maxTrendDays 是趋势查询的天数上限（一年）。
	maxTrendDays = 366
)

// trendStartDay 返回「今天往回 days-1 天」的本地零点。
func trendStartDay(days int) time.Time {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return today.AddDate(0, 0, -(days - 1))
}

// trendCounts 按本地日期聚合书数与章节数，返回两张 date→count 表。
func (s *Store) trendCounts(ctx context.Context, start time.Time) (map[string]int, map[string]int, error) {
	books, err := s.countByDay(ctx,
		"SELECT imported_at FROM books WHERE imported_at >= ?", start)
	if err != nil {
		return nil, nil, err
	}
	chapters, err := s.countByDay(ctx,
		"SELECT b.imported_at FROM chapters c JOIN books b ON b.id = c.book_id WHERE b.imported_at >= ?", start)
	if err != nil {
		return nil, nil, err
	}
	return books, chapters, nil
}

// countByDay 执行「取一列时间文本」的查询并按本地日期计数。
//
// 时间文本由 store 统一写成 UTC（YYYY-MM-DDTHH:MM:SS.sssZ），
// 这里解析后转本地时区再取日期，避免 UTC 跨日把凌晨导入算到前一天。
func (s *Store) countByDay(ctx context.Context, query string, args ...any) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: import trend: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int, 32)
	for rows.Next() {
		var raw sql.NullString
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("store: scan import trend: %w", err)
		}
		if !raw.Valid || raw.String == "" {
			continue
		}
		t, err := parseTime(raw.String)
		if err != nil {
			continue // 历史脏数据不影响趋势图
		}
		counts[t.Local().Format(dateLayout)]++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: import trend rows: %w", err)
	}
	return counts, nil
}

var _ = errors.Is
