package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 整章替换正文的口径：写新块 → 改 span 与 word_count → 同步三处计数 → 回收旧块。

// TestUpdateChapterContentReplacesText 替换正文后：GET 到的正文逐字节一致、
// 章 id / idx / 卷归属不变、新字数落入三处计数、书 updated_at 跟着刷新。
func TestUpdateChapterContentReplacesText(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedContentBook(t, ctx, st, "content-replace", []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "第一章", Content: "第一段正文。\n第二段正文。", WordCount: 12},
		{Idx: 1, VolumeIdx: 0, Title: "第二章", Content: "另一章的正文。", WordCount: 6},
	})
	// 把 updated_at 压到过去：否则「导入 + 替换」可能落在同一毫秒，断言「时间变了」会假失败。
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE books SET updated_at = ? WHERE id = ?", "2020-01-01T00:00:00.000Z", bookID); err != nil {
		t.Fatalf("重置 updated_at: %v", err)
	}
	before := bookByID(t, ctx, st, bookID)
	metricsBefore := mustMetrics(t, ctx, st)

	replacement := "替换后的正文。\n第二个段落，长度不同。"
	wantWords := countWords(replacement)
	requireInt(t, "样本 countWords（自校验样本口径）", wantWords, 15)

	if err := st.UpdateChapterContent(ctx, bookID, ids[0], replacement); err != nil {
		t.Fatalf("UpdateChapterContent: %v", err)
	}

	got := mustChapter(t, ctx, st, bookID, ids[0])
	requireString(t, "替换后正文（逐字节）", got.Content, replacement)
	if len(got.Content) != len(replacement) {
		t.Fatalf("替换后正文字节数 = %d，期望 %d", len(got.Content), len(replacement))
	}
	requireInt(t, "替换后章节字数", got.WordCount, wantWords)
	// 章 id / idx 不变：书签与阅读进度按 chapter id 指过来
	requireInt(t, "章 id", int(got.ID), int(ids[0]))
	requireInt(t, "章 idx", got.Idx, 0)
	briefs := chaptersOf(t, ctx, st, bookID)
	requireIdxSequence(t, briefs)
	requireInt(t, "章数（替换不增删章）", len(briefs), 2)
	// 第二章的正文与字数一个字节都不该动
	requireString(t, "第二章正文", mustChapter(t, ctx, st, bookID, ids[1]).Content, "另一章的正文。")
	requireInt(t, "第二章字数", briefs[1].WordCount, 6)

	detail := bookByID(t, ctx, st, bookID)
	requireInt(t, "book word_count", detail.WordCount, before.WordCount-12+wantWords)
	requireInt(t, "chapter_count", detail.ChapterCount, before.ChapterCount)
	if detail.UpdatedAt == before.UpdatedAt {
		t.Errorf("books.updated_at 未刷新：%q", detail.UpdatedAt)
	}
	after := mustMetrics(t, ctx, st)
	requireInt(t, "metrics chapter_word_sum",
		int(after.ChapterWordSum), int(metricsBefore.ChapterWordSum)-12+wantWords)
	// 旧块仍被第二章引用 → 不回收，正文体积只增（新正文的原始字节）
	requireInt(t, "metrics text_bytes_raw 增量",
		int(after.TextBytesRaw-metricsBefore.TextBytesRaw), len(replacement))
	t.Logf("替换：12 字 → %d 字；book word_count %d → %d；chapter_word_sum %d → %d；text_bytes_raw +%d",
		wantWords, before.WordCount, detail.WordCount,
		metricsBefore.ChapterWordSum, after.ChapterWordSum, after.TextBytesRaw-metricsBefore.TextBytesRaw)
}

// TestUpdateChapterContentReapsExclusiveBlock 章独占一块时，替换后旧块无人引用：
// 整块回收并把体积从 metrics 里冲减（「不回收就只涨不降」的回归测试）。
func TestUpdateChapterContentReapsExclusiveBlock(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	// 第一章超过单块目标 → splitBlocks 让它独占一块，第二章落到第二块。
	big := strings.Repeat("河堤上的风。", 60000) // 1,080,000 字节 > BlockTargetBytes
	bookID, ids := seedContentBook(t, ctx, st, "content-reap", []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "长章", Content: big, WordCount: 6000},
		{Idx: 1, VolumeIdx: 0, Title: "短章", Content: "短章正文。", WordCount: 4},
	})
	if len(big) <= BlockTargetBytes {
		t.Fatalf("样本 %d 字节没超过单块目标 %d，构造不出独占块", len(big), BlockTargetBytes)
	}
	blocksBefore := blockCountOf(t, ctx, st, bookID)
	oldBlockID := blockIDOf(t, ctx, st, bookID, ids[0])
	metricsBefore := mustMetrics(t, ctx, st)

	replacement := "改短了。"
	if err := st.UpdateChapterContent(ctx, bookID, ids[0], replacement); err != nil {
		t.Fatalf("UpdateChapterContent: %v", err)
	}

	if blockIDOf(t, ctx, st, bookID, ids[0]) == oldBlockID {
		t.Fatalf("章节仍指向旧块 %d，期望写新块", oldBlockID)
	}
	if n := blockRefCount(t, ctx, st, oldBlockID); n != 0 {
		t.Fatalf("旧块 %d 仍存在（%d 行），期望整块回收", oldBlockID, n)
	}
	requireInt(t, "块数（回收旧块 + 写入新块）", blockCountOf(t, ctx, st, bookID), blocksBefore)
	after := mustMetrics(t, ctx, st)
	if after.TextBytes >= metricsBefore.TextBytes {
		t.Fatalf("text_bytes 未下降：%d → %d（旧块没被冲减）",
			metricsBefore.TextBytes, after.TextBytes)
	}
	// 原始字节：旧块 raw（1,080,000）被减掉，新正文 raw 加回
	requireInt(t, "metrics text_bytes_raw 变化",
		int(after.TextBytesRaw-metricsBefore.TextBytesRaw), len(replacement)-len(big))
	t.Logf("回收：text_bytes %d → %d（-%d）；text_bytes_raw %d → %d；块数 %d（旧块 %d 已删）",
		metricsBefore.TextBytes, after.TextBytes, metricsBefore.TextBytes-after.TextBytes,
		metricsBefore.TextBytesRaw, after.TextBytesRaw, blockCountOf(t, ctx, st, bookID), oldBlockID)
	// 短章仍在，正文没被牵连
	requireString(t, "短章正文", mustChapter(t, ctx, st, bookID, ids[1]).Content, "短章正文。")
}

