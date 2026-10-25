package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSetBookTagsNormalizesAndReplaces(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	res, err := st.SaveBook(ctx, sampleBook("标签之书", "示例作者", "/path/to/tags.txt", strings.Repeat("f", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	if err := st.SetBookTags(ctx, res.BookID, []string{" 武侠 ", "", "武侠", "   "}); err != nil {
		t.Fatalf("SetBookTags: %v", err)
	}
	if got := listBookTags(t, ctx, st, res.BookID); !reflect.DeepEqual(got, []string{"武侠"}) {
		t.Fatalf("首次写入 = %v, want [武侠]", got)
	}

	// 全量替换：第二次写入只保留新标签，旧标签连同孤儿标签一起消失。
	if err := st.SetBookTags(ctx, res.BookID, []string{"科幻", "武侠"}); err != nil {
		t.Fatalf("SetBookTags 替换: %v", err)
	}
	if got := listBookTags(t, ctx, st, res.BookID); !reflect.DeepEqual(got, []string{"武侠", "科幻"}) {
		t.Fatalf("替换后 = %v, want [武侠 科幻]", got)
	}

	// 详情页要能读回标签（PATCH 后立即可见的口径）。
	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if !reflect.DeepEqual(detail.Tags, []string{"武侠", "科幻"}) {
		t.Fatalf("detail.Tags = %v", detail.Tags)
	}
}

func TestSetBookTagsClearsAndPrunes(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	res, err := st.SaveBook(ctx, sampleBook("清空标签之书", "示例作者", "/path/to/clear.txt", strings.Repeat("g", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	if err := st.SetBookTags(ctx, res.BookID, []string{"示例分类"}); err != nil {
		t.Fatalf("SetBookTags: %v", err)
	}
	if err := st.SetBookTags(ctx, res.BookID, nil); err != nil {
		t.Fatalf("SetBookTags(nil): %v", err)
	}
	if got := listBookTags(t, ctx, st, res.BookID); len(got) != 0 {
		t.Fatalf("清空后 = %v, want 空", got)
	}
	tags, err := st.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("孤儿标签未清理: %+v", tags)
	}
}

func TestListBookTagsErrors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.ListBookTags(ctx, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("ListBookTags(0) err = %v", err)
	}
	if err := st.SetBookTags(ctx, 0, []string{"x"}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("SetBookTags(0) err = %v", err)
	}
	if err := st.SetBookTags(ctx, 12345, []string{"x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetBookTags(缺失书) err = %v", err)
	}
}

// listBookTags 是测试内的读取助手，失败即 t.Fatal。
func listBookTags(t *testing.T, ctx context.Context, st *Store, bookID int64) []string {
	t.Helper()
	tags, err := st.ListBookTags(ctx, bookID)
	if err != nil {
		t.Fatalf("ListBookTags: %v", err)
	}
	return tags
}
