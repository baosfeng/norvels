package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 本文件覆盖「写入侧块打包 + 读取侧块解压」的往返一致性。
//
// migrate_0007_test.go 验证的是迁移路径，这里验证的是**日常导入路径**
// （SaveBook → writeChapterBlocks → GetChapter），两条路径共用同一套
// splitBlocks/joinChapters/chapterAt，若口径不一致会在这里先炸。

// TestSaveBookBlocksRoundTrip 大书导入后逐章内容必须与原文一致。
func TestSaveBookBlocksRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := bigBlockBook(120)

	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, Limit: 500})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	if len(page.Items) != len(book.Chapters) {
		t.Fatalf("章节数 = %d，期望 %d", len(page.Items), len(book.Chapters))
	}
	for i, item := range page.Items {
		ch, err := st.GetChapter(ctx, res.BookID, item.ID)
		if err != nil {
			t.Fatalf("GetChapter(%d): %v", item.ID, err)
		}
		if ch.Content != book.Chapters[i].Content {
			t.Errorf("第 %d 章正文不一致（%d vs %d 字节）",
				i, len(ch.Content), len(book.Chapters[i].Content))
		}
	}
}

// TestSaveBookBlocksSplitIntoMultiple 足够大的书应被切成多块。
func TestSaveBookBlocksSplitIntoMultiple(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := bigBlockBook(120)
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	var blocks int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapter_blocks WHERE book_id = ?", res.BookID).Scan(&blocks); err != nil {
		t.Fatalf("count blocks: %v", err)
	}
	if blocks < 2 {
		t.Errorf("块数 = %d，期望多块（本书正文约 %d 字节，超过 1 MB 目标）", blocks, totalBytes(book))
	}
	assertBlockChain(t, ctx, st, res.BookID, len(book.Chapters))
}

// assertBlockChain 校验块链条完整：序号连续、章节区间无空洞无重叠、末块覆盖到最后一章。
//
// 这三个不变量任一被破坏，读取侧都会切出错位的正文（比如读到「上一章尾巴 + 本章开头」），
// 而且是静默的——所以必须在写入侧就守住。
func assertBlockChain(t *testing.T, ctx context.Context, st *Store, bookID int64, chapterCount int) {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx,
		"SELECT seq, first_idx, last_idx, raw_bytes FROM chapter_blocks WHERE book_id = ? ORDER BY seq",
		bookID)
	if err != nil {
		t.Fatalf("query blocks: %v", err)
	}
	defer rows.Close()

	prevSeq, prevLast := 0, -1
	for rows.Next() {
		var seq, first, last, raw int
		if err := rows.Scan(&seq, &first, &last, &raw); err != nil {
			t.Fatalf("scan block: %v", err)
		}
		if seq != prevSeq+1 {
			t.Errorf("块序号不连续：前 %d，本 %d", prevSeq, seq)
		}
		if first != prevLast+1 {
			t.Errorf("章节区间有空洞：前块末 %d，本块首 %d", prevLast, first)
		}
		if raw <= 0 {
			t.Errorf("块 seq=%d 的 raw_bytes = %d", seq, raw)
		}
		prevSeq, prevLast = seq, last
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历块: %v", err)
	}
	if prevLast != chapterCount-1 {
		t.Errorf("末块覆盖到 idx %d，期望 %d", prevLast, chapterCount-1)
	}
}

// TestSaveBookEmptyChaptersUseNoBlock 全空的书不应产生块行。
func TestSaveBookEmptyChaptersUseNoBlock(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := &model.ParsedBook{
		Title: "空卷书", Author: "青川客", SourcePath: "/path/to/txt/empty.txt",
		SourceName: "empty.txt", Encoding: "utf-8", FileSize: 1,
		SHA256: strings.Repeat("e", 64), WordCount: 0,
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: -1, Title: "第一章 空", Content: ""},
			{Idx: 1, VolumeIdx: -1, Title: "第二章 空", Content: ""},
		},
	}
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	var blocks int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapter_blocks WHERE book_id = ?", res.BookID).Scan(&blocks); err != nil {
		t.Fatalf("count blocks: %v", err)
	}
	if blocks != 0 {
		t.Errorf("全空书的块数 = %d，期望 0（不应为空内容建块）", blocks)
	}
	// 空章仍要能正常读出（block_id=0 直接返回空串）。
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, Limit: 10})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	for _, item := range page.Items {
		ch, err := st.GetChapter(ctx, res.BookID, item.ID)
		if err != nil {
			t.Fatalf("GetChapter: %v", err)
		}
		if ch.Content != "" {
			t.Errorf("空章读出了内容：%q", ch.Content)
		}
	}
}

// TestForceOverwriteReplacesBlocks 覆盖写入后块内容也必须换新（不能留下旧块）。
func TestForceOverwriteReplacesBlocks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := bigBlockBook(120)
	if _, err := st.SaveBook(ctx, book, SaveOptions{}); err != nil {
		t.Fatalf("首次保存: %v", err)
	}

	// 改一章内容后强制覆盖。
	book.Chapters[0].Content = "改写后的开篇：长夜书简摊开，灯芯爆了一下。"
	book.SHA256 = strings.Repeat("f", 64)
	res, err := st.SaveBook(ctx, book, SaveOptions{Force: true})
	if err != nil {
		t.Fatalf("Force 保存: %v", err)
	}
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, Limit: 10})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	ch, err := st.GetChapter(ctx, res.BookID, page.Items[0].ID)
	if err != nil {
		t.Fatalf("GetChapter: %v", err)
	}
	if ch.Content != book.Chapters[0].Content {
		t.Errorf("覆盖后首章正文未更新：%q", ch.Content)
	}
}

// bigBlockBook 造一本 n 章、合计超过 1 MB 的书（每章约 12 KB）。
func bigBlockBook(n int) *model.ParsedBook {
	book := &model.ParsedBook{
		Title: "长夜书简", Author: "青川客", SourcePath: "/path/to/txt/big.txt",
		SourceName: "big.txt", Encoding: "utf-8", FileSize: 1 << 20,
		SHA256: strings.Repeat("b", 64),
	}
	for i := 0; i < n; i++ {
		book.Chapters = append(book.Chapters, model.ParsedChapter{
			Idx:       i,
			VolumeIdx: -1,
			Title:     "第" + itoaForTest(i+1) + "章",
			Content:   strings.Repeat("长夜书简的青川剑砚，灯下摊开的旧札。", 500) + "\n",
			WordCount: 8000,
		})
		book.WordCount += 8000
	}
	return book
}

// itoaForTest 是测试内的整数转字符串。
func itoaForTest(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// totalBytes 求一本书的正文总字节。
func totalBytes(b *model.ParsedBook) int {
	n := 0
	for i := range b.Chapters {
		n += len(b.Chapters[i].Content)
	}
	return n
}