// TestUpdateChapterContentValidation 入参校验：空正文 / 超 1 MiB / 章节不属于本书。
func TestUpdateChapterContentValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedContentBook(t, ctx, st, "content-valid", []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "第一章", Content: "原有正文。", WordCount: 4},
	})
	otherBook, otherIDs := seedContentBook(t, ctx, st, "content-valid-other", []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "别人的章", Content: "别人的正文。", WordCount: 5},
	})
	over := strings.Repeat("字", maxChapterContentBytes/3+1) // 每个汉字 3 字节
	if len(over) <= maxChapterContentBytes {
		t.Fatalf("超限样本 %d 字节，未超过上限 %d", len(over), maxChapterContentBytes)
	}

	cases := []struct {
		name      string
		bookID    int64
		chapterID int64
		content   string
		want      error
	}{
		{"空正文", bookID, ids[0], "", ErrInvalidArgument},
		{"空白正文", bookID, ids[0], "  \n\t  ", ErrInvalidArgument},
		{"超 1 MiB", bookID, ids[0], over, ErrInvalidArgument},
		{"章节不属于本书", bookID, otherIDs[0], "新正文。", ErrNotFound},
		{"章节不存在", bookID, 999999, "新正文。", ErrNotFound},
		{"书不存在", 999999, ids[0], "新正文。", ErrNotFound},
	}
	for _, tc := range cases {
		err := st.UpdateChapterContent(ctx, tc.bookID, tc.chapterID, tc.content)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s：err = %v，期望 %v", tc.name, err, tc.want)
		}
	}
	// 被拒绝的请求一个字节都不改（fail-closed）
	requireString(t, "拒绝后正文未变", mustChapter(t, ctx, st, bookID, ids[0]).Content, "原有正文。")
	requireInt(t, "拒绝后字数未变", chaptersOf(t, ctx, st, bookID)[0].WordCount, 4)
	// 别人的书一个字都没动
	requireString(t, "他人的章正文未变", mustChapter(t, ctx, st, otherBook, otherIDs[0]).Content, "别人的正文。")
	// 恰好 1 MiB 通过（上限是闭区间）
	if err := st.UpdateChapterContent(ctx, bookID, ids[0], strings.Repeat("a", maxChapterContentBytes)); err != nil {
		t.Fatalf("恰好 1 MiB 应通过：%v", err)
	}
	// 错误信息必须写清上限，前端与日志都直接展示它
	err := st.UpdateChapterContent(ctx, bookID, ids[0], over)
	if err == nil || !strings.Contains(err.Error(), "1048576") {
		t.Fatalf("超限错误信息应写清上限 1048576：%v", err)
	}
	t.Logf("超限错误信息：%v", err)
}

// seedContentBook 写一本只有指定章节的书；source_path / sha 按 tag 区分，
// 避免同一测试里两次 seed 命中同一本书（SaveBook 按路径或 sha 判重）。
func seedContentBook(t *testing.T, ctx context.Context, st *Store, tag string,
	chapters []model.ParsedChapter) (int64, []int64) {
	t.Helper()
	sum := sha256.Sum256([]byte("norvels-store-test:" + tag))
	res, err := st.SaveBook(ctx, &model.ParsedBook{
		Title:      "正文编辑样本 " + tag,
		Author:     "样本作者",
		SourcePath: "/books/" + tag + ".txt",
		SourceName: tag + ".txt",
		Encoding:   "utf-8",
		FileSize:   2048,
		SHA256:     hex.EncodeToString(sum[:]),
		Volumes:    []model.ParsedVolume{{Idx: 0, Title: "第一卷"}},
		Chapters:   chapters,
	}, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook(%s): %v", tag, err)
	}
	briefs := chaptersOf(t, ctx, st, res.BookID)
	ids := make([]int64, 0, len(briefs))
	for _, b := range briefs {
		ids = append(ids, b.ID)
	}
	return res.BookID, ids
}

// blockCountOf 数一本书的正文块行数。
func blockCountOf(t *testing.T, ctx context.Context, st *Store, bookID int64) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapter_blocks WHERE book_id = ?", bookID).Scan(&n); err != nil {
		t.Fatalf("count blocks: %v", err)
	}
	return n
}

// blockIDOf 读一章当前指向的块 id。
func blockIDOf(t *testing.T, ctx context.Context, st *Store, bookID, chapterID int64) int64 {
	t.Helper()
	var id int64
	if err := st.DB().QueryRowContext(ctx,
		"SELECT block_id FROM chapters WHERE id = ? AND book_id = ?", chapterID, bookID).Scan(&id); err != nil {
		t.Fatalf("read block_id: %v", err)
	}
	return id
}

// blockRefCount 数某个块还剩几行（0 = 已被回收）。
func blockRefCount(t *testing.T, ctx context.Context, st *Store, blockID int64) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapter_blocks WHERE id = ?", blockID).Scan(&n); err != nil {
		t.Fatalf("count block rows: %v", err)
	}
	return n
}
