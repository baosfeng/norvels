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

// TestSchemaHasNoSegOrFTS 锁死「正文检索已下线」的结构口径。
//
// 0006 之后 chapters 上不应再有分词镜像列，库里也不应再有 chapters_fts：
// 这两坨数据是为正文检索服务的，一旦被后续改动带回，库会重新膨胀数 GB。
func TestSchemaHasNoSegOrFTS(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	for _, col := range []string{"title_seg", "content_seg"} {
		if columnExists(t, ctx, st, "chapters", col) {
			t.Errorf("chapters 仍有分词镜像列 %q", col)
		}
	}
	for _, name := range []string{"chapters_fts", "chapters_fts_data", "chapters_fts_idx",
		"chapters_fts_docsize", "chapters_fts_config"} {
		exists, err := st.tableExists(ctx, name)
		if err != nil {
			t.Fatalf("tableExists(%q): %v", name, err)
		}
		if exists {
			t.Errorf("库内仍存在 %q（正文检索已下线）", name)
		}
	}
	for _, trg := range []string{"chapters_ai", "chapters_ad", "chapters_au"} {
		var n int
		if err := st.DB().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name = ?", trg).Scan(&n); err != nil {
			t.Fatalf("查触发器 %q: %v", trg, err)
		}
		if n != 0 {
			t.Errorf("仍存在触发器 %q", trg)
		}
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
