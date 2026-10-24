package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// seedTwoBooks 建两本书（含章节）用于驾驶舱相关查询。
func seedTwoBooks(t *testing.T, st *Store) (bookID, chapter1, chapter2 int64) {
	t.Helper()
	ctx := context.Background()
	res, err := st.SaveBook(ctx, sampleBook("驾驶舱一号", "作者甲", "/books/dash1.txt", repeat64("a")), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook 1: %v", err)
	}
	if _, err := st.SaveBook(ctx, sampleBook("驾驶舱二号", "作者乙", "/books/dash2.txt", repeat64("b")), SaveOptions{}); err != nil {
		t.Fatalf("SaveBook 2: %v", err)
	}
	chs, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, Limit: 10})
	if err != nil || len(chs.Items) < 2 {
		t.Fatalf("ListChapters: %v items=%d", err, len(chs.Items))
	}
	return res.BookID, chs.Items[0].ID, chs.Items[1].ID
}

// repeat64 生成 64 位重复字符（满足 file_sha256 长度约束）。
func repeat64(ch string) string {
	out := make([]byte, 64)
	for i := range out {
		out[i] = ch[0]
	}
	return string(out)
}

// TestReadingProgressListAndUpsert 覆盖写入、联表读取与 upsert。
func TestReadingProgressListAndUpsert(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ch1, ch2 := seedTwoBooks(t, st)

	assertProgressEmpty(t, st)
	requireSaveProgress(t, st, ReadingProgress{BookID: bookID, ChapterID: ch1, Percent: 0.3})
	assertProgressFields(t, st, bookID, ch1, 0.3)

	// upsert：同一本书换章节不新增行。
	requireSaveProgress(t, st, ReadingProgress{BookID: bookID, ChapterID: ch2, Percent: 0.9})
	items, err := st.ListReadingProgress(ctx, 5)
	if err != nil {
		t.Fatalf("ListReadingProgress: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("upsert 后行数 = %d，期望 1", len(items))
	}
	if items[0].ChapterID != ch2 || items[0].Percent != 0.9 {
		t.Errorf("upsert 未更新: %+v", items[0])
	}
}

// assertProgressEmpty 校验空表返回 0 条（不是 nil）。
func assertProgressEmpty(t *testing.T, st *Store) {
	t.Helper()
	items, err := st.ListReadingProgress(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListReadingProgress: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("空表 items = %d，期望 0", len(items))
	}
}

// requireSaveProgress 写一条进度并要求成功。
func requireSaveProgress(t *testing.T, st *Store, p ReadingProgress) {
	t.Helper()
	if err := st.SaveReadingProgress(context.Background(), p); err != nil {
		t.Fatalf("SaveReadingProgress(%+v): %v", p, err)
	}
}

// assertProgressFields 校验单条进度及其联表字段。
func assertProgressFields(t *testing.T, st *Store, bookID, chapterID int64, percent float64) {
	t.Helper()
	items, err := st.ListReadingProgress(context.Background(), 5)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListReadingProgress = %d items, err=%v", len(items), err)
	}
	got := items[0]
	if got.BookID != bookID || got.ChapterID != chapterID || got.Percent != percent {
		t.Errorf("进度 = %+v", got)
	}
	if got.BookTitle != "驾驶舱一号" || got.ChapterTitle == "" {
		t.Errorf("联表字段缺失: book=%q chapter=%q", got.BookTitle, got.ChapterTitle)
	}
}

// TestReadingProgressRejectsBadInput 覆盖入参校验与错误分类。
func TestReadingProgressRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ch1, _ := seedTwoBooks(t, st)

	cases := []struct {
		name string
		p    ReadingProgress
		want error
	}{
		{"bookId=0", ReadingProgress{BookID: 0, ChapterID: ch1}, ErrInvalidArgument},
		{"chapterId=0", ReadingProgress{BookID: bookID, ChapterID: 0}, ErrInvalidArgument},
		{"percent<0", ReadingProgress{BookID: bookID, ChapterID: ch1, Percent: -0.1}, ErrInvalidArgument},
		{"percent>1", ReadingProgress{BookID: bookID, ChapterID: ch1, Percent: 1.2}, ErrInvalidArgument},
		{"章节不存在", ReadingProgress{BookID: bookID, ChapterID: 999999}, ErrNotFound},
		{"章节不属于该书", ReadingProgress{BookID: bookID + 1000, ChapterID: ch1}, ErrNotFound},
	}
	for _, c := range cases {
		if err := st.SaveReadingProgress(ctx, c.p); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v，期望 %v", c.name, err, c.want)
		}
	}
}

// TestNormalizePercent 覆盖 percent 归一化。
func TestNormalizePercent(t *testing.T) {
	if _, ok := NormalizePercent(1.5); ok {
		t.Error("NormalizePercent(1.5) 应判非法")
	}
	if _, ok := NormalizePercent(-0.1); ok {
		t.Error("NormalizePercent(-0.1) 应判非法")
	}
	if v, ok := NormalizePercent(0.5); !ok || v != 0.5 {
		t.Errorf("NormalizePercent(0.5) = %v,%v", v, ok)
	}
}

