package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
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
	// warnMu 必须在这里建好，不能在 warn() 里懒初始化：下面 7 个 goroutine 会并发写
	// Warnings，懒初始化会让每个失败者各建一把锁（并发写 r.warnMu 字段本身也是数据竞态），
	// 然后用不同的锁保护同一份 slice 并发 append——实测能把进程直接带走。
	r := HealthReport{Warnings: []string{}, warnMu: &sync.Mutex{}}
	// 预计算快照：0025 的书级健康计数 + 0026 的库级总量（书数 / 书目字数 / 已算质量分）。
	// 可用时这些指标都只读个位数行的小表；不可用（未迁移或回填未完成）时回退实时 SQL，
	// 数字仍然正确，只是慢——降级绝不返回错数字，也不 500。
	pre := s.healthPrecompute(ctx)
	introWhere := s.missingIntroWhere(ctx)
	lengthLive := s.runBookMetrics(ctx, &r, pre, introWhere)

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
	r.Fingerprints = s.fingerprintHealthWithBackfill(ctx, &r, pre)
	// 质量分覆盖率：0020 之后版本择优要靠 backfill-quality 补齐历史书，缺口同样必须可见。
	// 0026 起「书数 / 已算书数」读预计算表，不再每次请求扫一遍 18 万本。
	r.Quality = s.qualityHealth(ctx, &r, pre)
	return r
}

// precomputedInt 优先返回 0025 的预计算值；不可用时回退实时 SQL（数字同一口径）。
func (s *Store) precomputedInt(ctx context.Context, r *HealthReport, name string,
	ready bool, value int, query string) *int {
	if ready {
		v := value
		return &v
	}
	return s.intMetric(ctx, r, name, query)
}

// chapterBuckets 取章节数分桶：优先 0025 的预计算表，缺失时回退实时区间 COUNT。
//
// 回退路径就是 0022 之前的老实现（六段区间 COUNT 走 idx_books_chapters），
// 生产 169,230 本实测 324 ms——慢但正确，只在「迁移刚跑完、后台回填还没完成」的窗口里出现。
func (s *Store) chapterBuckets(ctx context.Context, r *HealthReport,
	counts bookHealthCounts, ready bool) []Bucket {
	if ready && len(counts.Buckets) == len(chapterBucketLabels) {
		return makeBuckets(chapterBucketLabels, counts.Buckets)
	}
	return s.bucketMetric(ctx, r, "chapterBuckets",
		chapterBucketLabels, chapterBucketEdges, "chapter_count", "books")
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

// warnFallbackMu 是零值 HealthReport 的兜底锁（包级、只读共享、永不写回字段）。
//
// 为什么需要：HealthReport 由 HealthReport() 建结构体时就把 warnMu 建好了，但结构体是
// 可导出的，包内测试会直接构造 HealthReport{} 调 warn（composition_test.go）。
// 兜底只**读** r.warnMu、不写它，因此不存在「多个 goroutine 各建一把锁」的竞态——
// 这正是之前懒初始化的病灶（读 + 写 r.warnMu 字段本身即数据竞态）。
var warnFallbackMu sync.Mutex

// warn 在线程安全的前提下追加一条 Warnings。
//
// 锁由 HealthReport 在建结构体时一次建好（warnMu 是**指针**字段：HealthReport 按值返回、
// writeJSON 还会再复制一次，值类型的 sync.Mutex 会触发 go vet copylocks）。
func (r *HealthReport) warn(name string, err error) {
	mu := r.warnMu
	if mu == nil {
		mu = &warnFallbackMu //nolint:staticcheck // 零值结构体的兜底：只读字段，不写回
	}
	mu.Lock()
	defer mu.Unlock()
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
