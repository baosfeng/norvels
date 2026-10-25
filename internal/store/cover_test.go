package store

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// fakePNG 是「看起来像 PNG 的字节」：库层只按白名单校验 MIME 与大小，
// 不解析图片内容（真解析是 ebook 包的职责），所以测试不需要真图片。
var fakePNG = []byte("\x89PNG\r\n\x1a\n norvels-cover-test")

// seedCoverBook 存一本样例书并返回 bookID。
func seedCoverBook(t *testing.T, st *Store, ctx context.Context, title, path, sha string) int64 {
	t.Helper()
	res, err := st.SaveBook(ctx, sampleBook(title, "示例作者", path, sha), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	return res.BookID
}

func TestSaveAndGetCover(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedCoverBook(t, st, ctx, "示例封面之书", "/path/to/a.txt", strings.Repeat("c", 64))

	if err := st.SaveCover(ctx, bookID, "image/png", fakePNG); err != nil {
		t.Fatalf("SaveCover: %v", err)
	}
	got, err := st.GetCover(ctx, bookID)
	if err != nil {
		t.Fatalf("GetCover: %v", err)
	}
	if got.MIME != "image/png" || !bytes.Equal(got.Bytes, fakePNG) {
		t.Fatalf("cover = mime %q, %d bytes", got.MIME, len(got.Bytes))
	}
}

func TestSaveCoverOverwrite(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedCoverBook(t, st, ctx, "覆盖封面之书", "/path/to/b1.txt", strings.Repeat("d", 64))
	if err := st.SaveCover(ctx, bookID, "image/png", fakePNG); err != nil {
		t.Fatalf("SaveCover: %v", err)
	}
	next := append(append([]byte{}, fakePNG...), []byte("-v2")...)
	if err := st.SaveCover(ctx, bookID, "image/webp", next); err != nil {
		t.Fatalf("SaveCover overwrite: %v", err)
	}
	got, err := st.GetCover(ctx, bookID)
	if err != nil || got.MIME != "image/webp" || !bytes.Equal(got.Bytes, next) {
		t.Fatalf("覆盖后 cover = %+v, err=%v", got, err)
	}
}

func TestCoverFlagVisibleInListAndDetail(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedCoverBook(t, st, ctx, "列表封面之书", "/path/to/c1.txt", strings.Repeat("e", 64))
	if err := st.SaveCover(ctx, bookID, "image/gif", fakePNG); err != nil {
		t.Fatalf("SaveCover: %v", err)
	}

	page, err := st.ListBooks(ctx, BookQuery{Page: 1, Size: 10})
	if err != nil || len(page.Items) != 1 || !page.Items[0].HasCover {
		t.Fatalf("列表 hasCover = %+v, err=%v", page.Items, err)
	}
	detail, err := st.GetBook(ctx, bookID)
	if err != nil || !detail.HasCover {
		t.Fatalf("详情 hasCover = %v, err=%v", detail != nil && detail.HasCover, err)
	}
}

func TestGetCoverMissing(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedCoverBook(t, st, ctx, "无封面的书", "/path/to/b.txt", strings.Repeat("f", 64))
	if _, err := st.GetCover(ctx, bookID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetCover 未找到 err = %v, want ErrNotFound", err)
	}
	if _, err := st.GetCover(ctx, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("GetCover(0) err = %v, want ErrInvalidArgument", err)
	}
}

func TestSaveCoverValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedCoverBook(t, st, ctx, "校验之书", "/path/to/c.txt", strings.Repeat("g", 64))

	cases := []struct {
		name   string
		mime   string
		data   []byte
		bookID int64
	}{
		{"mime 不在白名单", "image/svg+xml", fakePNG, bookID},
		{"空字节", "image/png", nil, bookID},
		{"超过 2 MiB", "image/png", make([]byte, maxCoverBytes+1), bookID},
		{"书不存在", "image/png", fakePNG, bookID + 999},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := st.SaveCover(ctx, tc.bookID, tc.mime, tc.data)
			if tc.bookID != bookID {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("err = %v, want ErrNotFound", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err = %v, want ErrInvalidArgument", err)
			}
		})
	}
	if _, err := st.GetCover(ctx, bookID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("校验失败不应落库，err = %v", err)
	}
}
