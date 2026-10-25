package api

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"norvels/internal/store"
)

// bookPage 是 GET /api/books 的响应（items + total）。
type bookPage = store.Page[store.BookSummary]

func TestListBooksP3Filters(t *testing.T) {
	e := newEnv(t)
	first := e.seedBook("筛选甲", "作者甲")
	second := e.seedBook("筛选乙", "作者乙")
	requireStatus(t, e.patch(bookPath(first), `{"series":"示例系列","rating":5}`), http.StatusOK)
	requireStatus(t, e.patch(bookPath(second), `{"series":"示例系列","rating":3}`), http.StatusOK)

	series := decodeBody[bookPage](t, e.get("/api/books?series="+url.QueryEscape("示例系列")))
	if series.Total != 2 {
		t.Fatalf("按系列筛 = %d，期望 2", series.Total)
	}
	byRating := decodeBody[bookPage](t, e.get("/api/books?rating=5"))
	if byRating.Total != 1 || byRating.Items[0].ID != first {
		t.Fatalf("按评分筛 = %+v", byRating)
	}
	collection := createCollectionViaAPI(t, e, "筛选书单")
	requireStatus(t, sendCollection(t, e, http.MethodPut,
		fmt.Sprintf("/api/collections/%d/books", collection.ID),
		fmt.Sprintf(`{"bookIds":[%d]}`, second)), http.StatusNoContent)
	inCollection := decodeBody[bookPage](t, e.get(fmt.Sprintf("/api/books?collection=%d", collection.ID)))
	if inCollection.Total != 1 || inCollection.Items[0].ID != second {
		t.Fatalf("按书单筛 = %+v", inCollection)
	}
}

func TestListBooksSortByRating(t *testing.T) {
	e := newEnv(t)
	low := e.seedBook("低分书", "作者甲")
	high := e.seedBook("高分书", "作者乙")
	e.seedBook("未评分书", "作者丙")
	requireStatus(t, e.patch(bookPath(low), `{"rating":2}`), http.StatusOK)
	requireStatus(t, e.patch(bookPath(high), `{"rating":5}`), http.StatusOK)

	page := decodeBody[bookPage](t, e.get("/api/books?sort=rating"))
	if page.Total != 3 || page.Items[0].ID != high {
		t.Fatalf("sort=rating 首本 = %+v", page.Items[0])
	}
}

func TestListBooksP3FilterValidation(t *testing.T) {
	e := newEnv(t)
	requireError(t, e.get("/api/books?rating=0"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, e.get("/api/books?rating=6"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, e.get("/api/books?collection=abc"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, e.get("/api/books?sort=bogus"), http.StatusBadRequest, codeInvalidParam)
}