// TestReadingProgressCascadeOnBookDelete 校验删书级联清进度。
func TestReadingProgressCascadeOnBookDelete(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ch1, _ := seedTwoBooks(t, st)
	requireSaveProgress(t, st, ReadingProgress{BookID: bookID, ChapterID: ch1, Percent: 0.5})

	if _, err := st.DB().ExecContext(ctx, "DELETE FROM books WHERE id = ?", bookID); err != nil {
		t.Fatalf("删除书: %v", err)
	}
	var n int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM reading_progress").Scan(&n); err != nil {
		t.Fatalf("统计进度: %v", err)
	}
	if n != 0 {
		t.Errorf("删书后进度残留 %d 行（外键 CASCADE 未生效）", n)
	}
}

// TestHealthReportMetrics 覆盖健康报告的标量指标。
func TestHealthReportMetrics(t *testing.T) {
	st := newTestStore(t)
	seedTwoBooks(t, st)
	r := st.HealthReport(context.Background())

	if len(r.Warnings) != 0 {
		t.Fatalf("健康报告不应有 warnings: %v", r.Warnings)
	}
	assertIntPtr(t, "totalBooks", r.TotalBooks, 2)
	assertIntPtr(t, "missingIntro", r.MissingIntro, 0)
	assertIntPtr(t, "missingAuthor", r.MissingAuthor, 0)
}

// TestHealthReportBuckets 覆盖两张直方图分桶。
func TestHealthReportBuckets(t *testing.T) {
	st := newTestStore(t)
	seedTwoBooks(t, st)
	r := st.HealthReport(context.Background())

	if len(r.ChapterBuckets) != len(chapterBucketLabels) {
		t.Fatalf("章节分桶数 = %d，期望 %d", len(r.ChapterBuckets), len(chapterBucketLabels))
	}
	if got := sumBuckets(r.ChapterBuckets); got != 2 {
		t.Errorf("章节分桶合计 = %d，期望 2", got)
	}
	if got := sumBuckets(r.ChapterLengthBuckets); got != 6 {
		t.Errorf("章节长度分桶合计 = %d，期望 6", got)
	}
	// 每本书 3 章 → 落在 "2-9" 桶。
	if r.ChapterBuckets[1].Label != "2-9" || r.ChapterBuckets[1].Count != 2 {
		t.Errorf("2-9 桶 = %+v", r.ChapterBuckets[1])
	}
}

// assertIntPtr 断言 *int 指标等于期望值。
func assertIntPtr(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = nil，期望 %d", name, want)
		return
	}
	if *got != want {
		t.Errorf("%s = %d，期望 %d", name, *got, want)
	}
}

// sumBuckets 汇总各桶计数。
func sumBuckets(bs []Bucket) int {
	n := 0
	for _, b := range bs {
		n += b.Count
	}
	return n
}

// TestBucketIndex 覆盖分桶边界。
func TestBucketIndex(t *testing.T) {
	cases := []struct{ v, want int }{
		{0, 0}, {1, 0}, {2, 1}, {9, 1}, {10, 2}, {49, 2}, {50, 3},
		{199, 3}, {200, 4}, {999, 4}, {1000, 5}, {99999, 5},
	}
	for _, c := range cases {
		if got := bucketIndex(chapterBucketEdges, c.v); got != c.want {
			t.Errorf("bucketIndex(%d) = %d，期望 %d", c.v, got, c.want)
		}
	}
	if got := bucketIndex(lengthBucketEdges, 0); got != 0 {
		t.Errorf("bucketIndex(length,0) = %d", got)
	}
}

// TestImportTrendFillsMissingDays 覆盖连续日期轴与当天计数。
func TestImportTrendFillsMissingDays(t *testing.T) {
	st := newTestStore(t)
	seedTwoBooks(t, st)
	points := requireTrend(t, st, 7)

	assertDatesContinuous(t, points)
	today := points[len(points)-1]
	if today.Books != 2 || today.Chapters != 6 {
		t.Errorf("今天 = %+v，期望 books=2 chapters=6", today)
	}
	assertTrendZeroFilled(t, points[:len(points)-1])
}

// requireTrend 取趋势数据并要求成功。
func requireTrend(t *testing.T, st *Store, days int) []ImportTrendPoint {
	t.Helper()
	points, err := st.ImportTrend(context.Background(), days)
	if err != nil {
		t.Fatalf("ImportTrend: %v", err)
	}
	if len(points) != days {
		t.Fatalf("天数 = %d，期望 %d", len(points), days)
	}
	return points
}

// assertDatesContinuous 校验日期逐日递增且末位是今天。
func assertDatesContinuous(t *testing.T, points []ImportTrendPoint) {
	t.Helper()
	for i := 1; i < len(points); i++ {
		prev, _ := time.Parse(dateLayout, points[i-1].Date)
		cur, _ := time.Parse(dateLayout, points[i].Date)
		if cur.Sub(prev) != 24*time.Hour {
			t.Errorf("日期不连续: %s → %s", points[i-1].Date, points[i].Date)
		}
	}
	if last := points[len(points)-1].Date; last != time.Now().Format(dateLayout) {
		t.Errorf("最后一天 = %s，期望今天", last)
	}
}

