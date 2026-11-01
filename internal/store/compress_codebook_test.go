package store

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 本文件锁死 0x03（码本 + zstd）格式的契约与老块兼容：
//   - 0x03 往返必须逐字节相同——私用区字符、emoji、非法 UTF-8 字节都不许被改写；
//   - 0x02（码本之前的 zstd 块）必须仍能读出；
//   - 同一个库里 0x02 与 0x03 混存时，逐章读取都要正确；
//   - 块目标 8 MB 下 splitBlocks 仍按章节边界切。

// TestCodebookRoundTripByteExact 覆盖 0x03 的重点字符集，逐字节比对。
//
// 样本都放大到 4 KB 以上：太短的块压缩后可能比原文还大，会命中 0x00 原文分支，
// 那样就测不到码本往返了。
func TestCodebookRoundTripByteExact(t *testing.T) {
	cases := []struct {
		name    string
		content string
		// want 是期望的算法标记：中文 / 私用区 / emoji 走 0x03；非法字节段与大量 ASCII
		// 会让码本流不短于原文（转义段要额外字节），按设计回退 0x02。
		want byte
	}{
		{"中文正文", strings.Repeat("长夜书简摊在灯下，青川剑砚压着纸角。", 200), contentAlgoCodebook},
		{"私用区字符", strings.Repeat("反爬站自造的私用区：\uE0A1\uE4FF\uF8F0\n", 200), contentAlgoCodebook},
		{"emoji 与四字节字符", strings.Repeat("灯下的四字节字符：😀🌟🀄\n", 200), contentAlgoCodebook},
		{"非法 UTF-8 字节", strings.Repeat("\x80\xFF\xC3正文\xF5\n", 200), contentAlgoZstd},
		{"混合内容", strings.Repeat("ascii text \xfe 与中文😀\uE0A1\n", 200), contentAlgoZstd},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := []byte(c.content)
			packed, err := compressContent(raw)
			if err != nil {
				t.Fatalf("compressContent: %v", err)
			}
			if len(packed) == 0 {
				t.Fatalf("compressContent 返回空块（原文 %d 字节）", len(raw))
			}
			if packed[0] != c.want {
				t.Fatalf("首字节 = 0x%02x，期望 0x%02x", packed[0], c.want)
			}
			back, err := decompressContent(packed)
			if err != nil {
				t.Fatalf("decompressContent: %v", err)
			}
			if !bytes.Equal(back, raw) {
				t.Errorf("往返不一致：原文 %d 字节，还原 %d 字节", len(raw), len(back))
			}
		})
	}
}

// TestDecompressContentZstdCompat 0x02（码本之前的 zstd-best）块必须仍能读出。
//
// 格式位一旦写进磁盘就该永远读得回来，否则换算法就是一次静默数据丢失。
func TestDecompressContentZstdCompat(t *testing.T) {
	raw := []byte(strings.Repeat("码本之前的 zstd 老块，青川剑砚压着纸角。", 200))
	packed, err := compressLegacyZstd(raw)
	if err != nil {
		t.Fatalf("构造 0x02 老块: %v", err)
	}
	if packed[0] != contentAlgoZstd {
		t.Fatalf("首字节 = 0x%02x，期望 0x%02x", packed[0], contentAlgoZstd)
	}
	back, err := decompressContent(packed)
	if err != nil {
		t.Fatalf("decompressContent(0x02): %v", err)
	}
	if !bytes.Equal(back, raw) {
		t.Errorf("0x02 老块往返不一致：%d vs %d 字节", len(back), len(raw))
	}
}

// TestMixedAlgoBlocksBothReadable 同一个库混存 0x02 老块与 0x03 新块，两章都要读得对。
func TestMixedAlgoBlocksBothReadable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := mixedBook()
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, Limit: 10})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("ListChapters: %v，%d 章", err, len(page.Items))
	}

	// 把第二章改挂到手写的 0x02 老块上：第一章留在 0x03 新块里。
	attachLegacyBlock(t, ctx, st, res.BookID, page.Items[1].ID, []byte(book.Chapters[1].Content))

	for i, item := range page.Items {
		ch, err := st.GetChapter(ctx, res.BookID, item.ID)
		if err != nil {
			t.Fatalf("GetChapter(%d): %v", item.ID, err)
		}
		if want := book.Chapters[i].Content; ch.Content != want {
			t.Errorf("第 %d 章正文不一致（%d vs %d 字节）", i, len(ch.Content), len(want))
		}
	}
	assertBothAlgoMarkers(t, ctx, st, res.BookID)
}

