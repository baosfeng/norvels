package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// 评分与批量 PATCH 的口径：0 是未评、越界即 400；批量逐本隔离失败，
// 非法 patch 在开事务前就被拒（不变成「每本书各失败一次」）。

func TestSetBookRating(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedRatingBooks(t, ctx, st, 1)[0]

	for _, rating := range []int{5, 3, 0} {
		if err := st.SetBookRating(ctx, bookID, rating); err != nil {
			t.Fatalf("SetBookRating(%d): %v", rating, err)
		}
		if got := mustBookDetail(t, ctx, st, bookID).Rating; got != rating {
			t.Errorf("评分 = %d，期望 %d", got, rating)
		}
	}
	for _, bad := range []int{-1, 6, 100} {
		if err := st.SetBookRating(ctx, bookID, bad); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("rating=%d err = %v，期望 ErrInvalidArgument", bad, err)
		}
	}
	if err := st.SetBookRating(ctx, 0, 3); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("id=0 err = %v，期望 ErrInvalidArgument", err)
	}
	if err := st.SetBookRating(ctx, 999999, 3); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的书 err = %v，期望 ErrNotFound", err)
	}
}

func TestPatchBooksBatchAppliesAllFields(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := seedRatingBooks(t, ctx, st, 3)

	patch := BookMetaPatch{
		Title:       ptr("统一书名"),
		Series:      ptr("示例系列"),
		SeriesIndex: ptr(2),
		Tags:        ptr([]string{"玄幻", " 玄幻 ", "悬疑"}),
		Authors:     ptr([]string{"示例作者甲", "示例作者乙"}),
		Rating:      ptr(4),
		Publisher:   ptr("示例出版社"),
		PubDate:     ptr("2024-05-01"),
		ISBN:        ptr("978-7-000-00000-0"),
		Language:    ptr("zh-CN"),
	}
	updated, failed, err := st.PatchBooksBatch(ctx, ids, patch)
	if err != nil {
		t.Fatalf("PatchBooksBatch: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("不该有失败项: %v", failed)
	}
	if fmt.Sprint(updated) != fmt.Sprint(ids) {
		t.Fatalf("updated = %v，期望保持入参顺序 %v", updated, ids)
	}
	for _, id := range ids {
		d := mustBookDetail(t, ctx, st, id)
		if d.Title != "统一书名" || d.Series != "示例系列" || d.SeriesIndex != 2 || d.Rating != 4 {
			t.Errorf("书 %d 元数据未落全: %+v", id, d.BookSummary)
		}
		if d.Publisher != "示例出版社" || d.PubDate != "2024-05-01" ||
			d.ISBN != "978-7-000-00000-0" || d.Language != "zh-CN" {
			t.Errorf("书 %d 出版信息未落全: %+v", id, d.BookSummary)
		}
		// ListBookTags 按名称升序（字节序），不是入参顺序。
		if fmt.Sprint(d.Tags) != "[悬疑 玄幻]" {
			t.Errorf("书 %d 标签 = %v，期望去重后按名称升序", id, d.Tags)
		}
		if fmt.Sprint(d.Authors) != "[示例作者甲 示例作者乙]" {
			t.Errorf("书 %d 作者 = %v", id, d.Authors)
		}
	}
}

func TestPatchBooksBatchIsolatesFailures(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := seedRatingBooks(t, ctx, st, 2)
	batch := []int64{ids[0], 999999, ids[1]}

	updated, failed, err := st.PatchBooksBatch(ctx, batch, BookMetaPatch{Rating: ptr(2)})
	if err != nil {
		t.Fatalf("PatchBooksBatch: %v", err)
	}
	if fmt.Sprint(updated) != fmt.Sprint(ids) {
		t.Errorf("updated = %v，期望 %v", updated, ids)
	}
	if len(failed) != 1 || !errors.Is(failed[999999], ErrNotFound) {
		t.Fatalf("failed = %v，期望只含 999999 的 ErrNotFound", failed)
	}
	for _, id := range ids {
		if got := mustBookDetail(t, ctx, st, id).Rating; got != 2 {
			t.Errorf("书 %d 评分 = %d，期望 2（别的书失败不该连坐）", id, got)
		}
	}
}

func TestPatchBooksBatchRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := seedRatingBooks(t, ctx, st, 1)
	many := make([]int64, maxBatchBooks+1)
	for i := range many {
		many[i] = int64(i + 1)
	}

	cases := []struct {
		name  string
		ids   []int64
		patch BookMetaPatch
	}{
		{"空 ids", nil, BookMetaPatch{Rating: ptr(3)}},
		{"超过 200 本", many, BookMetaPatch{Rating: ptr(3)}},
		{"空 patch", ids, BookMetaPatch{}},
		{"评分越界", ids, BookMetaPatch{Rating: ptr(9)}},
		{"空标题", ids, BookMetaPatch{Title: ptr("   ")}},
		{"日期格式", ids, BookMetaPatch{PubDate: ptr("2024/05/01")}},
	}
	for _, c := range cases {
		_, _, err := st.PatchBooksBatch(ctx, c.ids, c.patch)
		if !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s err = %v，期望 ErrInvalidArgument", c.name, err)
		}
	}
}