// assertTrendZeroFilled 校验无数据日期补 0。
func assertTrendZeroFilled(t *testing.T, points []ImportTrendPoint) {
	t.Helper()
	for _, p := range points {
		if p.Books != 0 || p.Chapters != 0 {
			t.Errorf("%s 应有 0 值补位，实际 %+v", p.Date, p)
		}
	}
}

// TestImportTrendClampsDays 覆盖天数缺省（0 → 14）与上限（一年）。
func TestImportTrendClampsDays(t *testing.T) {
	st := newTestStore(t)
	pts, err := st.ImportTrend(context.Background(), 0)
	if err != nil {
		t.Fatalf("ImportTrend(0): %v", err)
	}
	if len(pts) != defaultTrendDaysInTest {
		t.Errorf("days=0 → %d 条，期望 %d", len(pts), defaultTrendDaysInTest)
	}
	// 上限夹紧：请求量超出上限时按 maxTrendDays 返回。
	clamped, err := st.ImportTrend(context.Background(), 100000)
	if err != nil {
		t.Fatalf("ImportTrend(100000): %v", err)
	}
	if len(clamped) != maxTrendDays {
		t.Errorf("days 超大 → %d 条，期望上限 %d", len(clamped), maxTrendDays)
	}
}

// defaultTrendDaysInTest 与 api 层默认值一致（14）。
const defaultTrendDaysInTest = 14

// TestStatsCompositionSumsToTotal 覆盖体积构成之和恒等于总大小。
func TestStatsCompositionSumsToTotal(t *testing.T) {
	st := newTestStore(t)
	seedTwoBooks(t, st)
	s, err := st.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if s.DBBytes <= 0 {
		t.Fatalf("DBBytes = %d，应为正", s.DBBytes)
	}
	if sum := s.TextBytes + s.IndexBytes + s.OtherBytes; sum != s.DBBytes {
		t.Errorf("体积构成之和 %d != DBBytes %d", sum, s.DBBytes)
	}
	if s.TextBytes <= 0 {
		t.Errorf("TextBytes = %d，正文非空库应大于 0", s.TextBytes)
	}
	if s.OtherBytes < 0 {
		t.Errorf("OtherBytes = %d，不应为负", s.OtherBytes)
	}
}

// TestFillCompositionClampsOversize 覆盖估算超出实际占用时的按比例压缩。
func TestFillCompositionClampsOversize(t *testing.T) {
	st := Stats{DBBytes: 1000}
	fillComposition(&st, 800, 400) // 合计 1200 > 1000
	if sum := st.TextBytes + st.IndexBytes + st.OtherBytes; sum != st.DBBytes {
		t.Errorf("压缩后合计 = %d，期望 %d", sum, st.DBBytes)
	}
	if st.OtherBytes < 0 {
		t.Errorf("压缩后 OtherBytes = %d", st.OtherBytes)
	}
}

// TestFillCompositionEdgeCases 覆盖全零估算与 DBBytes=0。
func TestFillCompositionEdgeCases(t *testing.T) {
	zero := Stats{DBBytes: 500}
	fillComposition(&zero, 0, 0)
	if zero.OtherBytes != 500 {
		t.Errorf("全空库 otherBytes = %d，期望 500", zero.OtherBytes)
	}
	noSize := Stats{}
	fillComposition(&noSize, 10, 20)
	if noSize.TextBytes != 10 || noSize.IndexBytes != 20 {
		t.Errorf("DBBytes=0 时应原样保留估算: %+v", noSize)
	}
}

// TestAuthorSortByWords 覆盖按字数排序。
func TestAuthorSortByWords(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)
	big := sampleBook("字数之王", "作者丙", "/books/big.txt", repeat64("c"))
	big.WordCount = 999999
	if _, err := st.SaveBook(ctx, big, SaveOptions{}); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	res, err := st.ListAuthors(ctx, AuthorQuery{Sort: "words", Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListAuthors: %v", err)
	}
	if len(res.Items) == 0 || res.Items[0].Name != "作者丙" {
		t.Fatalf("按字数排序首位 = %+v，期望 作者丙", res.Items)
	}
	if res.Items[0].WordCount != 999999 {
		t.Errorf("首位字数 = %d，期望 999999", res.Items[0].WordCount)
	}
}

// TestAuthorSortByName 覆盖按名排序仍可用。
func TestAuthorSortByName(t *testing.T) {
	st := newTestStore(t)
	seedTwoBooks(t, st)
	byName, err := st.ListAuthors(context.Background(), AuthorQuery{Sort: "name", Page: 1, Size: 10})
	if err != nil || len(byName.Items) == 0 {
		t.Fatalf("按名排序: %v items=%d", err, len(byName.Items))
	}
}
