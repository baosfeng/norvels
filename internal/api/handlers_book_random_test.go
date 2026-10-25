package api

import (
	"fmt"
	"net/http"
	"testing"

	"norvels/internal/store"
)

// bookAuthorID 读一本书的 authorId（随机一本按作者筛选时用）。
func bookAuthorID(t *testing.T, e *testEnv, bookID int64) int64 {
	t.Helper()
	detail := decodeBody[store.BookDetail](t, e.get(bookPath(bookID)))
	return detail.AuthorID
}

func TestRandomBook(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("随机示例书", "随机作者")
	rr := e.get("/api/books/random")
	requireStatus(t, rr, http.StatusOK)
	brief := decodeBody[store.BookBrief](t, rr)
	if brief.ID != bookID || brief.Title != "随机示例书" || brief.AuthorName != "随机作者" {
		t.Fatalf("随机一本 = %+v", brief)
	}
}

func TestRandomBookFilters(t *testing.T) {
	e := newEnv(t)
	tagged := e.seedBook("带标签的书", "作者甲")
	e.seedBook("另一本", "作者乙")
	bindTags(t, e, tagged, []string{"随机标签"})

	rr := e.get("/api/books/random?tag=" + "随机标签")
	requireStatus(t, rr, http.StatusOK)
	if got := decodeBody[store.BookBrief](t, rr); got.ID != tagged {
		t.Fatalf("按标签随机 = %+v，期望 %d", got, tagged)
	}
	authorID := bookAuthorID(t, e, tagged)
	if rr := e.get(fmt.Sprintf("/api/books/random?authorId=%d", authorID)); rr.Code != http.StatusOK {
		t.Fatalf("按作者随机 = %d，body=%s", rr.Code, rr.Body.String())
	}
}

func TestRandomBookCollectionFilter(t *testing.T) {
	e := newEnv(t)
	inList := e.seedBook("书单里的书", "作者甲")
	e.seedBook("书单外的书", "作者乙")
	collection := createCollectionViaAPI(t, e, "随机书单")
	requireStatus(t, sendCollection(t, e, http.MethodPut,
		fmt.Sprintf("/api/collections/%d/books", collection.ID),
		fmt.Sprintf(`{"bookIds":[%d]}`, inList)), http.StatusNoContent)

	rr := e.get(fmt.Sprintf("/api/books/random?collection=%d", collection.ID))
	requireStatus(t, rr, http.StatusOK)
	if got := decodeBody[store.BookBrief](t, rr); got.ID != inList {
		t.Fatalf("按书单随机 = %+v，期望 %d", got, inList)
	}
}

func TestRandomBookEmpty(t *testing.T) {
	e := newEnv(t)
	requireError(t, e.get("/api/books/random"), http.StatusNotFound, codeNotFound)
	requireError(t, e.get("/api/books/random?tag=不存在的标签"), http.StatusNotFound, codeNotFound)
	requireError(t, e.get("/api/books/random?collection=999"), http.StatusNotFound, codeNotFound)
	requireError(t, e.get("/api/books/random?collection=abc"), http.StatusBadRequest, codeInvalidParam)
}
