package api

import (
	"context"
	"net/http"
	"testing"

	"norvels/internal/store"
)

// 分类（0015）的接口口径：/api/categories 返回两级树；
// /api/books 按 categoryL1/categoryL2 筛选；PATCH 支持改分类。

// setCategory 直接走 store 写分类（导入链路之外的第二次写入路径）。
func setCategory(t *testing.T, e *testEnv, id int64, l1, l2 string) {
	t.Helper()
	if err := e.st.UpdateBookMeta(context.Background(), id,
		store.BookMetaPatch{CategoryL1: &l1, CategoryL2: &l2}); err != nil {
		t.Fatalf("设置分类: %v", err)
	}
}

func TestListCategoriesTree(t *testing.T) {
	e := newEnv(t)
	a := e.seedBook("玄幻甲", "作者甲")
	b := e.seedBook("玄幻乙", "作者乙")
	c := e.seedBook("历史丙", "作者丙")
	setCategory(t, e, a, "玄幻", "东方玄幻")
	setCategory(t, e, b, "玄幻", "奇幻玄幻")
	setCategory(t, e, c, "历史", "两宋元明")

	rr := e.get("/api/categories")
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[categoryListResponse](t, rr)
	if body.Total != 3 || len(body.Items) != 2 {
		t.Fatalf("分类树 = %+v", body)
	}
	if body.Items[0].L1 != "历史" || body.Items[0].Count != 1 || len(body.Items[0].Items) != 1 {
		t.Fatalf("历史节点 = %+v", body.Items[0])
	}
	if body.Items[1].L1 != "玄幻" || body.Items[1].Count != 2 || len(body.Items[1].Items) != 2 {
		t.Fatalf("玄幻节点 = %+v", body.Items[1])
	}
}

func TestListBooksFilterByCategory(t *testing.T) {
	e := newEnv(t)
	a := e.seedBook("玄幻甲", "作者甲")
	b := e.seedBook("玄幻乙", "作者乙")
	setCategory(t, e, a, "玄幻", "东方玄幻")
	setCategory(t, e, b, "玄幻", "奇幻玄幻")

	rr := e.get("/api/books?categoryL1=" + "玄幻")
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[booksPageResp](t, rr)
	if body.Total != 2 {
		t.Fatalf("大分类筛选 = %+v", body)
	}

	rr = e.get("/api/books?categoryL1=" + "玄幻" + "&categoryL2=" + "奇幻玄幻")
	requireStatus(t, rr, http.StatusOK)
	body = decodeBody[booksPageResp](t, rr)
	if body.Total != 1 || body.Items[0].Title != "玄幻乙" {
		t.Fatalf("小分类筛选 = %+v", body)
	}
}

func TestPatchBookCategory(t *testing.T) {
	e := newEnv(t)
	id := e.seedBook("改分类", "作者甲")
	rr := e.patch(bookPath(id), `{"categoryL1":"仙侠","categoryL2":"修真文明"}`)
	requireStatus(t, rr, http.StatusOK)

	got, err := e.st.GetBook(context.Background(), id)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if got.CategoryL1 != "仙侠" || got.CategoryL2 != "修真文明" {
		t.Fatalf("分类 = (%q, %q)", got.CategoryL1, got.CategoryL2)
	}
}