func TestPatchBooksBatchDedupesIDs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedRatingBooks(t, ctx, st, 1)[0]

	updated, failed, err := st.PatchBooksBatch(ctx, []int64{id, id}, BookMetaPatch{Rating: ptr(1)})
	if err != nil || len(failed) != 0 {
		t.Fatalf("PatchBooksBatch = %v, %v", failed, err)
	}
	if len(updated) != 1 || updated[0] != id {
		t.Errorf("updated = %v，期望去重后只有 %d", updated, id)
	}
}

// TestInSavepointRollsBackOnError 锁死「某本失败不连坐」的底层机制：
// fn 在保存点内写的行必须随失败一起消失，外层事务其余内容不动。
func TestInSavepointRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedRatingBooks(t, ctx, st, 1)[0]
	boom := errors.New("boom")

	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck // 测试内回滚是收尾
	err = inSavepoint(ctx, tx, "rollback_probe", func() error {
		if _, err := tx.ExecContext(ctx,
			"UPDATE books SET title = '不该留下的标题' WHERE id = ?", bookID); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("inSavepoint err = %v，期望透传 boom", err)
	}
	var title string
	if err := tx.QueryRowContext(ctx,
		"SELECT title FROM books WHERE id = ?", bookID).Scan(&title); err != nil {
		t.Fatalf("读回标题: %v", err)
	}
	if title == "不该留下的标题" {
		t.Errorf("保存点回滚失效：标题仍是 %q", title)
	}
}

// seedRatingBooks 造 n 本互不冲突的示例书（同一作者），返回按创建顺序的书 id。
func seedRatingBooks(t *testing.T, ctx context.Context, st *Store, n int) []int64 {
	t.Helper()
	ids := make([]int64, 0, n)
	for i := 1; i <= n; i++ {
		book := sampleBook(fmt.Sprintf("示例书%d", i), "示例作者",
			fmt.Sprintf("/path/to/txt/p3-rating-%d.txt", i), fmt.Sprintf("%064d", i))
		res, err := st.SaveBook(ctx, book, SaveOptions{})
		if err != nil {
			t.Fatalf("SaveBook %d: %v", i, err)
		}
		ids = append(ids, res.BookID)
	}
	return ids
}

// mustBookDetail 取详情，失败即 Fatal。
func mustBookDetail(t *testing.T, ctx context.Context, st *Store, id int64) *BookDetail {
	t.Helper()
	d, err := st.GetBook(ctx, id)
	if err != nil {
		t.Fatalf("GetBook(%d): %v", id, err)
	}
	return d
}
