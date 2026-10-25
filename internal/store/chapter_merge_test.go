package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 合并的分隔符口径：正文 == 各章正文按章序 join("\n")，逐字节相等。
// 用「章尾无句末标点 + 下一章起新句」的真实形态做样本——直拼会把两句连读成一句。
var mergeParts = []string{
	"远处有人拉起了手风琴",
	"夜雾沿着河堤漫上来",
	"他站在原地听着",
}

// TestMergeChaptersJoinsWithNewline 三章并一章：正文按章序用一个换行分隔，
// 首章 id 与标题保留，其余章删掉后 idx 连续，字数三处口径都不动。
func TestMergeChaptersJoinsWithNewline(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "merge-newline",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, mergePartsBook())
	before := bookByID(t, ctx, st, bookID)
	metricsBefore := mustMetrics(t, ctx, st)
	// countWords 口径：CJK 逐字计，样本共 10+9+7 = 26 字
	requireInt(t, "各章 countWords 之和", sumCountWords(mergeParts), 26)
	requireInt(t, "合并前首章字数", chaptersOf(t, ctx, st, bookID)[0].WordCount, 10)

	if err := st.MergeChapters(ctx, bookID, ids); err != nil {
		t.Fatalf("MergeChapters: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	if len(got) != 1 {
		t.Fatalf("合并后章数 = %d，期望 1", len(got))
	}
	requireIdxSequence(t, got)
	// 首章 id 必须留住（阅读进度与书签按它指过来），标题取首章原标题
	if got[0].ID != ids[0] {
		t.Errorf("首章 id = %d，期望 %d", got[0].ID, ids[0])
	}
	requireString(t, "合并后标题", got[0].Title, "第一章 手风琴")

	want := strings.Join(mergeParts, "\n")
	text := mustChapter(t, ctx, st, bookID, ids[0]).Content
	requireString(t, "合并后正文", text, want)
	// 逐字节相等 + 分隔符数量与位置：恰好两份换行，且不落在首尾
	if !strings.EqualFold(text, want) || len(text) != len(want) {
		t.Fatalf("合并后正文字节数 = %d，期望 %d", len(text), len(want))
	}
	requireInt(t, "正文内换行数", strings.Count(text, "\n"), len(mergeParts)-1)
	if strings.HasSuffix(text, "\n") || strings.HasPrefix(text, "\n") {
		t.Errorf("合并结果首尾不该多出换行：%q", text)
	}
	// 字数：插进来的换行是空白，countWords 不计；合并后重算与各章之和一致
	t.Logf("各章 countWords = %d，合并后 countWords = %d（插入换行不改变字数）",
		sumCountWords(mergeParts), countWords(text))
	t.Logf("合并后正文 %q（%d 字节，含 %d 个换行）", text, len(text), strings.Count(text, "\n"))
	requireInt(t, "合并后 countWords", countWords(text), sumCountWords(mergeParts))
	requireInt(t, "合并后首章字数（各章之和，不重算）", got[0].WordCount, 26)
	detail := bookByID(t, ctx, st, bookID)
	requireInt(t, "book word_count", detail.WordCount, before.WordCount)
	requireInt(t, "chapter_count", detail.ChapterCount, before.ChapterCount-2)
	after := mustMetrics(t, ctx, st)
	requireInt(t, "metrics 章节字数", int(after.ChapterWordSum), int(metricsBefore.ChapterWordSum))
}

