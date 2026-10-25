package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// 系列聚合与「按系列/评分筛书」的口径：系列按 TRIM(series) 分组、
// 只统计非空系列、按最近更新倒序；评分 0 不参与筛选与排序可见性。

func TestListSeriesAggregatesAndFilters(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := seedSeriesBook(t, ctx, st, 1, "系列甲之一", "示例系列", 2, 100)
	second := seedSeriesBook(t, ctx, st, 2, "系列甲之二", "示例系列", 1, 50)
	other := seedSeriesBook(t, ctx, st, 3, "系列乙", "另一系列", 0, 70)
	seedSeriesBook(t, ctx, st, 4, "没有系列", "", 0, 10)

	// 存量脏数据：库里可能带首尾空格，聚合必须按 SQL TRIM 归到同一个系列。
	mustExec(t, ctx, st, fmt.Sprintf("UPDATE books SET series = ' 示例系列 ' WHERE id = %d", second))
	if err := st.SaveCover(ctx, first, "image/png", []byte{0x89, 'P', 'N', 'G'}); err != nil {
		t.Fatalf("SaveCover: %v", err)
	}
	mustExec(t, ctx, st, fmt.Sprintf(
		"UPDATE books SET updated_at = '2026-10-01T00:00:00.000Z' WHERE id IN (%d, %d)", first, second))
	mustExec(t, ctx, st, fmt.Sprintf(
		"UPDATE books SET updated_at = '2026-10-02T00:00:00.000Z' WHERE id = %d", other))

	items, total, err := st.ListSeries(ctx, "", 1, 10)
	if err != nil {
		t.Fatalf("ListSeries: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d，期望 2（空系列不计）", total)
	}
	if len(items) != 2 || items[0].Name != "另一系列" || items[1].Name != "示例系列" {
		t.Fatalf("系列顺序 = %+v，期望按最近更新倒序", items)
	}
	got := items[1]
	if got.BookCount != 2 || got.WordCount != 150 {
		t.Errorf("示例系列 = %+v，期望 2 本 150 字", got)
	}
	if got.LastUpdatedAt != "2026-10-01T00:00:00.000Z" {
		t.Errorf("LastUpdatedAt = %q", got.LastUpdatedAt)
	}
	if got.CoverBookID != first {
		t.Errorf("CoverBookID = %d，期望有封面的 %d", got.CoverBookID, first)
	}
	if items[0].CoverBookID != other {
		t.Errorf("无封面系列应退化为最小 id %d，实际 %d", other, items[0].CoverBookID)
	}

	// q 过滤 + 第二页（size=1）只应看到唯一命中的那一行。
	filtered, total, err := st.ListSeries(ctx, "另一", 1, 10)
	if err != nil || total != 1 || len(filtered) != 1 || filtered[0].Name != "另一系列" {
		t.Errorf("q 过滤 = %+v total=%d err=%v", filtered, total, err)
	}
	page2, total, err := st.ListSeries(ctx, "", 2, 1)
	if err != nil || total != 2 || len(page2) != 1 || page2[0].Name != "示例系列" {
		t.Errorf("第二页 = %+v total=%d err=%v", page2, total, err)
	}
}

func TestSeriesBooksOrderAndErrors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	later := seedSeriesBook(t, ctx, st, 1, "系列内第二册", "示例系列", 3, 20)
	earlier := seedSeriesBook(t, ctx, st, 2, "系列内第一册", "示例系列", 1, 20)

	books, err := st.SeriesBooks(ctx, "示例系列")
	if err != nil {
		t.Fatalf("SeriesBooks: %v", err)
	}
	if len(books) != 2 || books[0].ID != earlier || books[1].ID != later {
		t.Fatalf("系列书顺序 = %+v，期望按 seriesIndex 升序", books)
	}
	if books[0].Title != "系列内第一册" || books[0].SeriesIndex != 1 || books[0].Series != "示例系列" {
		t.Errorf("BookBrief 字段不全: %+v", books[0])
	}
	// 前后空格按同一系列处理。
	if got, err := st.SeriesBooks(ctx, " 示例系列 "); err != nil || len(got) != 2 {
		t.Errorf("带空格系列名 = %d 本 err=%v", len(got), err)
	}
	if _, err := st.SeriesBooks(ctx, "不存在的系列"); !errors.Is(err, ErrNotFound) {
		t.Errorf("未知系列 err = %v，期望 ErrNotFound", err)
	}
	if _, err := st.SeriesBooks(ctx, "   "); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空系列名 err = %v，期望 ErrInvalidArgument", err)
	}
}

func TestListBooksSeriesAndRatingFilters(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	low := seedSeriesBook(t, ctx, st, 1, "系列甲低分", "示例系列", 1, 10)
	high := seedSeriesBook(t, ctx, st, 2, "系列甲高分", "示例系列", 2, 10)
	other := seedSeriesBook(t, ctx, st, 3, "别的书", "另一系列", 0, 10)
	for id, rating := range map[int64]int{low: 3, high: 5, other: 4} {
		if err := st.SetBookRating(ctx, id, rating); err != nil {
			t.Fatalf("SetBookRating(%d): %v", id, err)
		}
	}

	bySeries, err := st.ListBooks(ctx, BookQuery{Series: "示例系列", Page: 1, Size: 10})
	if err != nil || bySeries.Total != 2 {
		t.Fatalf("按系列筛选 = %+v err=%v", bySeries, err)
	}
	byRating, err := st.ListBooks(ctx, BookQuery{Rating: 4, Page: 1, Size: 10})
	if err != nil || byRating.Total != 1 || byRating.Items[0].ID != other {
		t.Fatalf("按评分筛选 = %+v err=%v", byRating, err)
	}
	sorted, err := st.ListBooks(ctx, BookQuery{Sort: "rating", Page: 1, Size: 10})
	if err != nil || sorted.Total != 3 {
		t.Fatalf("按评分排序 = %+v err=%v", sorted, err)
	}
	if sorted.Items[0].ID != high || sorted.Items[1].ID != other || sorted.Items[2].ID != low {
		t.Errorf("评分排序 = %v，期望 5 → 4 → 3", []int64{
			sorted.Items[0].ID, sorted.Items[1].ID, sorted.Items[2].ID})
	}
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
