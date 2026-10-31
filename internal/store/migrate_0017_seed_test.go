package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// 本文件是 0017 迁移测试的造库与断言工具：手工写入 1 MB / 0x02 的老块，
// 再造出「已迁完的目标形态」供主测试逐项比对。主测试见 migrate_0017_test.go。

// seedLegacyBlocks 造一个停在版本 16、块形态是 1 MB / 0x02 的库。
//
// 大书 90 章 × 96 KB ≈ 8.6 MB：旧口径下切 9 块，新口径下 2 块——块数下降肉眼可见。
func seedLegacyBlocks(t *testing.T, withBigBook bool) (*Store, context.Context, []int64) {
	t.Helper()
	ctx := context.Background()
	st, _ := openBare(t, "v16-blocks.db")
	if _, err := st.MigrateTo(ctx, 16); err != nil {
		t.Fatalf("迁到版本 16: %v", err)
	}
	var books []int64
	if withBigBook {
		books = append(books, insertLegacyBookBlocks(t, ctx, st, 1, "长夜书简", bigBookChapterTexts()))
	}
	books = append(books,
		insertLegacyBookBlocks(t, ctx, st, 2, "荒渡灯影", smallBookChapterTexts()),
		insertLegacyBookBlocks(t, ctx, st, 3, "孤灯旅舍", []string{"短"}),
		// 全空书：每章都是空串，一块都不用建，用来锁死「整本书都是空章」的路径。
		insertLegacyBookBlocks(t, ctx, st, 4, "空卷", []string{"", "", ""}),
	)
	return st, ctx, books
}

// bigBookChapterTexts 生成 90 章、每章约 96 KB 的正文（总量略超 8 MB 目标）。
func bigBookChapterTexts() []string {
	out := make([]string, 90)
	for i := range out {
		out[i] = legacyChapterText(96<<10, i)
	}
	return out
}

// smallBookChapterTexts 生成一本含空章的小书：空章用来锁死「全空区间不建块行」。
func smallBookChapterTexts() []string {
	return []string{
		"长夜书简摊在灯下，青川剑砚压着纸角。",
		"",
		"窗外的风穿过廊下，铜铃一声接一声地响。",
		variedProse(40),
	}
}

// legacyChapterText 拼出约 target 字节的中文正文：散文与高熵汉字混合，压缩率落在真实
// 小说的量级（而不是「同一段重复 N 遍」的假压缩）。salt 让各章内容互不相同。
func legacyChapterText(target, salt int) string {
	var b strings.Builder
	b.Grow(target + 512)
	s := uint64(salt)*0x9E3779B97F4A7C15 + 1
	for i := 0; b.Len() < target; i++ {
		b.WriteString(prosePool[(i*7+salt)%len(prosePool)])
		if i%6 == 5 {
			b.WriteString("\n")
		}
		for j := 0; j < 14; j++ {
			s += 0x9E3779B97F4A7C15
			z := s
			z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
			z = (z ^ (z >> 27)) * 0x94D049BB133111EB
			z ^= z >> 31
			b.WriteRune(rune(0x4E00 + z%0x5000))
		}
		b.WriteString("。")
	}
	return b.String()
}

// insertLegacyBookBlocks 写一本书：books 行 + chapters 行（block_id=0）+ 1 MB / 0x02 老块。
func insertLegacyBookBlocks(t *testing.T, ctx context.Context, st *Store, serial int, title string, contents []string) int64 {
	t.Helper()
	res, err := st.DB().ExecContext(ctx,
		"INSERT INTO books(title, author_id, source_path, source_name, file_size, file_sha256, "+
			"encoding, word_count, chapter_count, volume_count, intro, status) "+
			"VALUES (?, 0, ?, ?, 1024, ?, 'utf-8', 100, ?, 0, '', 'parsed')",
		title, fmt.Sprintf("/path/to/txt/legacy-%d.txt", serial),
		fmt.Sprintf("legacy-%d.txt", serial), fmt.Sprintf("%064d", serial), len(contents))
	if err != nil {
		t.Fatalf("插入书籍 %q: %v", title, err)
	}
	bookID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("取 book id: %v", err)
	}
	writeLegacyChapters(t, ctx, st, bookID, contents)
	return bookID
}

