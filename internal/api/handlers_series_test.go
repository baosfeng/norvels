package api

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"norvels/internal/store"
)

// seedSeriesBook 写入一本书并设置系列名/系列内序号（系列端点测试用）。
func seedSeriesBook(t *testing.T, e *testEnv, title, series string, idx int) int64 {
	t.Helper()
	id := e.seedBook(title, "作者甲")
	if err := e.st.UpdateBookMeta(context.Background(), id,
		store.BookMetaPatch{Series: &series, SeriesIndex: &idx}); err != nil {
		t.Fatalf("UpdateBookMeta: %v", err)
	}
	return id
}

func TestListSeriesAggregates(t *testing.T) {
	e := newEnv(t)
	seedSeriesBook(t, e, "甲书", "测试系列", 1)
	seedSeriesBook(t, e, "乙书", "测试系列", 2)
	e.seedBook("散书", "作者乙") // 无系列：不应出现在列表里

	rr := e.get("/api/series")
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[seriesListResponse](t, rr)
	if body.Total != 1 || len(body.Items) != 1 {
		t.Fatalf("系列列表 = %+v", body)
	}
	got := body.Items[0]
	if got.Name != "测试系列" || got.BookCount != 2 || got.CoverBookID <= 0 {
		t.Fatalf("系列聚合 = %+v", got)
	}
}

func TestListSeriesEmpty(t *testing.T) {
	e := newEnv(t)
	e.seedBook("散书", "作者乙") // 唯一一本没有系列

	rr := e.get("/api/series")
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[seriesListResponse](t, rr)
	if body.Total != 0 || body.Items == nil || len(body.Items) != 0 {
		t.Fatalf("空结果 = %+v", body)
	}
}

func TestGetSeriesBooks(t *testing.T) {
	e := newEnv(t)
	seedSeriesBook(t, e, "甲书", "测试系列", 2)
	seedSeriesBook(t, e, "乙书", "测试系列", 1)

	rr := e.get("/api/series/" + url.PathEscape("测试系列"))
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[seriesDetailResponse](t, rr)
	if body.Name != "测试系列" || len(body.Books) != 2 {
		t.Fatalf("系列详情 = %+v", body)
	}
	if body.Books[0].Title != "乙书" {
		t.Fatalf("应按系列序号升序，首本 = %q", body.Books[0].Title)
	}
}

func TestGetSeriesNotFound(t *testing.T) {
	e := newEnv(t)
	rr := e.get("/api/series/" + url.PathEscape("不存在的系列"))
	requireError(t, rr, http.StatusNotFound, codeNotFound)
}
