package api

import (
	"net/http"
	"testing"

	"norvels/internal/store"
)

// booksPageResp 是 GET /api/books 的分页响应（只取总数与条目 id）。
type booksPageResp struct {
	Items []store.BookSummary `json:"items"`
	Total int                 `json:"total"`
}

// GET /api/books?serialStatus=… 的筛选口径（管理页筛选条）：none 筛「未知」。
func TestListBooksSerialStatusFilter(t *testing.T) {
	env := newEnv(t)
	serial := env.seedBook("连载中的示例书", "示例作者")
	done := env.seedBook("已完结的示例书", "示例作者")
	env.seedBook("状态未知的示例书", "示例作者")

	if rr := env.patch(bookPath(serial), `{"serialStatus":"ongoing"}`); rr.Code != http.StatusOK {
		t.Fatalf("设置连载中失败: %d %s", rr.Code, rr.Body.String())
	}
	if rr := env.patch(bookPath(done), `{"serialStatus":"completed"}`); rr.Code != http.StatusOK {
		t.Fatalf("设置已完结失败: %d %s", rr.Code, rr.Body.String())
	}

	cases := []struct {
		query string
		total int
	}{
		{"", 3},
		{"?serialStatus=ongoing", 1},
		{"?serialStatus=completed", 1},
		{"?serialStatus=none", 1},
	}
	for _, tc := range cases {
		rr := env.get("/api/books" + tc.query)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s 状态码 = %d", tc.query, rr.Code)
		}
		if page := decodeBody[booksPageResp](t, rr); page.Total != tc.total {
			t.Errorf("%s 总数 = %d，期望 %d", tc.query, page.Total, tc.total)
		}
	}
	if rr := env.get("/api/books?serialStatus=abc"); rr.Code != http.StatusBadRequest {
		t.Errorf("非法取值状态码 = %d，期望 400", rr.Code)
	}
}