// mixedBook 造一本两章的书：第一章留在新格式块里，第二章会被改挂到老 0x02 块上。
func mixedBook() *model.ParsedBook {
	return &model.ParsedBook{
		Title: "混存书", Author: "青川客", SourcePath: "/path/to/txt/mixed.txt",
		SourceName: "mixed.txt", Encoding: "utf-8", FileSize: 1 << 20,
		SHA256: strings.Repeat("c", 64), WordCount: 1600,
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: -1, Title: "第一章 码本块", WordCount: 800,
				Content: strings.Repeat("长夜书简的码本块，灯下摊开的旧札。", 200)},
			{Idx: 1, VolumeIdx: -1, Title: "第二章 老块", WordCount: 800,
				Content: strings.Repeat("青川剑砚的 zstd 老块，墨迹慢慢洇开。", 200)},
		},
	}
}

// attachLegacyBlock 新写一行 0x02 老块，并把这一章改挂过去（块内偏移 0）。
func attachLegacyBlock(t *testing.T, ctx context.Context, st *Store, bookID, chapterID int64, raw []byte) {
	t.Helper()
	legacy, err := compressLegacyZstd(raw)
	if err != nil {
		t.Fatalf("构造 0x02 老块: %v", err)
	}
	var seq int
	if err := st.ContentDB().QueryRowContext(ctx,
		"SELECT COALESCE(MAX(seq), 0) + 1 FROM chapter_blocks WHERE book_id = ?", bookID).Scan(&seq); err != nil {
		t.Fatalf("取块序号: %v", err)
	}
	res, err := st.ContentDB().ExecContext(ctx,
		"INSERT INTO chapter_blocks(book_id, seq, first_idx, last_idx, raw_bytes, packed) "+
			"VALUES (?, ?, 1, 1, ?, ?)", bookID, seq, len(raw), legacy)
	if err != nil {
		t.Fatalf("插入老块: %v", err)
	}
	blockID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("老块 id: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE chapters SET block_id = ?, block_off = 0, block_len = ? WHERE id = ?",
		blockID, len(raw), chapterID); err != nil {
		t.Fatalf("改挂老块: %v", err)
	}
}

