package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"norvels/internal/model"
)

// ptr 是构造补丁字段的小助手。
func ptr[T any](v T) *T { return &v }

// seedMetaBook 存一本样例书并返回 bookID（书名词表全部虚构）。
func seedMetaBook(t *testing.T, st *Store, ctx context.Context, title, author, path, sha string) int64 {
	t.Helper()
	res, err := st.SaveBook(ctx, sampleBook(title, author, path, sha), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	return res.BookID
}

func TestUpdateBookMetaFields(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedMetaBook(t, st, ctx, "原标题", "原示例作者", "/path/to/meta.txt", strings.Repeat("h", 64))
	before, err := st.GetBook(ctx, id)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	time.Sleep(3 * time.Millisecond) // updated_at 是毫秒精度，确保可观测到变化

	patch := BookMetaPatch{
		Title:       ptr("改名后的示例书"),
		Intro:       ptr("  新简介  "),
		Series:      ptr("示例系列"),
		SeriesIndex: ptr(3),
		Tags:        ptr([]string{"示例分类"}),
	}
	if err := st.UpdateBookMeta(ctx, id, patch); err != nil {
		t.Fatalf("UpdateBookMeta: %v", err)
	}

	detail, err := st.GetBook(ctx, id)
	if err != nil {
		t.Fatalf("GetBook after patch: %v", err)
	}
	if detail.Title != "改名后的示例书" || detail.Intro != "新简介" {
		t.Fatalf("标题/简介 = %q / %q", detail.Title, detail.Intro)
	}
	if detail.Series != "示例系列" || detail.SeriesIndex != 3 {
		t.Fatalf("系列 = %q #%d", detail.Series, detail.SeriesIndex)
	}
	if !reflect.DeepEqual(detail.Tags, []string{"示例分类"}) {
		t.Fatalf("tags = %v", detail.Tags)
	}
	if !detail.UpdatedAt.After(before.UpdatedAt) {
		t.Errorf("updatedAt 未刷新: before=%v after=%v", before.UpdatedAt, detail.UpdatedAt)
	}
}

func TestUpdateBookMetaAuthorAggregate(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	movedID := seedMetaBook(t, st, ctx, "第一本书", "作者甲", "/path/to/x1.txt", strings.Repeat("i", 64))
	seedMetaBook(t, st, ctx, "第二本书", "作者乙", "/path/to/x2.txt", strings.Repeat("j", 64))

	if err := st.UpdateBookMeta(ctx, movedID, BookMetaPatch{Author: ptr("作者乙")}); err != nil {
		t.Fatalf("UpdateBookMeta: %v", err)
	}

	counts := authorBookCounts(t, ctx, st)
	// 新旧作者都必须重算：旧作者归零、新作者累加。
	if counts["作者乙"] != 2 {
		t.Errorf("作者乙 bookCount = %d, want 2（%v）", counts["作者乙"], counts)
	}
	if counts["作者甲"] != 0 {
		t.Errorf("作者甲 bookCount = %d, want 0（%v）", counts["作者甲"], counts)
	}
	detail, err := st.GetBook(ctx, movedID)
	if err != nil || detail.AuthorName != "作者乙" {
		t.Fatalf("作者变更后 detail = %+v, err=%v", detail, err)
	}
}

// authorBookCounts 返回「作者名 → 聚合作品数」。
func authorBookCounts(t *testing.T, ctx context.Context, st *Store) map[string]int {
	t.Helper()
	page, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 50})
	if err != nil {
		t.Fatalf("ListAuthors: %v", err)
	}
	out := make(map[string]int, len(page.Items))
	for _, a := range page.Items {
		out[a.Name] = a.BookCount
	}
	return out
}

func TestUpdateBookMetaEmptyPatchDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedMetaBook(t, st, ctx, "空补丁之书", "示例作者", "/path/to/empty.txt", strings.Repeat("k", 64))
	before, err := st.GetBook(ctx, id)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	time.Sleep(3 * time.Millisecond)
	if err := st.UpdateBookMeta(ctx, id, BookMetaPatch{}); err != nil {
		t.Fatalf("空 patch err = %v", err)
	}
	after, err := st.GetBook(ctx, id)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("空 patch 不应刷新 updatedAt: %v → %v", before.UpdatedAt, after.UpdatedAt)
	}
}

