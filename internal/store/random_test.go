package store

import (
	"context"
	"fmt"
	"testing"
)

// 随机取一本的口径：
//   - 只在「当前版本」（superseded_by = 0）里抽；
//   - 无筛选走 id 采样，带筛选（作者 / 标签 / 书单…）仍走 bookFilter 的同一份条件；
//   - 抽不到（空库 / 筛选无命中）返回 (nil, nil)，由 api 层映射 404。

// insertRandomBook 插一本当前版本的书并返回 id。
func insertRandomBook(t *testing.T, st *Store, title, sha string, authorID int64) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := st.DB().ExecContext(ctx,
		"INSERT INTO books(title, author_id, source_path, file_sha256, updated_at) "+
			"VALUES (?, ?, ?, ?, '2025-01-01T00:00:00.000Z')",
		title, authorID, "/path/to/txt/"+sha+".txt", sha)
	if err != nil {
		t.Fatalf("插入 %s: %v", title, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("取 id: %v", err)
	}
	return id
}

// TestRandomBookEmptyLibrary 空库返回 (nil, nil)：这不是错误，api 层据此回 404。
func TestRandomBookEmptyLibrary(t *testing.T) {
	st := newTestStore(t)
	got, err := st.RandomBook(context.Background(), BookQuery{})
	if err != nil {
		t.Fatalf("空库随机取书: %v", err)
	}
	if got != nil {
		t.Fatalf("空库应返回 nil，得到 id=%d", got.ID)
	}
}

// TestRandomBookSkipsSuperseded 被替换掉的旧版本永远不会被抽到（版本替换语义）。
func TestRandomBookSkipsSuperseded(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID := insertRandomBook(t, st, "旧版本", "old-sha", 0)
	insertRandomBook(t, st, "新版本", "new-sha", 0)
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE books SET superseded_by = (SELECT MAX(id) FROM books) WHERE id = ?", oldID); err != nil {
		t.Fatalf("标记替换: %v", err)
	}
	for i := 0; i < 50; i++ {
		got, err := st.RandomBook(ctx, BookQuery{})
		if err != nil {
			t.Fatalf("第 %d 次随机取书: %v", i, err)
		}
		if got == nil {
			t.Fatalf("第 %d 次抽样没有返回书", i)
		}
		if got.ID == oldID {
			t.Fatalf("抽到了被替换的旧版本 id=%d", oldID)
		}
	}
}

// TestRandomBookFilteredByAuthor 带作者筛选时只在该作者的书里抽。
func TestRandomBookFilteredByAuthor(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.DB().ExecContext(ctx, "INSERT INTO authors(name) VALUES ('甲作者'), ('乙作者')"); err != nil {
		t.Fatalf("插入作者: %v", err)
	}
	var firstAuthor int64
	if err := st.DB().QueryRowContext(ctx, "SELECT MIN(id) FROM authors").Scan(&firstAuthor); err != nil {
		t.Fatalf("取作者 id: %v", err)
	}
	mine := insertRandomBook(t, st, "甲的作品", "a-sha", firstAuthor)
	insertRandomBook(t, st, "乙的作品", "b-sha", firstAuthor+1)
	for i := 0; i < 30; i++ {
		got, err := st.RandomBook(ctx, BookQuery{AuthorID: firstAuthor})
		if err != nil {
			t.Fatalf("按作者随机: %v", err)
		}
		if got == nil || got.ID != mine {
			t.Fatalf("按作者随机取到 %v，期望只有 id=%d", got, mine)
		}
	}
}

// TestRandomBookFilteredNoMatch 筛选无命中返回 (nil, nil) 而不是随便塞一本。
func TestRandomBookFilteredNoMatch(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	insertRandomBook(t, st, "有书", "only-sha", 0)
	got, err := st.RandomBook(ctx, BookQuery{Tag: "不存在的标签"})
	if err != nil {
		t.Fatalf("按标签随机: %v", err)
	}
	if got != nil {
		t.Fatalf("无命中应返回 nil，得到 id=%d", got.ID)
	}
}

// TestRandomBookVariesAcrossDraws 多次抽样要能抽到不同的书（不是每次都同一条）。
func TestRandomBookVariesAcrossDraws(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for i := 0; i < 5; i++ {
		insertRandomBook(t, st, fmt.Sprintf("书%d", i), fmt.Sprintf("sha-%d", i), 0)
	}
	seen := make(map[int64]bool, 5)
	for i := 0; i < 40; i++ {
		got, err := st.RandomBook(ctx, BookQuery{})
		if err != nil {
			t.Fatalf("随机取书: %v", err)
		}
		seen[got.ID] = true
	}
	if len(seen) < 2 {
		t.Fatalf("40 次抽样只出现 %d 种结果，随机性可疑", len(seen))
	}
}
