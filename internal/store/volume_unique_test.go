package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// 0013 之后「同书卷名唯一」是数据库约束：并发建同名卷只有一个能落库，
// 另一个拿到 ErrVolumeNameTaken（409），索引是竞态兜底。

// TestCreateVolumeConcurrentSameName 两个连接同时建同名卷：一成功一 ErrVolumeNameTaken。
func TestCreateVolumeConcurrentSameName(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedVolumeBook(t, ctx, st, "/books/concurrent13.txt", "f13")

	const workers = 2
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = st.CreateVolume(ctx, bookID, "并发卷")
		}(i)
	}
	close(start)
	wg.Wait()

	ok, taken := 0, 0
	for _, err := range errs {
		if err == nil {
			ok++
			continue
		}
		requireStoreErr(t, err, ErrVolumeNameTaken, "并发建同名卷的失败方")
		taken++
	}
	requireTrue(t, ok == 1 && taken == 1,
		"期望一成功一 ErrVolumeNameTaken，实际成功 %d 失败 %d（%v）", ok, taken, errs)
	requireTrue(t, bookVolumeCountOf(t, ctx, st, bookID) == 1,
		"库里应只留下 1 卷，实际 %d", bookVolumeCountOf(t, ctx, st, bookID))
}

// TestVolumeTitleUniqueIndexRejectsTrimEqual 唯一索引按 TRIM(title) 比较：
// 绕过 store 查重直接插一条「首尾空白不同」的同名卷也会被库拒绝。
func TestVolumeTitleUniqueIndexRejectsTrimEqual(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedVolumeBook(t, ctx, st, "/books/trim13.txt", "e13")
	if _, err := st.CreateVolume(ctx, bookID, "同卷"); err != nil {
		t.Fatalf("建首卷: %v", err)
	}
	_, err := st.DB().ExecContext(ctx,
		"INSERT INTO volumes(book_id, idx, title) VALUES (?, 9, ?)", bookID, " 同卷 ")
	requireTrue(t, err != nil, "TRIM 后同名的卷应被唯一索引拒绝，实际插入了")
	requireTrue(t, strings.Contains(err.Error(), volumeTitleUniqueIndex),
		"错误应点名索引 %s，实际：%v", volumeTitleUniqueIndex, err)
}

// TestRenameVolumeRejectsTakenNameAfterIndex 唯一索引生效后，改名撞名仍是 409 且不动库。
func TestRenameVolumeRejectsTakenNameAfterIndex(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids, _ := seedVolumeFixture(t, ctx, st, "/books/rename13.txt")
	_, err := st.RenameVolume(ctx, bookID, ids[1], " 卷一 ")
	requireStoreErr(t, err, ErrVolumeNameTaken, "改名撞已有卷名")
	requireVolumeIdxSeq(t, ctx, st, ids)
}

// seedVolumeBook 建一本没有卷的书，返回 bookID（并发用例只需要书存在）。
func seedVolumeBook(t *testing.T, ctx context.Context, st *Store, path, sha string) int64 {
	t.Helper()
	mustExec(t, ctx, st,
		"INSERT INTO books(title, source_path, file_sha256) VALUES ('并发卷书', '"+path+"', '"+sha+"')")
	var bookID int64
	if err := st.DB().QueryRowContext(ctx,
		"SELECT id FROM books WHERE source_path = ?", path).Scan(&bookID); err != nil {
		t.Fatalf("读 book id: %v", err)
	}
	return bookID
}

// TestCreateVolumeConcurrentSameNameAcrossStores 两个独立 Store（两个连接池，
// 等价于两个进程）同时建同名卷：仍然一成功一 ErrVolumeNameTaken。
func TestCreateVolumeConcurrentSameNameAcrossStores(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "two-stores.db")
	first := openMigratedStore(t, ctx, path)
	second := openMigratedStore(t, ctx, path)
	bookID := seedVolumeBook(t, ctx, first, "/books/twostores13.txt", "g13")

	errs := raceCreateVolume(ctx, []*Store{first, second}, bookID, "双进程卷")
	requireOneSuccessOneTaken(t, errs)
	requireTrue(t, bookVolumeCountOf(t, ctx, first, bookID) == 1, "库里应只留下 1 卷")
}

// openMigratedStore 打开指定路径的库并迁到最新（同一文件可开多个 Store）。
func openMigratedStore(t *testing.T, ctx context.Context, path string) *Store {
	t.Helper()
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate %s: %v", path, err)
	}
	return st
}

// raceCreateVolume 让每个 Store 各起一个 goroutine 同时建同名卷，返回各自错误。
func raceCreateVolume(ctx context.Context, stores []*Store, bookID int64, title string) []error {
	errs := make([]error, len(stores))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, st := range stores {
		wg.Add(1)
		go func(i int, st *Store) {
			defer wg.Done()
			<-start
			_, errs[i] = st.CreateVolume(ctx, bookID, title)
		}(i, st)
	}
	close(start)
	wg.Wait()
	return errs
}

// requireOneSuccessOneTaken 断言「恰好一个成功、其余都是 ErrVolumeNameTaken」。
func requireOneSuccessOneTaken(t *testing.T, errs []error) {
	t.Helper()
	ok, taken := 0, 0
	for _, err := range errs {
		if err == nil {
			ok++
			continue
		}
		requireStoreErr(t, err, ErrVolumeNameTaken, "并发建同名卷的失败方")
		taken++
	}
	requireTrue(t, ok == 1 && taken == len(errs)-1,
		"期望一成功一 ErrVolumeNameTaken，实际成功 %d 失败 %d（%v）", ok, taken, errs)
}
