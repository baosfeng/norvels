package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/store"
)

// testBlock 是造库时的一块；packed 为空表示按 0x00 原文封装。
type testBlock struct {
	raw      []byte
	packed   []byte
	rawBytes int64 // 非 0 时覆盖 chapter_blocks.raw_bytes，用来造「块与记录不一致」
}

// testBook 是造库时的一本书：章节字节数 + 若干块。
type testBook struct {
	id     int64
	sizes  []int
	blocks []testBlock
}

// buildTestDB 造一对「主库 + 正文库」的最小库，返回两个路径。
//
// 布局与 0018 之后的真实库一致：
//
//	主库   norvels.db          chapters（块寻址与章节字节数）
//	正文库 norvels-content.db  chapter_blocks（packed BLOB）
//
// 正文库落在主库的兄弟路径上（就是 store.ContentDBPath 的推导结果），
// 因此默认推导这条路也被这些用例覆盖到了。
func buildTestDB(t *testing.T, books []testBook) (mainPath, contentPath string) {
	t.Helper()
	mainPath = filepath.Join(t.TempDir(), "norvels.db")
	contentPath = store.ContentDBPath(mainPath)
	main := openTestDB(t, mainPath, "CREATE TABLE chapters (id INTEGER PRIMARY KEY AUTOINCREMENT, "+
		"book_id INTEGER NOT NULL, idx INTEGER NOT NULL, block_len INTEGER NOT NULL DEFAULT 0)")
	content := openTestDB(t, contentPath, "CREATE TABLE chapter_blocks (id INTEGER PRIMARY KEY AUTOINCREMENT, "+
		"book_id INTEGER NOT NULL, seq INTEGER NOT NULL, raw_bytes INTEGER NOT NULL, packed BLOB NOT NULL)")
	for _, b := range books {
		if err := insertTestBook(t, main, content, b); err != nil {
			t.Fatalf("book %d: %v", b.id, err)
		}
	}
	return mainPath, contentPath
}

