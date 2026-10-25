package store

import (
	"context"
	"testing"
)

// TestBucketHistogramSQLMatchesBucketIndex 校验 SQL 层分桶与 Go 侧 bucketIndex 语义一致。
//
// 这是分桶从「Go 逐行」迁到「SQL 聚合」后最关键的一致性保证：
// 两边对同一边界表必须给出同一桶号，否则驾驶舱的两张直方图会静默错位。
func TestBucketHistogramSQLMatchesBucketIndex(t *testing.T) {
	cases := []struct {
		name   string
		edges  []int
		values []int
	}{
		{"章节数桶", chapterBucketEdges, []int{0, 1, 2, 9, 10, 49, 50, 199, 200, 999, 1000, 5000}},
		{"长度桶", lengthBucketEdges, []int{0, 1, 999, 1000, 4999, 5000, 19999, 20000, 99999, 100000, 500000}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, v := range c.values {
				want := bucketIndex(c.edges, v)
				expr := bucketCaseExpr(itoa(v), c.edges)
				got := evalBucketCase(t, expr)
				if got != want {
					t.Errorf("v=%d：SQL 桶号 %d，Go 桶号 %d", v, got, want)
				}
			}
		})
	}
}

// evalBucketCase 用 SQLite 求值 CASE 表达式（列名位置换成字面量，故无需建表）。
func evalBucketCase(t *testing.T, expr string) int {
	t.Helper()
	st := newTestStore(t)
	var got int
	if err := st.DB().QueryRowContext(context.Background(), "SELECT "+expr).Scan(&got); err != nil {
		t.Fatalf("求值 %s: %v", expr, err)
	}
	return got
}

// itoa 是测试内的整数转字符串（避免为一处转换引入 strconv）。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestBucketMetricUsesSingleAggregate 校验分桶统计的总数守恒（每行恰好落入一个桶）。
func TestBucketMetricUsesSingleAggregate(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)

	r := st.HealthReport(ctx)
	if len(r.Warnings) != 0 {
		t.Fatalf("HealthReport 有告警: %v", r.Warnings)
	}
	if got := sumBuckets(r.ChapterBuckets); got != 2 {
		t.Errorf("章节数分桶合计 = %d，期望 2（两本书）", got)
	}
	if got := sumBuckets(r.ChapterLengthBuckets); got != 6 {
		t.Errorf("长度分桶合计 = %d，期望 6（六章）", got)
	}
}

// TestMetricsUpdatedBySaveBook 校验导入（SaveBook）会自动累加 store_metrics 的聚合值。
func TestMetricsUpdatedBySaveBook(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)

	snap, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if snap.Stale {
		t.Errorf("写入后 metrics 不应过期: %+v", snap)
	}

	// 与全表精确值逐项比对（口径：字符数，不是字节数；正文含 chapters + books 两侧）。
	wantText, err := exactTextBytes(ctx, st)
	if err != nil {
		t.Fatalf("精确文本: %v", err)
	}
	wantWords, err := st.scalarInt64(ctx, chapterWordSumSQL)
	if err != nil {
		t.Fatalf("精确字数: %v", err)
	}
	if snap.TextBytes != wantText {
		t.Errorf("TextBytes = %d，全表精确值 %d", snap.TextBytes, wantText)
	}
	if snap.ChapterWordSum != wantWords {
		t.Errorf("ChapterWordSum = %d，全表精确值 %d", snap.ChapterWordSum, wantWords)
	}
}

// exactTextBytes 返回全表精确的正文字符数（chapters 侧 + books 侧）。
func exactTextBytes(ctx context.Context, st *Store) (int64, error) {
	chapters, err := st.scalarInt64(ctx, chapterTextBytesSQL)
	if err != nil {
		return 0, err
	}
	books, err := st.scalarInt64(ctx, bookTextBytesSQL)
	if err != nil {
		return 0, err
	}
	return chapters + books, nil
}

// TestMetricsForceOverwriteNoDoubleCount 校验覆盖写入（Force）不会重复计数。
func TestMetricsForceOverwriteNoDoubleCount(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := sampleBook("覆盖测试", "作者", "/books/force.txt", repeat64("z"))
	if _, err := st.SaveBook(ctx, book, SaveOptions{}); err != nil {
		t.Fatalf("首次保存: %v", err)
	}
	before, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}

	if _, err := st.SaveBook(ctx, book, SaveOptions{Force: true}); err != nil {
		t.Fatalf("强制覆盖: %v", err)
	}
	after, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if after.TextBytes != before.TextBytes || after.ChapterWordSum != before.ChapterWordSum {
		t.Errorf("覆盖写入后重复计数：前 %+v，后 %+v", before, after)
	}

	// 并且与全表精确值仍然一致。
	wantText, err := exactTextBytes(ctx, st)
	if err != nil {
		t.Fatalf("精确值: %v", err)
	}
	if after.TextBytes != wantText {
		t.Errorf("覆盖后 TextBytes = %d，精确值 %d", after.TextBytes, wantText)
	}
}

