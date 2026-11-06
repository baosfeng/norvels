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
	// Content 是正文库对账摘要（孤儿块 / 悬空引用），见 ReconcileContent。
	// 增量字段：前端未适配时忽略即可。对账结果缓存 10 分钟，不随每次请求重跑。
	Content *ContentHealth `json:"content,omitempty"`
	// Fingerprints 是章级指纹覆盖率（0019 起）：缺口 > 0 时 Hint 指向回填命令。
	// 增量字段：迁移未应用时为 nil。
	Fingerprints *FingerprintHealth `json:"fingerprints,omitempty"`
	// Quality 是质量分覆盖率（0020 起）：缺口 > 0 时 Hint 指向 backfill-quality。
	// 增量字段：统计失败时为 nil。
	Quality *QualityHealth `json:"quality,omitempty"`

	// warnMu 串行化并发统计时的 Warnings 追加（指针以免 HealthReport 被复制时带走锁）。
	warnMu *sync.Mutex
}

// 分桶边界（写死在代码里，改动需同步 docs/api.md §8）：
//
//	章节数分桶：1 / 2-9 / 10-49 / 50-199 / 200-999 / 1000+
//	  用途：识别「超短篇」与「超大合集」，一本书一章的书通常是碎片文件。
//	章节长度分桶（按章字数）：<1千 / 1千-3千 / 3千-5千 / 5千-1万 / 1万+
//	  用途：识别切章失败（整本挤进一章）与碎章（每章十几字）。
//
//	  旧档位 <1千字/1千-5千/5千-2万/2万-10万/10万+ 在真实语料上失真：82% 压进一格，
//	  2万-10万 只有 0.04%、10万+ 恒为 0（实测 42.3 万章）。柱状图上后两根是空的，
//	  前一根吞掉所有信息。改为在 1千-1万 之间细分，五档都有实质占比
//	  （实测 2.3% / 67.4% / 14.7% / 11.8% / 3.9%），能看出切章质量的形状。
var (
	chapterBucketLabels = []string{"1", "2-9", "10-49", "50-199", "200-999", "1000+"}
	chapterBucketEdges  = []int{1, 2, 10, 50, 200, 1000} // 第 i 桶下界
	lengthBucketLabels  = []string{"<1千", "1千-3千", "3千-5千", "5千-1万", "1万+"}
	lengthBucketEdges   = []int{0, 1000, 3000, 5000, 10000}
)

// 健康判据阈值。
const (
	thinBookChapters = 1 // 章节数 ≤ 1 视为「超薄」
	// 单章字数 ≥ 3 万视为「超大章」（切章可能失败）。这个阈值必须与驾驶舱文案
	// 「单章超过 3 万字」一致：之前写 10 万，而真实语料最长 31873 字，
	// 导致这一项恒为 0，看着像「没有任何问题」，实际有 5 章超 3 万字。
	hugeChapterWords = 30000
)

