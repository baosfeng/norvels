package recstore

import (
	"context"
	"testing"
	"time"
)

// 脏数据用例：SQLite 是动态类型，TEXT 能写进 INTEGER/REAL 列，直到扫描时才报错。
// 这里确认数据层返回错误（而不是 panic 或静默吞掉），顺带覆盖各扫描分支的失败路径。
func TestDirtyRowsSurfaceAsErrors(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	exec := func(q string) {
		t.Helper()
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("写脏数据失败: %v", err)
		}
	}

	exec("INSERT INTO negative_items(user_id, book_id, ts) VALUES (1, 'x', '')")
	if _, err := s.NegativeItems(ctx, 1); err == nil {
		t.Fatal("负反馈里脏 book_id 应报错")
	}

	exec("INSERT INTO negative_authors(user_id, author_id, ts) VALUES (1, 'x', '')")
	if _, err := s.NegativeAuthors(ctx, 1); err == nil {
		t.Fatal("作者拉黑里脏 author_id 应报错")
	}

	exec("INSERT INTO user_interest(user_id, dim, key, weight, updated_at) VALUES (1, 'category_l1', '武侠', 'x', '')")
	if _, err := s.InterestsAt(ctx, 1, nowUTC()); err == nil {
		t.Fatal("兴趣表里脏 weight 应报错")
	}
	// 写入路径同样要暴露：累加时会先读旧权重（decayedWeight）。
	if _, err := s.RecordEvents(ctx, 1, []Event{{Kind: KindClick, BookID: 1, CategoryL1: "武侠"}}); err == nil {
		t.Fatal("脏权重应让写入失败而不是静默覆盖")
	}
}

// 查询侧的脏数据：同样是 SQLite 动态类型带来的「扫描时才报错」，
// 确认这些查询方法返回错误而不是 panic / 静默丢行。
func TestDirtyRowsInQueries(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	exec := func(q string) {
		t.Helper()
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("写脏数据失败: %v", err)
		}
	}

	exec("INSERT INTO cooccur(book_id, other_id, users) VALUES (1, 2, 'x')")
	if _, err := s.CooccurForBook(ctx, 1, 10); err == nil {
		t.Fatal("共读表脏 users 应报错")
	}

	// ts 用当前时间，查询窗口才筛得到这一行（否则脏行被窗口过滤掉，测不到扫描分支）。
	exec("INSERT INTO recent_items(user_id, book_id, ts) VALUES (1, 'x', '" +
		formatTime(nowUTC()) + "')")
	if _, err := s.RecentImpressions(ctx, 1, nowUTC().Add(-time.Hour)); err == nil {
		t.Fatal("曝光表脏 book_id 应报错")
	}

	exec("INSERT INTO events(user_id, kind, book_id, author_id, category_l1, category_l2, value, extra, ts) " +
		"VALUES (1, 'read', 'x', 0, '', '', 0, '', '')")
	if _, err := s.ReadBooks(ctx, 1); err == nil {
		t.Fatal("事件表脏 book_id 应报错")
	}
}

func TestCooccurForBook(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	// 另一个账号把 1/2/3 都读了 → 生成共读对。
	recordEvents(t, s, 9,
		Event{Kind: KindRead, BookID: 1},
		Event{Kind: KindRead, BookID: 2},
		Event{Kind: KindRead, BookID: 3},
	)
	pairs, err := s.CooccurForBook(ctx, 1, 10)
	if err != nil {
		t.Fatalf("CooccurForBook: %v", err)
	}
	if len(pairs) != 2 {
		t.Fatalf("书 1 的共读邻居 = %+v，期望 2 个", pairs)
	}
	if _, err := s.CooccurForBook(ctx, 0, 10); err == nil {
		t.Fatal("book_id=0 应报错")
	}
}

func TestCooccurForBookOnClosedDB(t *testing.T) {
	s := openTemp(t)
	closeStore(t, s)
	if _, err := s.CooccurForBook(context.Background(), 1, 10); err == nil {
		t.Fatal("库已关闭应报错")
	}
}