// writeLegacyChapters 按 1 MB 章节边界把正文打成老块（0x02）并回填块寻址。
func writeLegacyChapters(t *testing.T, ctx context.Context, st *Store, bookID int64, contents []string) {
	t.Helper()
	ids := make([]int64, len(contents))
	sizes := make([]int, len(contents))
	for i, c := range contents {
		res, err := st.DB().ExecContext(ctx,
			"INSERT INTO chapters(book_id, volume_id, idx, title, word_count) VALUES (?, 0, ?, ?, ?)",
			bookID, i, fmt.Sprintf("第%d章 旧块", i+1), len([]rune(c)))
		if err != nil {
			t.Fatalf("插入章节 %d: %v", i, err)
		}
		if ids[i], err = res.LastInsertId(); err != nil {
			t.Fatalf("取 chapter id: %v", err)
		}
		sizes[i] = len(c)
	}
	seq := 0
	for _, span := range legacySpans(sizes) {
		first, last := span[0], span[1]
		raw, offsets := joinChapters(contents[first : last+1])
		if len(raw) == 0 {
			continue // 全空区间不建块行（与 0007 写入侧同口径）
		}
		seq++
		packed := legacyPacked(t, raw)
		res, err := st.DB().ExecContext(ctx,
			"INSERT INTO chapter_blocks(book_id, seq, first_idx, last_idx, raw_bytes, packed) "+
				"VALUES (?, ?, ?, ?, ?, ?)", bookID, seq, first, last, len(raw), packed)
		if err != nil {
			t.Fatalf("插入老块 seq=%d: %v", seq, err)
		}
		blockID, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("取 block id: %v", err)
		}
		for i := first; i <= last; i++ {
			if _, err := st.DB().ExecContext(ctx,
				"UPDATE chapters SET block_id = ?, block_off = ?, block_len = ? WHERE id = ?",
				blockID, offsets[i-first], len(contents[i]), ids[i]); err != nil {
				t.Fatalf("回填章节 %d: %v", ids[i], err)
			}
		}
	}
}

// legacyBlockTarget 是 0007 时代的块目标（1 MB），与今天的 BlockTargetBytes 不同。
const legacyBlockTarget = 1 << 20

// legacySpans 是按 1 MB 目标、章节边界切块的旧口径（今天 splitBlocks 硬编码 8 MB）。
func legacySpans(sizes []int) [][2]int {
	if len(sizes) == 0 {
		return nil
	}
	blocks := make([][2]int, 0, len(sizes)/8+1)
	first, acc := 0, 0
	for i, n := range sizes {
		if acc > 0 && acc+n > legacyBlockTarget {
			blocks = append(blocks, [2]int{first, i - 1})
			first, acc = i, 0
		}
		acc += n
	}
	return append(blocks, [2]int{first, len(sizes) - 1})
}

// legacyPacked 产出 0x02 老块字节：极短块退回 0x00（与 compressContent 的下限一致）。
func legacyPacked(t *testing.T, raw []byte) []byte {
	t.Helper()
	if len(raw) < contentRawThreshold {
		return append([]byte{contentAlgoRaw}, raw...)
	}
	out, err := zstdCompressBest(raw)
	if err != nil {
		t.Fatalf("压老块: %v", err)
	}
	return append([]byte{contentAlgoZstd}, out...)
}

// ---------- 断言 ----------

// digestsByBook 逐本算正文指纹（迁移前后各算一次即「正文没变」的直接证据）。
func digestsByBook(t *testing.T, ctx context.Context, st *Store, books []int64) map[int64]uint64 {
	t.Helper()
	out := make(map[int64]uint64, len(books))
	for _, id := range books {
		d, err := st.BookContentDigest(ctx, id)
		if err != nil {
			t.Fatalf("BookContentDigest(%d): %v", id, err)
		}
		out[id] = d
	}
	return out
}

// blocksPerBook 统计每本书的块数。
func blocksPerBook(t *testing.T, ctx context.Context, st *Store) map[int64]int {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx, "SELECT book_id, COUNT(*) FROM chapter_blocks GROUP BY book_id")
	if err != nil {
		t.Fatalf("统计块数: %v", err)
	}
	defer rows.Close()
	out := make(map[int64]int, 8)
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatalf("扫描块数: %v", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历块数: %v", err)
	}
	return out
}

// blockSnapshot 快照块表（id / seq / raw_bytes / packed 长度），用于幂等比对。
func blockSnapshot(t *testing.T, ctx context.Context, st *Store) []string {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx,
		"SELECT id, book_id, seq, raw_bytes, length(packed) FROM chapter_blocks ORDER BY id")
	if err != nil {
		t.Fatalf("快照块表: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, bookID int64
		var seq, raw, packed int
		if err := rows.Scan(&id, &bookID, &seq, &raw, &packed); err != nil {
			t.Fatalf("扫描块快照: %v", err)
		}
		out = append(out, fmt.Sprintf("%d|%d|%d|%d|%d", id, bookID, seq, raw, packed))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历块快照: %v", err)
	}
	return out
}

