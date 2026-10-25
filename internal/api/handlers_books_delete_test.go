package api

import (
	"context"
	"net/http"
	"testing"

	"norvels/internal/store"
)

// TestDeleteBookEndpoint 覆盖 spec §4 的删书验收：204 → 列表 -1 → 再删 404。
func TestDeleteBookEndpoint(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("待删的书", "作者甲")

	rr := e.do(http.MethodDelete, bookPath(bookID), nil, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE 状态码 = %d，期望 204，body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Errorf("204 不该有响应体，实际 %q", rr.Body.String())
	}

	books := decodeBody[store.Page[store.BookSummary]](t, e.get("/api/books?page=1&size=10"))
	if books.Total != 0 || len(books.Items) != 0 {
		t.Errorf("删后列表 = %+v，期望空", books)
	}

	requireError(t, e.get(bookPath(bookID)), http.StatusNotFound, codeNotFound)
	requireError(t, e.do(http.MethodDelete, bookPath(bookID), nil, nil),
		http.StatusNotFound, codeNotFound)
}

// TestDeleteBookClearsReadingProgress 删书后「最近在读」不再返回这本书。
func TestDeleteBookClearsReadingProgress(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("进度书", "作者乙")
	chapterID := e.firstChapterID(bookID)

	rr := e.putJSON("/api/reading/progress", map[string]any{
		"bookId": bookID, "chapterId": chapterID, "percent": 0.4,
	})
	if rr.Code != http.StatusNoContent && rr.Code != http.StatusOK {
		t.Fatalf("写进度状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if before := e.progressItems(); len(before.Items) != 1 {
		t.Fatalf("删前进度条数 = %d，期望 1", len(before.Items))
	}

	if rr := e.do(http.MethodDelete, bookPath(bookID), nil, nil); rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if after := e.progressItems(); len(after.Items) != 0 {
		t.Errorf("删后仍返回进度: %+v", after.Items)
	}
}

// TestPatchSerialStatus 连载状态：合法值回写详情，非法值 400 invalid_param。
func TestPatchSerialStatus(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("连载书", "作者丙")
	target := bookPath(bookID)

	if rr := e.patch(target, `{"serialStatus":"ongoing"}`); rr.Code != http.StatusOK {
		t.Fatalf("PATCH 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if got := decodeBody[bookDetailResp](t, e.get(target)).SerialStatus; got != "ongoing" {
		t.Errorf("serialStatus = %q，期望 ongoing", got)
	}
	// 改连载状态不该动解析状态。
	if got := decodeBody[bookDetailResp](t, e.get(target)).Status; got != "parsed" {
		t.Errorf("解析状态被改了: %q", got)
	}
	for _, want := range []string{"completed", ""} {
		if rr := e.patch(target, `{"serialStatus":"`+want+`"}`); rr.Code != http.StatusOK {
			t.Fatalf("PATCH %q 状态码 = %d", want, rr.Code)
		}
		if got := decodeBody[bookDetailResp](t, e.get(target)).SerialStatus; got != want {
			t.Errorf("serialStatus = %q，期望 %q", got, want)
		}
	}

	requireError(t, e.patch(target, `{"serialStatus":"abc"}`), http.StatusBadRequest, codeInvalidParam)
}

// TestBookDetailLastChapter 详情返回 idx 最大的章节标题与序号。
func TestBookDetailLastChapter(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("最新章节书", "作者丁")

	detail := decodeBody[bookDetailResp](t, e.get(bookPath(bookID)))
	if detail.LastChapterIdx != 2 {
		t.Errorf("lastChapterIdx = %d，期望 2", detail.LastChapterIdx)
	}
	if detail.LastChapterTitle != "第2章 承" {
		t.Errorf("lastChapterTitle = %q，期望「第2章 承」", detail.LastChapterTitle)
	}
}

// firstChapterID 取一本书的第一章 id（按 store 的 idx 升序）。
func (e *testEnv) firstChapterID(bookID int64) int64 {
	e.t.Helper()
	var id int64
	if err := e.st.DB().QueryRowContext(context.Background(),
		"SELECT id FROM chapters WHERE book_id = ? ORDER BY idx LIMIT 1", bookID).Scan(&id); err != nil {
		e.t.Fatalf("读首章 id: %v", err)
	}
	return id
}
