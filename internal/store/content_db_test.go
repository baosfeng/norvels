package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 本文件是「正文拆库」之后测试侧的公共适配：块表可能在主库（0018 之前），
// 也可能在正文库（0018 之后），跨库 JOIN 也不再成立。

// blockDB 返回「此刻块表所在的那个库」：0018 之前是主库，之后是正文库。
//
// 迁移类测试要跨两个阶段比对同一张表（0007/0017 阶段在主库、0018 之后在正文库），
// 写死任何一个库都会让另一半断言失效——因此按「表在哪」选库，而不是按版本判断。
func blockDB(t *testing.T, ctx context.Context, st *Store) *sql.DB {
	t.Helper()
	ok, err := st.contentTableExists(ctx)
	if err != nil {
		t.Fatalf("检查正文库块表: %v", err)
	}
	if ok {
		return st.cdb
	}
	return st.db
}

// scalarIntBlock 在「块表所在的库」上执行单值整数查询。
func scalarIntBlock(t *testing.T, ctx context.Context, st *Store, query string, args ...any) int64 {
	t.Helper()
	var v int64
	if err := blockDB(t, ctx, st).QueryRowContext(ctx, query, args...).Scan(&v); err != nil {
		t.Fatalf("查询 %q: %v", query, err)
	}
	return v
}

// assertSpansWithinBlocks 校验每章的 block_off/block_len 落在它所属块的原始字节内。
//
// 拆库前这一条是 chapters JOIN chapter_blocks 一条 SQL；跨库 JOIN 不成立，
// 等价物是「块字节读进 map（正文库）+ 章节逐行比对（主库）」——语义完全一致。
func assertSpansWithinBlocks(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	raw := blockRawBytes(t, ctx, st)
	rows, err := st.db.QueryContext(ctx,
		"SELECT id, block_id, block_off, block_len FROM chapters WHERE block_id > 0")
	if err != nil {
		t.Fatalf("读章节 span: %v", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄

	bad := 0
	for rows.Next() {
		var id, blockID int64
		var off, length int
		if err := rows.Scan(&id, &blockID, &off, &length); err != nil {
			t.Fatalf("扫描章节 span: %v", err)
		}
		size, ok := raw[blockID]
		if !ok {
			bad++ // 块不存在也算越界（悬空引用，见 ReconcileContent）
			continue
		}
		if off < 0 || off+length > int(size) {
			bad++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历章节 span: %v", err)
	}
	if bad != 0 {
		t.Errorf("%d 章的块内区间越界（或指向不存在的块）", bad)
	}
}

// blockRawBytes 读出块 id → 原始字节数（正文库）。
func blockRawBytes(t *testing.T, ctx context.Context, st *Store) map[int64]int64 {
	t.Helper()
	rows, err := blockDB(t, ctx, st).QueryContext(ctx, "SELECT id, raw_bytes FROM chapter_blocks")
	if err != nil {
		t.Fatalf("读块原始字节: %v", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄

	out := make(map[int64]int64, 64)
	for rows.Next() {
		var id, raw int64
		if err := rows.Scan(&id, &raw); err != nil {
			t.Fatalf("扫描块原始字节: %v", err)
		}
		out[id] = raw
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历块原始字节: %v", err)
	}
	return out
}

// TestGetChapterSpanCorruptionMentionsBlock 章节的 block_off/block_len 越出块时，
// 读路径要报成 *BlockError（带块定位），而不是笼统的 500 —— 这是块级故障的定位依据。
//
// 拆库后块在正文库、span 在主库，最容易出的正是「两库不同步」这类越界。
func TestGetChapterSpanCorruptionMentionsBlock(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	res := saveOrFail(t, ctx, st, sampleBook("越界之书", "作者戊", "/books/span.txt", strings.Repeat("s", 64)))
	chapterID := firstChapterID(t, ctx, st, res.BookID)
	// 把这一章的长度改到远超块原始字节：解压能成功，切片必然越界。
	if _, err := st.db.ExecContext(ctx,
		"UPDATE chapters SET block_len = block_len + 1048576 WHERE id = ?", chapterID); err != nil {
		t.Fatalf("篡改章节区间: %v", err)
	}
	_, err := st.GetChapter(ctx, res.BookID, chapterID)
	if err == nil {
		t.Fatal("越界区间必须报错")
	}
	var be *BlockError
	if !asBlockError(err, &be) {
		t.Fatalf("错误不是 *BlockError：%v", err)
	}
	if be.BookID != res.BookID || be.ChapterID != chapterID || be.Seq <= 0 {
		t.Errorf("定位信息不全：%+v", be)
	}
}

// TestOpenFailsWhenContentDBUnusable 正文库打不开时整体打开失败：
// 只拿到主库连接就继续跑，等于让所有正文读写悄悄落到错的地方。
func TestOpenFailsWhenContentDBUnusable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad-content.db")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatalf("造目录: %v", err)
	}
	t.Setenv(EnvContentDB, bad)
	if _, err := Open(ctx, filepath.Join(dir, "main.db")); err == nil {
		t.Fatal("正文库路径是目录时必须打开失败")
	}
}

// TestOpenRejectsEmptyPath 空路径直接报错（不让上层拿到一个半初始化的 Store）。
func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("空路径必须报错")
	}
}