// TestMergeChaptersSplitThenMergeWritesNewBlock 拆完再合并也走「拼接写新块」：
// 多插的换行改变了正文，原来那条「同块连续区间拉长 span」的零拷贝路径已删除，
// 旧块无人引用后应被整块回收，只留装合并正文的那一块。
func TestMergeChaptersSplitThenMergeWritesNewBlock(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "merge-contiguous",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "长章", Content: longChapter, WordCount: 20},
		})
	if _, err := st.SplitChapter(ctx, bookID, ids[0], []int{1}); err != nil {
		t.Fatalf("SplitChapter: %v", err)
	}
	split := chapterIDsOf(t, ctx, st, bookID)
	if len(split) != 2 {
		t.Fatalf("拆分后章数 = %d，期望 2", len(split))
	}
	metricsBefore := mustMetrics(t, ctx, st)
	pieces := []string{
		mustChapter(t, ctx, st, bookID, split[0]).Content,
		mustChapter(t, ctx, st, bookID, split[1]).Content,
	}

	if err := st.MergeChapters(ctx, bookID, split); err != nil {
		t.Fatalf("MergeChapters: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	if len(got) != 1 || got[0].ID != split[0] {
		t.Fatalf("合并结果 = %+v，期望只剩首章 %d", got, split[0])
	}
	requireString(t, "合并后标题", got[0].Title, "长章（1/2）")
	requireString(t, "合并后正文", mustChapter(t, ctx, st, bookID, split[0]).Content,
		strings.Join(pieces, "\n"))
	// 拆分时按 countWords 重算过分段字数，合并取存量之和 → 仍是 16
	requireInt(t, "合并后字数", got[0].WordCount, 16)
	if n := blockCount(t, ctx, st, bookID); n != 1 {
		t.Errorf("块数 = %d，期望 1（旧块无人引用应被回收）", n)
	}
	after := mustMetrics(t, ctx, st)
	requireInt(t, "metrics 章节字数", int(after.ChapterWordSum), int(metricsBefore.ChapterWordSum))
}

// TestMergeChaptersAcrossBlocksWritesNewBlock 两章分属不同块：只能新建一块装拼接后的正文。
// 合并结果超过 1 MB 分块目标也照写不误（一章独占一块，不为「超限」把正文截断）。
func TestMergeChaptersAcrossBlocksWritesNewBlock(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	// 每章约 660 KB：超过 1 MB 块目标的一半，两章必然各占一块
	first := strings.Repeat("甲段正文。", 44000) + "甲尾"
	second := strings.Repeat("乙段正文。", 44000) + "乙尾"
	bookID, ids := seedStructBook(t, ctx, st, "merge-blocks",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "前章", Content: first, WordCount: 100},
			{Idx: 1, VolumeIdx: 0, Title: "后章", Content: second, WordCount: 200},
		})
	if n := blockCount(t, ctx, st, bookID); n != 2 {
		t.Fatalf("初始块数 = %d，期望 2", n)
	}

	if err := st.MergeChapters(ctx, bookID, []int64{ids[1], ids[0]}); err != nil { // 故意给反序
		t.Fatalf("MergeChapters: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	if len(got) != 1 || got[0].ID != ids[0] {
		t.Fatalf("合并结果 = %+v，期望只剩首章 %d", got, ids[0])
	}
	requireString(t, "合并后标题", got[0].Title, "前章")
	// 拼接顺序由 idx 决定，不跟参数顺序走
	want := first + "\n" + second
	requireString(t, "合并后正文", mustChapter(t, ctx, st, bookID, ids[0]).Content, want)
	if len(want) <= BlockTargetBytes {
		t.Fatalf("样本正文 %d 字节，没超过单块目标 %d，测不到超长合并", len(want), BlockTargetBytes)
	}
	requireInt(t, "合并后字数", got[0].WordCount, 300)
	if n := blockCount(t, ctx, st, bookID); n != 1 {
		t.Errorf("块数 = %d，期望 1（旧块无人引用应被回收）", n)
	}
	detail := bookByID(t, ctx, st, bookID)
	requireInt(t, "chapter_count", detail.ChapterCount, 1)
	requireInt(t, "word_count", detail.WordCount, 300)
}