// TestMetricsRefreshMatchesIncremental 校验全量重算与增量累加得到同一个数。
func TestMetricsRefreshMatchesIncremental(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)

	incremental, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if err := st.RefreshMetrics(ctx); err != nil {
		t.Fatalf("RefreshMetrics: %v", err)
	}
	refreshed, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if refreshed.TextBytes != incremental.TextBytes {
		t.Errorf("重算 TextBytes = %d，增量累加 = %d", refreshed.TextBytes, incremental.TextBytes)
	}
	if refreshed.ChapterWordSum != incremental.ChapterWordSum {
		t.Errorf("重算 ChapterWordSum = %d，增量累加 = %d",
			refreshed.ChapterWordSum, incremental.ChapterWordSum)
	}
	if refreshed.Stale {
		t.Errorf("重算后不应过期")
	}
}

// TestMetricsStaleWhenEmpty 校验空 store_metrics（老库未迁移）时接口不报错且标注 stale。
func TestMetricsStaleWhenEmpty(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.clearMetrics(ctx); err != nil {
		t.Fatalf("清空 metrics: %v", err)
	}
	snap, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if !snap.Stale {
		t.Errorf("空表时应标注 stale")
	}
	comp, err := st.Composition(ctx)
	if err != nil {
		t.Fatalf("Composition: %v", err)
	}
	if !comp.Estimated {
		t.Errorf("stale 时 Composition.Estimated 应为 true")
	}
}

// TestEnsureCompositionIndexIdempotent 校验覆盖索引的幂等创建（迁移会重复调用）。
func TestEnsureCompositionIndexIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for i := 0; i < 3; i++ {
		if err := st.ensureCompositionIndex(ctx); err != nil {
			t.Fatalf("第 %d 次 ensureCompositionIndex: %v", i+1, err)
		}
	}
	var n int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_chapters_wc'").Scan(&n); err != nil {
		t.Fatalf("查索引: %v", err)
	}
	if n != 1 {
		t.Errorf("idx_chapters_wc 数量 = %d，期望 1", n)
	}
}

// TestEnsureCompositionIndexWithoutChaptersTable 校验 chapters 不存在时跳过建索引（老库路径）。
func TestEnsureCompositionIndexWithoutChaptersTable(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "nochapters.db")
	if err := st.ensureCompositionIndex(ctx); err != nil {
		t.Fatalf("无 chapters 表时应跳过而非报错: %v", err)
	}
}

// TestBucketMetricWarnsOnMissingTable 校验分桶查询失败时记 Warnings 并返回 nil。
func TestBucketMetricWarnsOnMissingTable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	r := HealthReport{Warnings: []string{}}
	got := st.bucketMetric(ctx, &r, "bogus", []string{"a", "b"}, []int{0, 1}, "word_count", "no_such_table")
	if got != nil {
		t.Errorf("查询失败时应返回 nil，实际 %+v", got)
	}
	if len(r.Warnings) != 1 {
		t.Errorf("应记 1 条 Warnings，实际 %v", r.Warnings)
	}
}

// TestBucketCaseExprSingleEdge 覆盖边界表只有 1 个元素时退化为常量 0。
func TestBucketCaseExprSingleEdge(t *testing.T) {
	if got := bucketCaseExpr("word_count", []int{0}); got != "0" {
		t.Errorf("单边界应返回 0，实际 %q", got)
	}
	if got := bucketCaseExpr("word_count", nil); got != "0" {
		t.Errorf("空边界应返回 0，实际 %q", got)
	}
}

// TestBucketHistogramSQLShape 校验生成的聚合语句形状（CASE 平铺 + GROUP BY）。
func TestBucketHistogramSQLShape(t *testing.T) {
	sql := bucketHistogramSQL("word_count", "chapters", []int{0, 1000, 5000})
	for _, want := range []string{
		"SELECT ", "CASE", "WHEN word_count >= 5000 THEN 2", "WHEN word_count >= 1000 THEN 1",
		"ELSE 0 END", "COUNT(*)", "FROM chapters", "GROUP BY bucket",
	} {
		if !contains(sql, want) {
			t.Errorf("生成的 SQL 缺少 %q：%s", want, sql)
		}
	}
}

// contains 是测试内的子串判断。
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// clearMetrics 清空预计算表（模拟老库未算过的状态）。
func (s *Store) clearMetrics(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM store_metrics")
	return err
}