func TestUpdateBookMetaErrors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedMetaBook(t, st, ctx, "错误路径之书", "示例作者", "/path/to/err.txt", strings.Repeat("l", 64))

	if err := st.UpdateBookMeta(ctx, id, BookMetaPatch{Title: ptr("   ")}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空标题 err = %v, want ErrInvalidArgument", err)
	}
	if err := st.UpdateBookMeta(ctx, id+999, BookMetaPatch{Intro: ptr("x")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("缺失书 err = %v, want ErrNotFound", err)
	}
	if err := st.UpdateBookMeta(ctx, 0, BookMetaPatch{Intro: ptr("x")}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("id=0 err = %v, want ErrInvalidArgument", err)
	}
	// 负的系列序号被夹到 0（列语义是「0 表示未知」）。
	if err := st.UpdateBookMeta(ctx, id, BookMetaPatch{SeriesIndex: ptr(-5)}); err != nil {
		t.Fatalf("UpdateBookMeta: %v", err)
	}
	detail, err := st.GetBook(ctx, id)
	if err != nil || detail.SeriesIndex != 0 {
		t.Fatalf("seriesIndex = %d, err=%v", detail.SeriesIndex, err)
	}
}

func TestSaveBookWritesMetadataAndCover(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := sampleBook("元数据之书", "示例作者", "/path/to/meta.epub", strings.Repeat("m", 64))
	book.SourceName = "meta.epub"
	book.Language = "zh-CN"
	book.Publisher = "示例出版社"
	book.ISBN = "9780000000001"
	book.PubDate = "2026-01-02"
	book.Series = "示例系列"
	book.SeriesIndex = 2
	book.Subjects = []string{" 科幻 ", "科幻", ""}
	book.Cover = &model.CoverImage{MIME: "image/jpeg", Bytes: []byte("\xff\xd8\xff fake jpeg")}
	book.SourceFormat = "epub"

	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	checkMetaFields(t, detail, "epub", true)
	if !reflect.DeepEqual(detail.Tags, []string{"科幻"}) {
		t.Errorf("tags = %v, want [科幻]", detail.Tags)
	}
	cover, err := st.GetCover(ctx, res.BookID)
	if err != nil || cover.MIME != "image/jpeg" {
		t.Fatalf("cover = %+v, err=%v", cover, err)
	}
}

// checkMetaFields 校验 0008 的书籍身份字段在详情上的落库结果。
func checkMetaFields(t *testing.T, detail *BookDetail, format string, wantCover bool) {
	t.Helper()
	if detail.Language != "zh-CN" || detail.Publisher != "示例出版社" || detail.ISBN != "9780000000001" {
		t.Errorf("语言/出版社/书号 = %q/%q/%q", detail.Language, detail.Publisher, detail.ISBN)
	}
	if detail.PubDate != "2026-01-02" || detail.Series != "示例系列" || detail.SeriesIndex != 2 {
		t.Errorf("出版日期/系列 = %q/%q#%d", detail.PubDate, detail.Series, detail.SeriesIndex)
	}
	if detail.Format != format || detail.HasCover != wantCover {
		t.Errorf("format/hasCover = %q/%v, want %q/%v", detail.Format, detail.HasCover, format, wantCover)
	}
}

func TestSaveBookForceReplacesCoverKeepsTags(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := sampleBook("覆盖导入之书", "示例作者", "/path/to/force.epub", strings.Repeat("n", 64))
	book.Subjects = []string{"示例分类"}
	book.Cover = &model.CoverImage{MIME: "image/png", Bytes: fakePNG}
	book.SourceFormat = "epub"
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	// 覆盖导入一本没有封面、没有分类的同源文件（典型场景：txt 重导）。
	again := sampleBook("覆盖导入之书", "示例作者", "/path/to/force.epub", strings.Repeat("n", 64))
	if _, err := st.SaveBook(ctx, again, SaveOptions{Force: true}); err != nil {
		t.Fatalf("SaveBook force: %v", err)
	}
	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if detail.HasCover || detail.Format != "txt" {
		t.Errorf("覆盖后 hasCover/format = %v/%q, want false/txt", detail.HasCover, detail.Format)
	}
	if _, err := st.GetCover(ctx, res.BookID); !errors.Is(err, ErrNotFound) {
		t.Errorf("旧封面未清理: err=%v", err)
	}
	// 没有 subjects 的覆盖导入保留既有标签（用户手工标签不该被 txt 重导抹掉）。
	if !reflect.DeepEqual(detail.Tags, []string{"示例分类"}) {
		t.Errorf("tags = %v, want [示例分类]", detail.Tags)
	}
}