// TestMergeChaptersLeavesNeighboursIntact 合并中间两章：前后章仍指向旧块，
// 块不许回收，前后章的正文一个字节都不许动。
func TestMergeChaptersLeavesNeighboursIntact(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "merge-middle",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, fiveChapters())
	metricsBefore := mustMetrics(t, ctx, st)

	if err := st.MergeChapters(ctx, bookID, []int64{ids[1], ids[2]}); err != nil {
		t.Fatalf("MergeChapters: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	requireTitles(t, got, []string{"章甲", "章乙", "章丁", "章戊"})
	requireIdxSequence(t, got)
	requireChapterText(t, ctx, st, bookID, ids[0], "章甲正文。")
	requireChapterText(t, ctx, st, bookID, ids[1], "章乙正文。\n章丙正文。")
	requireChapterText(t, ctx, st, bookID, ids[3], "章丁正文。")
	// 旧块仍被前后章引用（不回收）+ 合并正文的新块
	if n := blockCount(t, ctx, st, bookID); n != 2 {
		t.Errorf("块数 = %d，期望 2（旧块不许回收 + 新块）", n)
	}
	after := mustMetrics(t, ctx, st)
	requireInt(t, "metrics 章节字数", int(after.ChapterWordSum), int(metricsBefore.ChapterWordSum))
}

// TestMergeChaptersKeepsCountsAndUpdatedAt 三处计数同事务维护，books.updated_at 刷新。
func TestMergeChaptersKeepsCountsAndUpdatedAt(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "merge-counts",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, fiveChapters())
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE books SET updated_at = ? WHERE id = ?", "2020-01-01T00:00:00.000Z", bookID); err != nil {
		t.Fatalf("重置 updated_at: %v", err)
	}
	before := bookByID(t, ctx, st, bookID)
	metricsBefore := mustMetrics(t, ctx, st)

	if err := st.MergeChapters(ctx, bookID, []int64{ids[0], ids[1], ids[2]}); err != nil {
		t.Fatalf("MergeChapters: %v", err)
	}
	detail := bookByID(t, ctx, st, bookID)
	requireInt(t, "chapter_count", detail.ChapterCount, before.ChapterCount-2)
	// 字数守恒：三章 12 字并成 12 字，书级总字数与库级字数之和都不该变
	requireInt(t, "word_count", detail.WordCount, before.WordCount)
	after := mustMetrics(t, ctx, st)
	requireInt(t, "metrics 章节字数", int(after.ChapterWordSum), int(metricsBefore.ChapterWordSum))
	if detail.UpdatedAt == before.UpdatedAt {
		t.Errorf("books.updated_at 没有刷新：%q", detail.UpdatedAt)
	}
	if got := chaptersOf(t, ctx, st, bookID); got[0].WordCount != 12 {
		t.Errorf("合并后首章字数 = %d，期望 12（各章之和）", got[0].WordCount)
	}
}

// TestMergeChaptersRollsBackWhenCompressFails 压缩失败时整体不动：
// 不许留下「首章已被改写、正文只写了半截」的半章状态。
func TestMergeChaptersRollsBackWhenCompressFails(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "merge-rollback",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, fiveChapters())
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE books SET updated_at = ? WHERE id = ?", "2020-01-01T00:00:00.000Z", bookID); err != nil {
		t.Fatalf("重置 updated_at: %v", err)
	}
	before := bookByID(t, ctx, st, bookID)
	metricsBefore := mustMetrics(t, ctx, st)
	textsBefore := make([]string, len(ids))
	for i, id := range ids {
		textsBefore[i] = mustChapter(t, ctx, st, bookID, id).Content
	}
	stubCompressFailure(t)

	if err := st.MergeChapters(ctx, bookID, []int64{ids[0], ids[1]}); err == nil {
		t.Fatal("压缩失败时 MergeChapters 应该报错")
	}
	got := chaptersOf(t, ctx, st, bookID)
	requireTitles(t, got, []string{"章甲", "章乙", "章丙", "章丁", "章戊"})
	requireIdxSequence(t, got)
	for i, id := range ids {
		requireString(t, fmt.Sprintf("回滚后第 %d 章正文", i), mustChapter(t, ctx, st, bookID, id).Content, textsBefore[i])
	}
	if n := blockCount(t, ctx, st, bookID); n != 1 {
		t.Errorf("块数 = %d，期望 1（失败的新块不该留下）", n)
	}
	after := bookByID(t, ctx, st, bookID)
	requireInt(t, "chapter_count", after.ChapterCount, before.ChapterCount)
	requireInt(t, "word_count", after.WordCount, before.WordCount)
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("回滚后 updated_at 变了：%v → %v", before.UpdatedAt, after.UpdatedAt)
	}
	metricsAfter := mustMetrics(t, ctx, st)
	requireInt(t, "metrics 章节字数", int(metricsAfter.ChapterWordSum), int(metricsBefore.ChapterWordSum))
	requireInt(t, "metrics 正文体积", int(metricsAfter.TextBytes), int(metricsBefore.TextBytes))
}

