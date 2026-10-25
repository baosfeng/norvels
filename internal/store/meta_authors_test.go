package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestUpdateBookMetaAuthorsFullReplace PATCH authors 是全量替换，且主作者同步成 [0]。
func TestUpdateBookMetaAuthorsFullReplace(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedMetaBook(t, st, ctx, "补丁示例书", "原示例作者", "/path/to/patch-authors.txt", strings.Repeat("t", 64))

	if err := st.UpdateBookMeta(ctx, id, BookMetaPatch{Authors: ptr([]string{"作者甲", "作者乙"})}); err != nil {
		t.Fatalf("UpdateBookMeta: %v", err)
	}
	detail, err := st.GetBook(ctx, id)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if !reflect.DeepEqual(detail.Authors, []string{"作者甲", "作者乙"}) {
		t.Fatalf("authors = %v，期望 [作者甲 作者乙]", detail.Authors)
	}
	if detail.AuthorName != "作者甲" {
		t.Fatalf("authorName = %q，期望 authors[0]", detail.AuthorName)
	}

	// 再换成单作者：旧关联行必须清掉，不留孤儿。
	if err := st.UpdateBookMeta(ctx, id, BookMetaPatch{Authors: ptr([]string{" 作者丙 ", ""})}); err != nil {
		t.Fatalf("再次 UpdateBookMeta: %v", err)
	}
	names, _ := bookAuthorRows(t, st, ctx, id)
	if !reflect.DeepEqual(names, []string{"作者丙"}) {
		t.Fatalf("替换后 books_authors = %v，期望 [作者丙]", names)
	}
	if got := primaryAuthorName(t, st, ctx, id); got != "作者丙" {
		t.Fatalf("替换后主作者 = %q", got)
	}
}

// TestUpdateBookMetaAuthorAndAuthorsPrefersAuthors 同时传 author 与 authors 时 authors 优先。
func TestUpdateBookMetaAuthorAndAuthorsPrefersAuthors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedMetaBook(t, st, ctx, "冲突示例书", "原示例作者", "/path/to/patch-conflict.txt", strings.Repeat("u", 64))

	err := st.UpdateBookMeta(ctx, id, BookMetaPatch{
		Author:  ptr("作者丁"),
		Authors: ptr([]string{"作者甲", "作者乙"}),
	})
	if err != nil {
		t.Fatalf("UpdateBookMeta: %v", err)
	}
	detail, err := st.GetBook(ctx, id)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if detail.AuthorName != "作者甲" || !reflect.DeepEqual(detail.Authors, []string{"作者甲", "作者乙"}) {
		t.Fatalf("authors/authorName = %v/%q，期望 [作者甲 作者乙]/作者甲", detail.Authors, detail.AuthorName)
	}
}

// TestUpdateBookMetaSingleAuthorEqualsList 只传 author 时等价于 authors=[author]。
func TestUpdateBookMetaSingleAuthorEqualsList(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedMetaBook(t, st, ctx, "单值示例书", "原示例作者", "/path/to/patch-single.txt", strings.Repeat("v", 64))

	if err := st.UpdateBookMeta(ctx, id, BookMetaPatch{Authors: ptr([]string{"作者甲", "作者乙"})}); err != nil {
		t.Fatalf("先写多作者: %v", err)
	}
	if err := st.UpdateBookMeta(ctx, id, BookMetaPatch{Author: ptr("作者戊")}); err != nil {
		t.Fatalf("单值改作者: %v", err)
	}
	names, _ := bookAuthorRows(t, st, ctx, id)
	if !reflect.DeepEqual(names, []string{"作者戊"}) {
		t.Fatalf("books_authors = %v，期望 [作者戊]（单值等价于列表一项）", names)
	}
}

// TestUpdateBookMetaEmptyAuthorsRejected 空作者列表在库层也拒绝（api 层先拦 400）。
func TestUpdateBookMetaEmptyAuthorsRejected(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedMetaBook(t, st, ctx, "空作者示例书", "原示例作者", "/path/to/patch-empty.txt", strings.Repeat("w", 64))

	err := st.UpdateBookMeta(ctx, id, BookMetaPatch{Authors: ptr([]string{"", "   "})})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空 authors 的错误 = %v，期望 ErrInvalidArgument", err)
	}
}
