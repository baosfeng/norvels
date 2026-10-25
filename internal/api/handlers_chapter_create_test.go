package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"norvels/internal/store"
)

func TestCreateChapterAppend(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("建章之书", "建章作者")
	before := e.bookChapters(bookID)
	target := fmt.Sprintf("/api/books/%d/chapters", bookID)

	rr := sendBooks(t, e, http.MethodPost, target, `{"title":"第3章 转","content":"新章正文。","afterChapterId":0}`)
	requireStatus(t, rr, http.StatusCreated)
	got := decodeBody[store.ChapterBrief](t, rr)
	if got.ID <= 0 || got.Idx != before.Total || got.Title != "第3章 转" || got.WordCount <= 0 {
		t.Fatalf("新章 = %+v，期望 idx=%d", got, before.Total)
	}
	after := e.bookChapters(bookID)
	if len(after.Items) != before.Total+1 || after.Items[len(after.Items)-1].ID != got.ID {
		t.Fatalf("建章后目录 = %+v", after.Items)
	}
}

func TestCreateChapterInsertAfter(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("插章之书", "插章作者")
	chapters := e.bookChapters(bookID)
	anchor := chapters.Items[1].ID
	target := fmt.Sprintf("/api/books/%d/chapters", bookID)

	rr := sendBooks(t, e, http.MethodPost, target,
		fmt.Sprintf(`{"title":"插入章","content":"插入正文。","afterChapterId":%d}`, anchor))
	requireStatus(t, rr, http.StatusCreated)
	got := decodeBody[store.ChapterBrief](t, rr)
	if got.Idx != chapters.Items[1].Idx+1 {
		t.Fatalf("插入章 idx = %d，期望 %d", got.Idx, chapters.Items[1].Idx+1)
	}
	after := e.bookChapters(bookID)
	if len(after.Items) != len(chapters.Items)+1 || after.Items[got.Idx].Title != "插入章" {
		t.Fatalf("插入后目录 = %+v", after.Items)
	}
}

func TestCreateChapterValidation(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("建章校验之书", "建章作者")
	otherID := e.seedBook("别的书", "别的作者")
	foreign := e.bookChapters(otherID).Items[0].ID
	target := fmt.Sprintf("/api/books/%d/chapters", bookID)
	tooLong := strings.Repeat("a", maxCreateChapterContentBytes+1)

	requireError(t, sendBooks(t, e, http.MethodPost, target, `{"title":"  ","content":"正文"}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendBooks(t, e, http.MethodPost, target, `{"title":"标题","content":"   "}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendBooks(t, e, http.MethodPost, target,
		fmt.Sprintf(`{"title":"标题","content":%q}`, tooLong)), http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendBooks(t, e, http.MethodPost, target,
		`{"title":"标题","content":"正文","afterChapterId":-1}`), http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendBooks(t, e, http.MethodPost, target,
		fmt.Sprintf(`{"title":"标题","content":"正文","afterChapterId":%d}`, foreign)),
		http.StatusBadRequest, codeInvalidParam)
	longTitle := strings.Repeat("题", 250) // store 侧标题上限 200 字 → 参数错误 400
	requireError(t, sendBooks(t, e, http.MethodPost, target,
		fmt.Sprintf(`{"title":%q,"content":"正文"}`, longTitle)), http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendBooks(t, e, http.MethodPost, "/api/books/999999/chapters",
		`{"title":"标题","content":"正文"}`), http.StatusNotFound, codeNotFound)
}