// assertNoLegacyAlgo 断言库里已经没有 0x01/0x02 的老块。
func assertNoLegacyAlgo(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if got := scalarInt(t, ctx, st,
		"SELECT COUNT(*) FROM chapter_blocks WHERE substr(packed,1,1) IN (x'01', x'02')"); got != 0 {
		t.Errorf("仍有 %d 个 0x01/0x02 老块", got)
	}
	if got := scalarInt(t, ctx, st,
		"SELECT COUNT(*) FROM chapter_blocks WHERE substr(packed,1,1) NOT IN (x'00', x'03')"); got != 0 {
		t.Errorf("%d 个块首字节既不是 0x00 也不是 0x03", got)
	}
}

// assertBlockShape 断言块大小、章节区间与字节守恒三件事同时对。
func assertBlockShape(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if got := scalarIntArg(t, ctx, st, "SELECT COUNT(*) FROM chapter_blocks WHERE raw_bytes > ?", int64(BlockTargetBytes)); got != 0 {
		t.Errorf("%d 个块超过 8 MB 目标", got)
	}
	if got := scalarInt(t, ctx, st, "SELECT COUNT(*) FROM chapter_blocks WHERE first_idx > last_idx"); got != 0 {
		t.Errorf("%d 个块的 first_idx > last_idx", got)
	}
	if got := scalarInt(t, ctx, st, "SELECT COUNT(*) FROM chapters WHERE block_id = 0 AND block_len <> 0"); got != 0 {
		t.Errorf("%d 章有长度却没有块归属", got)
	}
	if got := scalarInt(t, ctx, st, "SELECT COUNT(*) FROM chapters c JOIN chapter_blocks b ON b.id = c.block_id "+
		"WHERE c.block_off < 0 OR c.block_off + c.block_len > b.raw_bytes"); got != 0 {
		t.Errorf("%d 章的块内区间越界", got)
	}
	// 块内字节守恒：一块由整数章拼接而成，各章长度之和必须等于块原始字节数。
	end := scalarInt(t, ctx, st, "SELECT COALESCE(SUM(block_len),0) FROM chapters")
	raw := scalarInt(t, ctx, st, "SELECT COALESCE(SUM(raw_bytes),0) FROM chapter_blocks")
	if end != raw {
		t.Errorf("各章长度之和 %d ≠ 块原始字节之和 %d", end, raw)
	}
}

// assertDigestsEqual 逐本比对迁移前后的正文指纹。
func assertDigestsEqual(t *testing.T, ctx context.Context, st *Store, before map[int64]uint64) {
	t.Helper()
	after := digestsByBook(t, ctx, st, keysOf(before))
	if len(after) != len(before) {
		t.Fatalf("书数不一致：迁移前 %d，迁移后 %d", len(before), len(after))
	}
	for id, want := range before {
		if got := after[id]; got != want {
			t.Errorf("书 %d 正文指纹变了：%x → %x", id, want, got)
		}
	}
	t.Logf("逐本比对：%d 本书的正文指纹全部一致", len(before))
}

// assertSnapshotEqual 比对两份块表快照。
func assertSnapshotEqual(t *testing.T, got, want []string, label string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s：块数变了，%d → %d", label, len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s：第 %d 块变了，%s → %s", label, i, want[i], got[i])
		}
	}
}

// assertBlockErrorMentions 断言错误里点明了 book_id 与 block_seq。
func assertBlockErrorMentions(t *testing.T, err error, bookID int64, seq int) {
	t.Helper()
	msg := err.Error()
	for _, want := range []string{fmt.Sprintf("book_id=%d", bookID), fmt.Sprintf("block_seq=%d", seq)} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息缺少 %q：%v", want, err)
		}
	}
	if !strings.Contains(msg, fmt.Sprintf("book %d:", bookID)) {
		t.Errorf("错误信息缺少书名定位：%v", err)
	}
}

// assertMigrationRolledBack 断言失败后版本没前进、块表原样、重建目标表没留下。
func assertMigrationRolledBack(t *testing.T, ctx context.Context, st *Store, wantVersion int, before []string) {
	t.Helper()
	if v, err := st.maxRecordedVersion(ctx); err != nil || v != wantVersion {
		t.Errorf("失败后版本 = %d（err=%v），期望停在 %d", v, err, wantVersion)
	}
	assertSnapshotEqual(t, blockSnapshot(t, ctx, st), before, "回滚后")
	if exists, err := st.tableExists(ctx, newBlocksTable); err != nil || exists {
		t.Errorf("回滚后不应残留 %s（err=%v）", newBlocksTable, err)
	}
}

// scalarIntArg 执行带参数的单值整数查询。
func scalarIntArg(t *testing.T, ctx context.Context, st *Store, query string, args ...any) int64 {
	t.Helper()
	var v int64
	if err := st.DB().QueryRowContext(ctx, query, args...).Scan(&v); err != nil {
		t.Fatalf("查询 %q: %v", query, err)
	}
	return v
}

// keysOf 返回 map 的键（顺序无关，仅用于逐本比对）。
func keysOf(m map[int64]uint64) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