// openTestDB 建一个测试库并执行一条 DDL，返回连接（测试结束自动关）。
func openTestDB(t *testing.T, path, ddl string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("建测试库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}

// insertTestBook 写入一本书的块（正文库）与章节尺寸（主库）。
func insertTestBook(t *testing.T, main, content *sql.DB, b testBook) error {
	t.Helper()
	for i, blk := range b.blocks {
		packed := blk.packed
		if len(packed) == 0 {
			packed = append([]byte{storedRaw}, blk.raw...)
		}
		rawBytes := blk.rawBytes
		if rawBytes == 0 {
			rawBytes = int64(len(blk.raw))
		}
		if _, err := content.Exec("INSERT INTO chapter_blocks(book_id, seq, raw_bytes, packed) VALUES (?,?,?,?)",
			b.id, i+1, rawBytes, packed); err != nil {
			return err
		}
	}
	for i, n := range b.sizes {
		if _, err := main.Exec("INSERT INTO chapters(book_id, idx, block_len) VALUES (?,?,?)", b.id, i, n); err != nil {
			return err
		}
	}
	return nil
}

// sampleBooks 造三本测试书：单块原文、两块拼一本、码本块。
func sampleBooks(t *testing.T) []testBook {
	t.Helper()
	c, err := newCompressor()
	if err != nil {
		t.Fatalf("建压缩器失败: %v", err)
	}
	defer c.Close()

	b1 := []byte("第一章正文。第二章正文。")
	b2a := []byte("上一块正文。")
	b2b := []byte("下一块正文。")
	b3 := []byte("码本块正文字符串。")
	coded, err := packOne(c, b3, true)
	if err != nil {
		t.Fatalf("打包码本块失败: %v", err)
	}
	return []testBook{
		{id: 101, sizes: []int{15, len(b1) - 15}, blocks: []testBlock{{raw: b1}}},
		{id: 202, sizes: []int{len(b2a), len(b2b)}, blocks: []testBlock{{raw: b2a}, {raw: b2b}}},
		{id: 303, sizes: []int{len(b3)}, blocks: []testBlock{{raw: b3, packed: coded}}},
	}
}

// TestOpenReadOnlyRejectsWrites 锁住「只读」这件事：两个库上连 UPDATE 都必须被拒。
func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	mainPath, contentPath := buildTestDB(t, sampleBooks(t))
	db, err := openReadOnly(mainPath)
	if err != nil {
		t.Fatalf("只读打开主库失败: %v", err)
	}
	defer db.Close() //nolint:errcheck // 只读句柄
	cdb, err := openReadOnly(contentPath)
	if err != nil {
		t.Fatalf("只读打开正文库失败: %v", err)
	}
	defer cdb.Close() //nolint:errcheck // 只读句柄

	if _, err := cdb.Exec("UPDATE chapter_blocks SET raw_bytes = 0"); err == nil {
		t.Fatal("正文库只读连接执行 UPDATE 必须报错，否则工具可能改坏生产库")
	}
	if _, err := db.Exec("INSERT INTO chapters(book_id, idx, block_len) VALUES (1,1,1)"); err == nil {
		t.Fatal("主库只读连接执行 INSERT 必须报错")
	}
}

// TestLoadLibraryAggregates 校验全库一次聚合（扫正文库）：书数、块数、体积与分档书数。
func TestLoadLibraryAggregates(t *testing.T) {
	_, contentPath := buildTestDB(t, sampleBooks(t))
	db, err := openReadOnly(contentPath)
	if err != nil {
		t.Fatalf("只读打开正文库失败: %v", err)
	}
	defer db.Close() //nolint:errcheck // 只读句柄

	lib, sizes, err := loadLibrary(context.Background(), db)
	if err != nil {
		t.Fatalf("全库聚合失败: %v", err)
	}
	wantEqual(t, "书数", lib.Books, 3)
	wantEqual(t, "块数", lib.Blocks, 4)
	wantEqual(t, "逐本条数", len(sizes), 3)
	wantEqual(t, "首本 id", sizes[0].ID, int64(101))
	// 合成文本只有几十字节，块头占比高，压缩率会 > 1——这里只校验口径等式，不校验数值大小。
	if lib.RawBytes <= 0 || lib.PackedBytes <= 0 {
		t.Fatalf("体积口径异常: raw=%d packed=%d", lib.RawBytes, lib.PackedBytes)
	}
	if want := float64(lib.PackedBytes) / float64(lib.RawBytes); lib.Ratio != want {
		t.Fatalf("压缩率 %v 与体积之商 %v 不等", lib.Ratio, want)
	}
	wantEqual(t, "<4MB 档书数", lib.BinBooks[binSmall], 3)
}

// TestLoadSampleRebuildsBook 校验逐本还原：块来自正文库、章节尺寸来自主库。
func TestLoadSampleRebuildsBook(t *testing.T) {
	db, cdb := openPair(t, sampleBooks(t))

	c, err := newCompressor()
	if err != nil {
		t.Fatalf("建压缩器失败: %v", err)
	}
	defer c.Close()

	b, err := loadSample(context.Background(), db, cdb, 202, c)
	if err != nil {
		t.Fatalf("还原多块书失败: %v", err)
	}
	if string(b.raw) != "上一块正文。下一块正文。" {
		t.Fatalf("拼接结果错误: %q", b.raw)
	}
	if len(b.sizes) != 2 || b.sizes[0] != len("上一块正文。") {
		t.Fatalf("章节尺寸错误: %v", b.sizes)
	}
	if b.dbBlobBytes <= 0 {
		t.Fatalf("库内 BLOB 体积累加错误: %d", b.dbBlobBytes)
	}

	coded, err := loadSample(context.Background(), db, cdb, 303, c)
	if err != nil {
		t.Fatalf("还原码本块书失败: %v", err)
	}
	if string(coded.raw) != "码本块正文字符串。" {
		t.Fatalf("0x03 块还原错误: %q", coded.raw)
	}
}

// TestLoadSampleDetectsInconsistency 锁住两条数据一致性检查。
func TestLoadSampleDetectsInconsistency(t *testing.T) {
	c, err := newCompressor()
	if err != nil {
		t.Fatalf("建压缩器失败: %v", err)
	}
	defer c.Close()
	raw := []byte("六个字正文")

	bad := []testBook{
		{id: 1, sizes: []int{len(raw)}, blocks: []testBlock{{raw: raw, rawBytes: int64(len(raw)) + 5}}},
	}
	db1, cdb1 := openPair(t, bad)
	if _, err := loadSample(context.Background(), db1, cdb1, 1, c); err == nil {
		t.Fatal("块解压长度与 raw_bytes 不符必须报错")
	}

	sumBad := []testBook{
		{id: 2, sizes: []int{3}, blocks: []testBlock{{raw: raw}}},
	}
	db2, cdb2 := openPair(t, sumBad)
	if _, err := loadSample(context.Background(), db2, cdb2, 2, c); err == nil {
		t.Fatal("章节字节和与块正文不符必须报错")
	}
}

// openPair 造一对测试库并以只读方式打开（主库 + 正文库），返回两个连接。
func openPair(t *testing.T, books []testBook) (*sql.DB, *sql.DB) {
	t.Helper()
	mainPath, contentPath := buildTestDB(t, books)
	db, err := openReadOnly(mainPath)
	if err != nil {
		t.Fatalf("只读打开主库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cdb, err := openReadOnly(contentPath)
	if err != nil {
		t.Fatalf("只读打开正文库失败: %v", err)
	}
	t.Cleanup(func() { _ = cdb.Close() })
	return db, cdb
}

// TestPickBooksStratified 校验四档等额分位抽样：大书档全取、id 不扎堆。
func TestPickBooksStratified(t *testing.T) {
	sizes := make([]BookSize, 0, 300)
	for i := int64(1); i <= 200; i++ {
		sizes = append(sizes, BookSize{ID: i, Raw: 1 << 20})
	}
	for i := int64(201); i <= 280; i++ {
		sizes = append(sizes, BookSize{ID: i, Raw: 8 << 20})
	}
	for i := int64(281); i <= 295; i++ {
		sizes = append(sizes, BookSize{ID: i, Raw: 20 << 20})
	}
	for i := int64(296); i <= 300; i++ {
		sizes = append(sizes, BookSize{ID: i, Raw: 60 << 20})
	}

	ids := pickBooks(sizes, 20)
	if len(ids) != 20 {
		t.Fatalf("抽样 %d 本，期望 20 本", len(ids))
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for i := int64(296); i <= 300; i++ {
		if !seen[i] {
			t.Fatalf(">40MB 档只有 5 本，应全部入选，缺 %d", i)
		}
	}
	if len(pickBooks(sizes, 0)) != 0 {
		t.Fatal("-sample 0 应只做全库口径，不抽样")
	}
}

// TestRunEndToEnd 走完整流程：开两个库 → 抽样 → 五组实测 → 最大书 → 两份产物落盘。
func TestRunEndToEnd(t *testing.T) {
	path, contentPath := buildTestDB(t, sampleBooks(t))
	out := filepath.Join(t.TempDir(), "cb.md")
	if err := run(path, "", 3, out, 303); err != nil {
		t.Fatalf("全流程失败: %v", err)
	}
	blob, err := os.ReadFile(strings.TrimSuffix(out, ".md") + ".json")
	if err != nil {
		t.Fatalf("结果 JSON 未落盘: %v", err)
	}
	var res Result
	if err := json.Unmarshal(blob, &res); err != nil {
		t.Fatalf("结果 JSON 不可解析: %v", err)
	}
	assertAllGroupsPassed(t, res)
	assertLargestMeasured(t, res)
	// 默认推导出的正文库路径要写进产物（运维据此确认读的是哪个文件）。
	if res.ContentDBPath != contentPath {
		t.Fatalf("结果里的正文库路径 = %q，期望 %q", res.ContentDBPath, contentPath)
	}
}

// assertAllGroupsPassed 校验五组都有样本且往返零失败。
func assertAllGroupsPassed(t *testing.T, res Result) {
	t.Helper()
	wantEqual(t, "方案数", len(res.Groups), len(groups))
	for _, g := range res.Groups {
		if g.Failures != 0 || g.Books == 0 {
			t.Fatalf("%s: 应全部通过并有样本，得到 %+v", g.Key, g)
		}
	}
}

// assertLargestMeasured 校验最大书单独测的结果被带进产物。
func assertLargestMeasured(t *testing.T, res Result) {
	t.Helper()
	if res.Largest == nil {
		t.Fatal("最大书单独测缺失")
	}
	wantEqual(t, "最大书 id", res.Largest.BookID, int64(303))
	if !res.Largest.RoundTrip {
		t.Fatal("最大书整本往返校验未通过")
	}
}

// TestRunFailsOnMissingDB 校验打不开库时向上报错（而不是写出一份空报告）。
func TestRunFailsOnMissingDB(t *testing.T) {
	out := filepath.Join(t.TempDir(), "cb.md")
	if err := run(filepath.Join(t.TempDir(), "nope.db"), "", 1, out, 0); err == nil {
		t.Fatal("库不存在时必须报错")
	}
}

// TestRunHintsMissingContentDB 0018 之前的老库（块表在主库）要给出可执行的提示，
// 而不是把 sqlite 的 "unable to open database file" 直接抛给运维。
func TestRunHintsMissingContentDB(t *testing.T) {
	mainPath, _ := buildTestDB(t, sampleBooks(t))
	// 删掉正文库，模拟「块表还在主库」的老库形态。
	if err := os.Remove(store.ContentDBPath(mainPath)); err != nil {
		t.Fatalf("移除正文库: %v", err)
	}
	err := run(mainPath, "", 1, filepath.Join(t.TempDir(), "cb.md"), 0)
	if err == nil || !strings.Contains(err.Error(), "-content-db") {
		t.Fatalf("缺正文库时应提示 -content-db，得到 %v", err)
	}
}

// TestRunExplicitContentDB 显式给 -content-db：正文库可以和主库不在同一目录。
func TestRunExplicitContentDB(t *testing.T) {
	mainPath, contentPath := buildTestDB(t, sampleBooks(t))
	moved := filepath.Join(t.TempDir(), "content-elsewhere.db")
	if err := os.Rename(contentPath, moved); err != nil {
		t.Fatalf("搬移正文库: %v", err)
	}
	out := filepath.Join(t.TempDir(), "cb.md")
	if err := run(mainPath, moved, 2, out, 0); err != nil {
		t.Fatalf("显式 -content-db 全流程失败: %v", err)
	}
	blob, err := os.ReadFile(strings.TrimSuffix(out, ".md") + ".json")
	if err != nil {
		t.Fatalf("结果 JSON 未落盘: %v", err)
	}
	var res Result
	if err := json.Unmarshal(blob, &res); err != nil {
		t.Fatalf("结果 JSON 不可解析: %v", err)
	}
	if res.ContentDBPath != moved || res.Library.Blocks == 0 {
		t.Fatalf("显式正文库未生效：path=%q blocks=%d", res.ContentDBPath, res.Library.Blocks)
	}
}
