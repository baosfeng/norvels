package store

import (
	"context"
	"fmt"
	"testing"
)

// 系列聚合与「按系列/评分筛书」的口径：系列按 TRIM(series) 分组、
// 只统计非空系列、按最近更新倒序；评分 0 不参与筛选与排序可见性。

// seriesFixture 是系列测试的三本书：两本同系列（其中一本的库内 series 带首尾空格）、
// 一本另一系列；另有一本没有系列。
type seriesFixture struct {
	first  int64 // 示例系列，有封面
	second int64 // 示例系列，库里 series 带首尾空格
	other  int64 // 另一系列
}

func seedSeriesFixture(t *testing.T, ctx context.Context, st *Store) seriesFixture {
	t.Helper()
	f := seriesFixture{
		first:  seedSeriesBook(t, ctx, st, 1, "系列甲之一", "示例系列", 2, 100),
		second: seedSeriesBook(t, ctx, st, 2, "系列甲之二", "示例系列", 1, 50),
		other:  seedSeriesBook(t, ctx, st, 3, "系列乙", "另一系列", 0, 70),
	}
	seedSeriesBook(t, ctx, st, 4, "没有系列", "", 0, 10)
	mustExec(t, ctx, st, fmt.Sprintf("UPDATE books SET series = ' 示例系列 ' WHERE id = %d", f.second))
	if err := st.SaveCover(ctx, f.first, "image/png", []byte{0x89, 'P', 'N', 'G'}); err != nil {
		t.Fatalf("SaveCover: %v", err)
	}
	mustExec(t, ctx, st, fmt.Sprintf(
		"UPDATE books SET updated_at = '2026-10-01T00:00:00.000Z' WHERE id IN (%d, %d)", f.first, f.second))
	mustExec(t, ctx, st, fmt.Sprintf(
		"UPDATE books SET updated_at = '2026-10-02T00:00:00.000Z' WHERE id = %d", f.other))
	return f
}

func TestListSeriesAggregates(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	f := seedSeriesFixture(t, ctx, st)

	items, total, err := st.ListSeries(ctx, "", 1, 10)
	requireOK(t, "ListSeries", err)
	requireEqual(t, "series total", total, 2) // 空系列不计
	requireEqual(t, "series rows", len(items), 2)
	requireEqual(t, "首行系列名（最近更新倒序）", items[0].Name, "另一系列")
	requireEqual(t, "示例系列书名数", items[1].BookCount, 2)
	requireEqual(t, "示例系列字数", items[1].WordCount, 150)
	requireEqual(t, "示例系列最近更新", items[1].LastUpdatedAt, "2026-10-01T00:00:00.000Z")
	requireEqual(t, "示例系列封面书", items[1].CoverBookID, f.first)
	requireEqual(t, "无封面系列封面书退化", items[0].CoverBookID, f.other)
}

func TestListSeriesFiltersAndPaging(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedSeriesFixture(t, ctx, st)

	filtered, total, err := st.ListSeries(ctx, "另一", 1, 10)
	requireOK(t, "ListSeries(q)", err)
	requireEqual(t, "过滤后 total", total, 1)
	requireEqual(t, "过滤后行数", len(filtered), 1)
	requireEqual(t, "过滤命中系列", filtered[0].Name, "另一系列")

	page2, total, err := st.ListSeries(ctx, "", 2, 1)
	requireOK(t, "ListSeries(page=2)", err)
	requireEqual(t, "分页 total", total, 2)
	requireEqual(t, "第二页行数", len(page2), 1)
	requireEqual(t, "第二页系列", page2[0].Name, "示例系列")
}

func TestSeriesBooksOrder(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	later := seedSeriesBook(t, ctx, st, 1, "系列内第二册", "示例系列", 3, 20)
	earlier := seedSeriesBook(t, ctx, st, 2, "系列内第一册", "示例系列", 1, 20)

	books, err := st.SeriesBooks(ctx, "示例系列")
	requireOK(t, "SeriesBooks", err)
	requireEqual(t, "系列书数", len(books), 2)
	requireEqual(t, "首本（seriesIndex 升序）", books[0].ID, earlier)
	requireEqual(t, "次本", books[1].ID, later)
	requireEqual(t, "首本标题", books[0].Title, "系列内第一册")
	requireEqual(t, "首本系列序号", books[0].SeriesIndex, 1)
	requireEqual(t, "首本系列名", books[0].Series, "示例系列")

	// 前后空格按同一系列处理。
	spaced, err := st.SeriesBooks(ctx, " 示例系列 ")
	requireOK(t, "SeriesBooks(空格)", err)
	requireEqual(t, "带空格系列名书数", len(spaced), 2)
}

func TestSeriesBooksErrors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedSeriesBook(t, ctx, st, 1, "系列内第一册", "示例系列", 1, 20)

	_, err := st.SeriesBooks(ctx, "不存在的系列")
	requireErr(t, "未知系列", err, ErrNotFound)
	_, err = st.SeriesBooks(ctx, "   ")
	requireErr(t, "空系列名", err, ErrInvalidArgument)
}

func TestListBooksSeriesFilter(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedSeriesFixture(t, ctx, st)

	bySeries, err := st.ListBooks(ctx, BookQuery{Series: "示例系列", Page: 1, Size: 10})
	requireOK(t, "ListBooks(series)", err)
	requireEqual(t, "按系列筛书", bySeries.Total, 2)
}

func TestListBooksRatingFilterAndSort(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	low := seedSeriesBook(t, ctx, st, 1, "系列甲低分", "示例系列", 1, 10)
	high := seedSeriesBook(t, ctx, st, 2, "系列甲高分", "示例系列", 2, 10)
	other := seedSeriesBook(t, ctx, st, 3, "别的书", "另一系列", 0, 10)
	requireOK(t, "SetBookRating low", st.SetBookRating(ctx, low, 3))
	requireOK(t, "SetBookRating high", st.SetBookRating(ctx, high, 5))
	requireOK(t, "SetBookRating other", st.SetBookRating(ctx, other, 4))

	byRating, err := st.ListBooks(ctx, BookQuery{Rating: 4, Page: 1, Size: 10})
	requireOK(t, "ListBooks(rating)", err)
	requireEqual(t, "按评分筛书", byRating.Total, 1)
	requireEqual(t, "评分命中的书", byRating.Items[0].ID, other)

	sorted, err := st.ListBooks(ctx, BookQuery{Sort: "rating", Page: 1, Size: 10})
	requireOK(t, "ListBooks(sort=rating)", err)
	requireEqual(t, "按评分排序总数", sorted.Total, 3)
	requireEqual(t, "评分第 1", sorted.Items[0].ID, high)
	requireEqual(t, "评分第 2", sorted.Items[1].ID, other)
	requireEqual(t, "评分第 3", sorted.Items[2].ID, low)
}

// seedSeriesBook 造一本示例书并设定系列/系列序号/字数，返回 id。
func seedSeriesBook(t *testing.T, ctx context.Context, st *Store, n int, title, series string,
	seriesIndex, words int) int64 {
	t.Helper()
	book := sampleBook(title, "示例作者", fmt.Sprintf("/path/to/txt/p3-series-%d.txt", n),
		fmt.Sprintf("%064d", n))
	book.Series, book.SeriesIndex, book.WordCount = series, seriesIndex, words
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook %q: %v", title, err)
	}
	return res.BookID
}