// TestMergeChaptersRejectsBadSelection 跨卷 / 不连续 / 单章 / 空 / 重复 / 跨书 一律拒绝。
func TestMergeChaptersRejectsBadSelection(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	vols := []model.ParsedVolume{{Idx: 0, Title: "第一卷"}, {Idx: 1, Title: "第二卷"}}
	bookID, ids := seedStructBook(t, ctx, st, "merge-reject", vols, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "甲", Content: "甲正文。", WordCount: 4},
		{Idx: 1, VolumeIdx: 0, Title: "乙", Content: "乙正文。", WordCount: 4},
		{Idx: 2, VolumeIdx: 0, Title: "丙", Content: "丙正文。", WordCount: 4},
		{Idx: 3, VolumeIdx: 1, Title: "丁", Content: "丁正文。", WordCount: 4},
	})
	_, otherIDs := seedStructBook(t, ctx, st, "merge-reject-other", vols, fiveChapters())

	cases := []struct {
		name string
		cids []int64
		want error
		why  string
	}{
		{"空列表", nil, ErrInvalidArgument, ""},
		{"重复 id", []int64{ids[0], ids[0]}, ErrInvalidArgument, ""},
		{"只有一章", []int64{ids[0]}, ErrNotMergeable, "少于 2 章"},
		{"跨卷", []int64{ids[2], ids[3]}, ErrNotMergeable, "跨卷"},
		{"不连续", []int64{ids[0], ids[2]}, ErrNotMergeable, "不连续"},
		{"别的书的章", []int64{ids[0], otherIDs[1]}, ErrNotFound, ""},
	}
	for _, tc := range cases {
		err := st.MergeChapters(ctx, bookID, tc.cids)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s：err = %v，期望 %v", tc.name, err, tc.want)
			continue
		}
		if tc.why != "" && !strings.Contains(err.Error(), tc.why) {
			t.Errorf("%s：错误信息缺少原因 %q：%v", tc.name, tc.why, err)
		}
	}
	if err := st.MergeChapters(ctx, 0, []int64{ids[0], ids[1]}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("书 id 非法 err = %v，期望 ErrInvalidArgument", err)
	}
	if got := chaptersOf(t, ctx, st, bookID); len(got) != 4 {
		t.Errorf("被拒的合并不该改目录，实际 %d 章", len(got))
	}
}

// mergePartsBook 把 mergeParts 包成一卷三章（WordCount 留 0：落库时按 countWords 现算）。
func mergePartsBook() []model.ParsedChapter {
	titles := []string{"第一章 手风琴", "第二章 夜雾", "第三章 站着"}
	out := make([]model.ParsedChapter, len(mergeParts))
	for i, text := range mergeParts {
		out[i] = model.ParsedChapter{Idx: i, VolumeIdx: 0, Title: titles[i], Content: text}
	}
	return out
}

// sumCountWords 求若干段正文的 countWords 之和。
func sumCountWords(parts []string) int {
	total := 0
	for _, p := range parts {
		total += countWords(p)
	}
	return total
}