// HealthReport 汇总库健康指标：每项独立查询，单项失败返回 nil 并记 Warnings。
//
// 七项统计相互独立，这里并发发起：连接池有 8 个连接，串行会白等 IO。
// 真实库（42 万章）实测串行 138 ms、并发约 40 ms。
func (s *Store) HealthReport(ctx context.Context) HealthReport {
	r := HealthReport{Warnings: []string{}}
	// lengthLive 记录「长度直方图这次是实时算的」（预计算表不可用）：写在这里而不是
	// 直接改 r.Stale，是因为它由并发 goroutine 写入，wg.Wait() 之后才允许读。
	var lengthLive bool
	var wg sync.WaitGroup
	wg.Add(7)
	go func() {
		defer wg.Done()
		// 只数当前版本：健康报告的书数与书架同口径，否则替换一次旧版就多出「一本书」。
		r.TotalBooks = s.intMetric(ctx, &r, "totalBooks", "SELECT COUNT(*) FROM books WHERE superseded_by = 0")
	}()
	// 四项判据都来自 health_issues.go 的同一份 WHERE：报告的数字与「查看明细」
	// 接口列出的条数由同一条 SQL 条件保证一致，不会两处各写一套而漂移。
	go func() {
		defer wg.Done()
		r.ThinBooks = s.intMetric(ctx, &r, "thinBooks",
			"SELECT COUNT(*) FROM books WHERE "+thinBookWhere("chapter_count"))
	}()
	go func() {
		defer wg.Done()
		r.MissingAuthor = s.intMetric(ctx, &r, "missingAuthor",
			"SELECT COUNT(*) FROM books WHERE "+whereMissingAuthor)
	}()
	go func() {
		defer wg.Done()
		r.MissingIntro = s.intMetric(ctx, &r, "missingIntro",
			"SELECT COUNT(*) FROM books WHERE "+whereMissingIntro)
	}()
	go func() {
		defer wg.Done()
		r.HugeChapters = s.intMetric(ctx, &r, "hugeChapters",
			"SELECT COUNT(*) FROM chapters WHERE "+hugeChapterWhere("word_count"))
	}()
	go func() {
		defer wg.Done()
		r.ChapterBuckets = s.bucketMetric(ctx, &r, "chapterBuckets",
			chapterBucketLabels, chapterBucketEdges, "chapter_count", "books")
	}()
	go func() {
		defer wg.Done()
		// 0022 起走预计算直方图：chapters 有 2,469 万行时，实时跑那 5 段区间 COUNT
		// 要扫 5 遍索引（NAS 实测 3.35 s，9 个驾驶舱请求并发时 25.5 s）。
		// 缓存缺失时降到实时算并置 Stale，接口照常 200。
		r.ChapterLengthBuckets, lengthLive = s.lengthBuckets(ctx, &r)
	}()
	wg.Wait()

	if snap, err := s.Metrics(ctx); err == nil {
		r.Stale = snap.Stale || lengthLive
	} else {
		r.Stale = lengthLive
	}
	// 正文库对账：孤儿块 / 悬空引用是两库（没有跨库事务）唯一会漂移的地方，
	// 必须出现在健康报告里。但它是全表扫描（生产数十秒、会把整个接口拖到 60 s 超时，
	// 而驾驶舱的 STATS_TIMEOUT_MS = 20_000，21 s 的响应本来就是超时失败），
	// 所以这里只读后台刷新留下的快照，过期就标 stale（见 health_refresh.go）。
	r.Content = s.contentHealth(&r)
	// 指纹覆盖率：0019 之后历史书要靠 backfill-fingerprints 补齐，缺口必须可见。
	// 0023 起读书级覆盖表（O(书数)）；覆盖表的历史回填由后台任务分批补，
	// 补完之前这里标 stale + backfilling，并在读取时顺带确保后台任务在跑。
	r.Fingerprints = s.fingerprintHealthWithBackfill(ctx, &r)
	// 质量分覆盖率：0020 之后版本择优要靠 backfill-quality 补齐历史书，缺口同样必须可见。
	r.Quality = s.qualityHealth(ctx, &r)
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
//
// 0022 起章节长度直方图走预计算表（lengthBuckets → lengthHistFromTable），这条实时路径只在
// 缓存缺失时降级使用：NAS 生产库 24,697,647 章时它要扫 5 遍索引，实测 3.35 s。
func (s *Store) bucketMetric(ctx context.Context, r *HealthReport, name string,
	labels []string, edges []int, column, table string) []Bucket {
	counts, err := bucketCountsOn(ctx, s.db, column, table, edges)
	if err != nil {
		r.warn(name, err)
		return nil
	}
	return makeBuckets(labels, counts)
}

// lengthBuckets 取章节长度直方图：优先读预计算表，缺失时降级为实时区间 COUNT。
//
// 返回 live=true 表示这次是**实时算的**（预计算不可用），调用方据此把报告标 stale——
// 数字是对的，只是慢，而这一次慢必须让运维看见（否则「为什么今天驾驶舱这么卡」
// 会变成一个没有线索的调查）。
func (s *Store) lengthBuckets(ctx context.Context, r *HealthReport) (buckets []Bucket, live bool) {
	if ok, _ := s.PrecomputeStatus(ctx); ok {
		counts, err := s.lengthHistFromTable(ctx)
		if err == nil {
			return makeBuckets(lengthBucketLabels, counts), false
		}
		r.warn("chapterLengthBuckets", err)
		return nil, false
	}
	return s.bucketMetric(ctx, r, "chapterLengthBuckets",
		lengthBucketLabels, lengthBucketEdges, "word_count", "chapters"), true
}

// makeBuckets 把各桶计数配上标签（标签与计数长度必然一致，都由同一份 edges 派生）。
func makeBuckets(labels []string, counts []int) []Bucket {
	out := make([]Bucket, len(labels))
	for i := range labels {
		out[i] = Bucket{Label: labels[i], Count: counts[i]}
	}
	return out
}

// bucketCountsOn 用一个 SELECT 拿到各桶计数（第 i 个结果列 = 第 i 桶）。
//
// 接受 queryer（*sql.DB / *sql.Tx）而不是绑死 s.db：0022 的回填要在迁移事务里
// 算同一份直方图，必须读到自己刚提交前的数据（换连接读会读到旧快照）。
func bucketCountsOn(ctx context.Context, q queryer, column, table string, edges []int) ([]int, error) {
	args := make([]any, 0, len(edges)*2)
	for i := range edges {
		args = append(args, edges[i])
		if i+1 < len(edges) {
			args = append(args, edges[i+1])
		}
	}
	counts := make([]int, len(edges))
	row := q.QueryRowContext(ctx, bucketRangeSQL(column, table, edges), args...)
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

// ImportTrend 的实现在 dashboard_read.go（0022 起优先读 import_daily）。

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

// trendCounts 按本地日期聚合书数与章节数，返回两张 date→count 表（降级路径）。
//
// 章节不再 JOIN 24,697,647 行的 chapters 逐行 parseTime：章节数在 books.chapter_count
// 上本来就有，按书归日再累加即可。NAS 生产库实测从 28.85 s 降到亚秒级。
func (s *Store) trendCounts(ctx context.Context, start time.Time) (map[string]int, map[string]int, error) {
	return s.countByDayFromBooks(ctx,
		"SELECT imported_at, chapter_count FROM books WHERE imported_at >= ?", start)
}

// countByDayFromBooks 执行「取 (时间文本, 章节数)」的查询并按本地日期聚合。
//
// 时间文本由 store 统一写成 UTC（YYYY-MM-DDTHH:MM:SS.sssZ），
// 这里解析后转本地时区再取日期，避免 UTC 跨日把凌晨导入算到前一天。
// 章节数直接取每行的 chapter_count（books 表 7 万行），不再为每一章查一次归属。
func (s *Store) countByDayFromBooks(ctx context.Context, query string, args ...any) (map[string]int, map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("store: import trend: %w", err)
	}
	defer rows.Close()

	books := make(map[string]int, 32)
	chapters := make(map[string]int, 32)
	for rows.Next() {
		var (
			raw          sql.NullString
			chapterCount int
		)
		if err := rows.Scan(&raw, &chapterCount); err != nil {
			return nil, nil, fmt.Errorf("store: scan import trend: %w", err)
		}
		if !raw.Valid || raw.String == "" {
			continue
		}
		t, err := parseTime(raw.String)
		if err != nil {
			continue // 历史脏数据不影响趋势图
		}
		day := t.Local().Format(dateLayout)
		books[day]++
		chapters[day] += chapterCount
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("store: import trend rows: %w", err)
	}
	return books, chapters, nil
}

var _ = errors.Is
