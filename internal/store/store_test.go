package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(context.Background(), filepath.Join(t.TempDir(), "norvels.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return st
}

func sampleBook(title, author, path, sha string) *model.ParsedBook {
	return &model.ParsedBook{
		Title:      title,
		Author:     author,
		SourcePath: path,
		SourceName: filepath.Base(path),
		Encoding:   "utf-8",
		FileSize:   1024,
		SHA256:     sha,
		WordCount:  30,
		Intro:      "简介：" + title,
		Volumes: []model.ParsedVolume{
			{Idx: 0, Title: "第一卷"},
			{Idx: 1, Title: "第二卷"},
		},
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "第一章 风起", Content: "华夏国临川市，午后的秋日，微风拂过城楼。", WordCount: 10},
			{Idx: 1, VolumeIdx: 0, Title: "第二章 云涌", Content: "他站在城楼上，望着远方的梅宁城与风雪。", WordCount: 10},
			{Idx: 2, VolumeIdx: -1, Title: "番外 小记", Content: "番外正文，没有归入任何卷。", WordCount: 10},
		},
	}
}

func TestSegmentRoundTrip(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"华夏国，午后", "华 夏 国 ，午 后 "},
		{"abc 123", "abc 123"},
		{"临川市", "临 川 市 "},
		{"", ""},
		{"a梅b", "a梅 b"},
		{"，。！", "，。！"},
	}
	for _, c := range cases {
		got := Segment(c.in)
		if got != c.want {
			t.Errorf("Segment(%q) = %q, want %q", c.in, got, c.want)
		}
		if back := Unsegment(got); back != c.in {
			t.Errorf("Unsegment(Segment(%q)) = %q", c.in, back)
		}
	}
}

func TestSaveBookUnknownAuthor(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	b := sampleBook("无名之书", "", "/books/nobody.txt", strings.Repeat("9", 64))
	if _, err := st.SaveBook(ctx, b, SaveOptions{}); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	authors, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 10})
	if err != nil || authors.Total != 1 {
		t.Fatalf("ListAuthors = %+v, err=%v", authors, err)
	}
	if authors.Items[0].Name != unknownAuthor || authors.Items[0].BookCount != 1 {
		t.Errorf("空作者应落「%s」: %+v", unknownAuthor, authors.Items[0])
	}
	books, err := st.ListBooks(ctx, BookQuery{AuthorID: authors.Items[0].ID, Page: 1, Size: 10})
	if err != nil || books.Total != 1 {
		t.Errorf("按作者筛选 = %+v, err=%v", books, err)
	}
}

func TestReindexAndTriggerCleanup(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	b := sampleBook("索引测试", "作者", "/books/idx.txt", strings.Repeat("f", 64))
	saved, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := st.Reindex(ctx); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	r, err := st.Search(ctx, SearchQuery{Q: "临川市"})
	if err != nil || r.Chapters.Total != 1 {
		t.Fatalf("after reindex = %+v, err=%v", r.Chapters, err)
	}

	// 直接删书：级联删章节，触发器同步清理 FTS。
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM books WHERE id = ?", saved.BookID); err != nil {
		t.Fatalf("delete book: %v", err)
	}
	var ftsRows int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM chapters_fts").Scan(&ftsRows); err != nil {
		t.Fatalf("count fts: %v", err)
	}
	if ftsRows != 0 {
		t.Errorf("级联删除后 FTS 残留 %d 行", ftsRows)
	}
	r2, err := st.Search(ctx, SearchQuery{Q: "临川市"})
	if err != nil || r2.Chapters.Total != 0 {
		t.Errorf("删除后仍能搜到: %+v, err=%v", r2.Chapters, err)
	}
}

func TestMigrateTwiceAndInvalidArgs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if _, err := st.SaveBook(ctx, nil, SaveOptions{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("nil book err = %v", err)
	}
	if _, err := st.SaveBook(ctx, &model.ParsedBook{SourcePath: "x"}, SaveOptions{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("empty sha err = %v", err)
	}
	if _, err := st.ListChapters(ctx, ChapterQuery{BookID: 0}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("chapter query err = %v", err)
	}
	if _, err := st.GetBook(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing book err = %v", err)
	}
}
