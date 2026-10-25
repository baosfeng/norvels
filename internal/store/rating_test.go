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
		requireOK(t, fmt.Sprintf("SetBookRating(%d)", rating), st.SetBookRating(ctx, bookID, rating))
		requireEqual(t, fmt.Sprintf("评分 %d", rating), mustBookDetail(t, ctx, st, bookID).Rating, rating)
	}
	for _, bad := range []int{-1, 6, 100} {
		requireErr(t, fmt.Sprintf("rating=%d", bad), st.SetBookRating(ctx, bookID, bad), ErrInvalidArgument)
	}
	requireErr(t, "id=0", st.SetBookRating(ctx, 0, 3), ErrInvalidArgument)
	requireErr(t, "不存在的书", st.SetBookRating(ctx, 999999, 3), ErrNotFound)
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
	requireOK(t, "PatchBooksBatch", err)
	requireEqual(t, "失败项数", len(failed), 0)
	requireEqual(t, "updated 保持入参顺序", fmt.Sprint(updated), fmt.Sprint(ids))
	for _, id := range ids {
		requirePatchedBook(t, mustBookDetail(t, ctx, st, id))
	}
}

// requirePatchedBook 断言一本书落齐了批量 patch 的全部字段（表驱动断言，无分支）。
func requirePatchedBook(t *testing.T, d *BookDetail) {
	t.Helper()
	requireEqual(t, "标题", d.Title, "统一书名")
	requireEqual(t, "系列", d.Series, "示例系列")
	requireEqual(t, "系列序号", d.SeriesIndex, 2)
	requireEqual(t, "评分", d.Rating, 4)
	requireEqual(t, "出版社", d.Publisher, "示例出版社")
	requireEqual(t, "出版日期", d.PubDate, "2024-05-01")
	requireEqual(t, "ISBN", d.ISBN, "978-7-000-00000-0")
	requireEqual(t, "语言", d.Language, "zh-CN")
	requireSliceEqual(t, "标签（去重后按名称升序）", d.Tags, []string{"悬疑", "玄幻"})
	requireSliceEqual(t, "作者", d.Authors, []string{"示例作者甲", "示例作者乙"})
}

func TestPatchBooksBatchIsolatesFailures(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := seedRatingBooks(t, ctx, st, 2)
	batch := []int64{ids[0], 999999, ids[1]}

	updated, failed, err := st.PatchBooksBatch(ctx, batch, BookMetaPatch{Rating: ptr(2)})
	requireOK(t, "PatchBooksBatch", err)
	requireEqual(t, "updated", fmt.Sprint(updated), fmt.Sprint(ids))
	requireEqual(t, "失败项数", len(failed), 1)
	requireErr(t, "不存在的书", failed[999999], ErrNotFound)
	for _, id := range ids {
		requireEqual(t, "别的书不受连坐", mustBookDetail(t, ctx, st, id).Rating, 2)
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
		requireErr(t, c.name, err, ErrInvalidArgument)
	}
}

func TestPatchBooksBatchDedupesIDs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedRatingBooks(t, ctx, st, 1)[0]

	updated, failed, err := st.PatchBooksBatch(ctx, []int64{id, id}, BookMetaPatch{Rating: ptr(1)})
	requireOK(t, "PatchBooksBatch", err)
	requireEqual(t, "失败项数", len(failed), 0)
	requireEqual(t, "去重后只有一条", fmt.Sprint(updated), fmt.Sprintf("[%d]", id))
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
	requireErr(t, "inSavepoint 透传 boom", err, boom)
	var title string
	requireOK(t, "读回标题", tx.QueryRowContext(ctx,
		"SELECT title FROM books WHERE id = ?", bookID).Scan(&title))
	requireEqual(t, "保存点已回滚", title != "不该留下的标题", true)
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