// assertBothAlgoMarkers 确认库里确实混存着 0x02 与 0x03 两种块，否则这个测试没测到重点。
func assertBothAlgoMarkers(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	rows, err := st.ContentDB().QueryContext(ctx,
		"SELECT packed FROM chapter_blocks WHERE book_id = ? ORDER BY seq", bookID)
	if err != nil {
		t.Fatalf("查块标记: %v", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄

	seen := map[byte]bool{}
	for rows.Next() {
		var packed []byte
		if err := rows.Scan(&packed); err != nil {
			t.Fatalf("扫块: %v", err)
		}
		if len(packed) > 0 {
			seen[packed[0]] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历块: %v", err)
	}
	if !seen[contentAlgoZstd] || !seen[contentAlgoCodebook] {
		t.Errorf("库里的块标记 = %v，期望同时出现 0x02 与 0x03", seen)
	}
}

// TestSplitBlocksBoundaryAtEightMegabytes 8 MB 目标下的边界行为。
func TestSplitBlocksBoundaryAtEightMegabytes(t *testing.T) {
	if BlockTargetBytes != 8<<20 {
		t.Fatalf("BlockTargetBytes = %d，期望 8 MB（%d）", BlockTargetBytes, 8<<20)
	}
	cases := []struct {
		name  string
		sizes []int
		want  [][2]int
	}{
		// 合计正好 8 MB：闭区间边界，不切块。
		{"正好 8 MB 不切", []int{4 << 20, 4 << 20}, [][2]int{{0, 1}}},
		// 再超 1 字节就切：第二章被推到下一块。
		{"超 1 字节即切", []int{4 << 20, 4<<20 + 1}, [][2]int{{0, 0}, {1, 1}}},
		// 每章 3 MB：前两章 6 MB 一块，第三章再加就 9 MB 超限，后两章一块。
		{"按章节边界切", []int{3 << 20, 3 << 20, 3 << 20, 3 << 20}, [][2]int{{0, 1}, {2, 3}}},
		// 单章自己超过 8 MB：独占一块，不切碎正文。
		{"超长章独占", []int{100, 9 << 20, 100}, [][2]int{{0, 0}, {1, 1}, {2, 2}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitBlocks(c.sizes)
			if len(got) != len(c.want) {
				t.Fatalf("块数 = %d，期望 %d：%v", len(got), len(c.want), got)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("第 %d 块 = %v，期望 %v", i, got[i], c.want[i])
				}
			}
		})
	}
}

// TestDecompressContentCodebookCorrupt 0x03 载荷里的码本流坏了必须报 ErrCorruptContent，
// 不能返回半截正文——zstd 解得开、码本流解不开的那一类损坏。
func TestDecompressContentCodebookCorrupt(t *testing.T) {
	// 0xC3 是 2 字节码的首字节，后面没有次字节 → 码本流截断。
	payload, err := zstdCompressBest([]byte{0xC3})
	if err != nil {
		t.Fatalf("zstdCompressBest: %v", err)
	}
	packed := append([]byte{contentAlgoCodebook}, payload...)
	if _, err := decompressContent(packed); !errors.Is(err, ErrCorruptContent) {
		t.Errorf("err = %v，期望 ErrCorruptContent", err)
	}
}

// TestCodebookBlockCorruptKeepsBlockError 0x03 块损坏时 GetChapter 仍必须给出 *BlockError，
// 且带 book_id / seq / 章节范围：换算法不许让错误分类与定位信息退化。
func TestCodebookBlockCorruptKeepsBlockError(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	res, chapterID := seedCorruptCodebookBook(t, ctx, st)

	_, err := st.GetChapter(ctx, res.BookID, chapterID)
	var be *BlockError
	if !errors.As(err, &be) {
		t.Fatalf("错误应为 *BlockError，实际 %v", err)
	}
	checkBlockErrorLocates(t, be, res.BookID)
	checkBlockErrorSentinels(t, err)
}

// seedCorruptCodebookBook 造一本两章的书，把它唯一的块写成坏 0x03 块，返回书与首章 id。
func seedCorruptCodebookBook(t *testing.T, ctx context.Context, st *Store) (SaveResult, int64) {
	t.Helper()
	res, err := st.SaveBook(ctx, mixedBook(), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, Limit: 10})
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("ListChapters: %v，%d 章", err, len(page.Items))
	}
	corruptCodebookBlock(t, ctx, st, res.BookID)
	return res, page.Items[0].ID
}

// checkBlockErrorLocates 校验定位字段：book_id / seq / 章节范围都要带上。
func checkBlockErrorLocates(t *testing.T, be *BlockError, bookID int64) {
	t.Helper()
	if be.BookID != bookID || be.Seq != 1 || be.FirstIdx != 0 || be.LastIdx != 1 {
		t.Errorf("定位信息不完整：%+v", be)
	}
}

// checkBlockErrorSentinels 校验错误分类：块级损坏与正文损坏两个哨兵都要能命中。
func checkBlockErrorSentinels(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrBlockCorrupted) || !errors.Is(err, ErrCorruptContent) {
		t.Errorf("错误应同时 Is 到 ErrBlockCorrupted 与 ErrCorruptContent：%v", err)
	}
}

// corruptCodebookBlock 把本书第一块的载荷换成「合法 zstd + 坏码本流」，保留 0x03 标记。
func corruptCodebookBlock(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	payload, err := zstdCompressBest([]byte{0xC3})
	if err != nil {
		t.Fatalf("zstdCompressBest: %v", err)
	}
	broken := append([]byte{contentAlgoCodebook}, payload...)
	if _, err := st.ContentDB().ExecContext(ctx,
		"UPDATE chapter_blocks SET packed = ? WHERE book_id = ? AND seq = 1",
		broken, bookID); err != nil {
		t.Fatalf("写坏块: %v", err)
	}
}

// TestCodebookBlockSmallerThanLegacyZstd 体积对账：同一段中文正文，0x03（码本 + zstd）
// 必须比 0x02（纯 zstd）更小——码本层的价值就在这里，退化了要当场发现。
func TestCodebookBlockSmallerThanLegacyZstd(t *testing.T) {
	raw := realisticProse(2000, 7)
	legacy, err := compressLegacyZstd(raw)
	if err != nil {
		t.Fatalf("构造 0x02 块: %v", err)
	}
	codebook, err := compressContent(raw)
	if err != nil {
		t.Fatalf("compressContent: %v", err)
	}
	t.Logf("体积对账：原文 %d 字节；0x02 = %d 字节（%.2f%%）；0x03 = %d 字节（%.2f%%）；省 %d 字节（%.1f%%）",
		len(raw), len(legacy), pctOf(len(legacy), len(raw)),
		len(codebook), pctOf(len(codebook), len(raw)),
		len(legacy)-len(codebook), pctOf(len(legacy)-len(codebook), len(legacy)))
	if len(codebook) >= len(legacy) {
		t.Errorf("0x03 = %d 字节，未小于 0x02 = %d 字节", len(codebook), len(legacy))
	}
}

// realisticProse 拼一段「高频句 + 幂律长尾字」的正文。
//
// 为什么不直接重复同一句：那样压缩率会虚高到 0.1%，对账就没意义了。字频要像真实小说
// 那样头部集中（前 1000 字覆盖 89.3%），这里用 80% 取自常用句、20% 均匀长尾来近似，
// 压缩率落在 20~25% 的量级（0x02 24.5% / 0x03 21.6%，实测）。
func realisticProse(sentences, seed int) []byte {
	var b strings.Builder
	s := uint64(seed)
	for i := 0; i < sentences; i++ {
		sentence := prosePool[i%len(prosePool)]
		b.WriteString(sentence)
		runes := []rune(sentence)
		for j := 0; j < 14; j++ {
			s += 0x9E3779B97F4A7C15
			z := s
			z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
			z = (z ^ (z >> 27)) * 0x94D049BB133111EB
			z ^= z >> 31
			if z%10 < 8 {
				b.WriteRune(runes[z%uint64(len(runes))])
				continue
			}
			b.WriteRune(rune(0x4E00 + z%0x5000))
		}
		b.WriteString("。")
	}
	return []byte(b.String())
}

// pctOf 返回 n 占 total 的百分比（total 为 0 时返回 0）。
func pctOf(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

// TestCompressContentNonChineseUsesPlainZstd 非中文正文必须回退 0x02。
//
// 码本把 ASCII 落进 2 字节层，整段码本流比原文还长（实测英文 ≈200%），再压也不如
// 直接压 UTF-8；回退只看长度，是零成本判断。
func TestCompressContentNonChineseUsesPlainZstd(t *testing.T) {
	raw := asciiProse(20000)
	packed, err := compressContent(raw)
	if err != nil {
		t.Fatalf("compressContent: %v", err)
	}
	if packed[0] != contentAlgoZstd {
		t.Fatalf("首字节 = 0x%02x，期望回退到 0x%02x（码本对 ASCII 无收益）", packed[0], contentAlgoZstd)
	}
	back, err := decompressContent(packed)
	if err != nil {
		t.Fatalf("decompressContent: %v", err)
	}
	if !bytes.Equal(back, raw) {
		t.Errorf("往返不一致：%d vs %d 字节", len(back), len(raw))
	}
}

// asciiProse 拼一段伪随机英文正文（词表小、读起来像人写的，但不会压到极端低）。
func asciiProse(words int) []byte {
	pool := []string{"night", "river", "stone", "quiet", "valley", "letter", "wind",
		"harbor", "shadow", "silence", "morning", "lantern", "bridge", "winter"}
	var b strings.Builder
	s := uint64(3)
	for i := 0; i < words; i++ {
		s += 0x9E3779B97F4A7C15
		z := s
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		z ^= z >> 31
		b.WriteString(pool[z%uint64(len(pool))])
		b.WriteByte(' ')
	}
	return []byte(b.String())
}
