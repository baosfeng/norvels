package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"

	"norvels/internal/model"
)

// seedStructBook 写一本指定卷/章节的书，返回 bookID 与目录顺序的章节 id。
//
// 每个测试给独立 name：source_path 与 file_sha256 都有 UNIQUE 约束，
// 同一个临时库里写第二本书时必须换路径与哈希，否则第二次 SaveBook 变成 Skipped。
func seedStructBook(t *testing.T, ctx context.Context, st *Store, name string,
	vols []model.ParsedVolume, chapters []model.ParsedChapter) (int64, []int64) {
	t.Helper()
	b := &model.ParsedBook{
		Title:      "结构操作样本",
		Author:     "样本作者",
		SourcePath: "/books/" + name + ".txt",
		SourceName: name + ".txt",
		Encoding:   "utf-8",
		FileSize:   4096,
		SHA256:     fmt.Sprintf("%x", sha256.Sum256([]byte(name))),
		Volumes:    vols,
		Chapters:   chapters,
	}
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	return res.BookID, chapterIDsOf(t, ctx, st, res.BookID)
}

// chapterIDsOf 读回目录顺序的章节 id。
func chapterIDsOf(t *testing.T, ctx context.Context, st *Store, bookID int64) []int64 {
	t.Helper()
	items := chaptersOf(t, ctx, st, bookID)
	ids := make([]int64, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

// volumeIDsOf 读回按卷序号排列的卷 id（重编号的范围参数要用它）。
func volumeIDsOf(t *testing.T, ctx context.Context, st *Store, bookID int64) []int64 {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx,
		"SELECT id FROM volumes WHERE book_id = ? ORDER BY idx", bookID)
	if err != nil {
		t.Fatalf("list volumes: %v", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan volume: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list volumes rows: %v", err)
	}
	return ids
}

// fiveChapters 是「同一块里的 5 章」，用于目录重排类断言。
func fiveChapters() []model.ParsedChapter {
	names := []string{"章甲", "章乙", "章丙", "章丁", "章戊"}
	out := make([]model.ParsedChapter, len(names))
	for i, name := range names {
		out[i] = model.ParsedChapter{
			Idx: i, VolumeIdx: 0, Title: name, Content: name + "正文。", WordCount: 4,
		}
	}
	return out
}

// TestDeleteChaptersShiftsIdxAndKeepsTexts 一次删两章：序号补成 0 起连续，剩下章节正文不乱。
func TestDeleteChaptersShiftsIdxAndKeepsTexts(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "delete-batch",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, fiveChapters())
	before := bookByID(t, ctx, st, bookID)
	metricsBefore := mustMetrics(t, ctx, st)

	if err := st.DeleteChapters(ctx, bookID, []int64{ids[1], ids[3]}); err != nil {
		t.Fatalf("DeleteChapters: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	requireTitles(t, got, []string{"章甲", "章丙", "章戊"})
	requireIdxSequence(t, got)
	for i, want := range []struct {
		id   int64
		text string
	}{{ids[0], "章甲正文。"}, {ids[2], "章丙正文。"}, {ids[4], "章戊正文。"}} {
		if got[i].ID != want.id {
			t.Errorf("第 %d 章 id = %d，期望 %d", i, got[i].ID, want.id)
		}
		requireChapterText(t, ctx, st, bookID, want.id, want.text)
	}
	// 计数守恒：删 2 章 8 字，书级与库级同步
	detail := bookByID(t, ctx, st, bookID)
	requireInt(t, "chapter_count", detail.ChapterCount, before.ChapterCount-2)
	requireInt(t, "word_count", detail.WordCount, before.WordCount-8)
	after := mustMetrics(t, ctx, st)
	requireInt(t, "metrics 章节字数", int(after.ChapterWordSum), int(metricsBefore.ChapterWordSum-8))
	// 这 5 章同在一块，删掉两章后块仍被引用 → 不许回收
	if n := blockCount(t, ctx, st, bookID); n != 1 {
		t.Errorf("块数 = %d，期望 1（仍被剩余章节引用）", n)
	}
}

// TestDeleteChaptersReapsOrphanBlock 删掉独占一块的章：整块回收，text_bytes 随之下降。
func TestDeleteChaptersReapsOrphanBlock(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	// 两章各 ~660 KB（单章 > 1 MB 目标的一半，两章必然分到不同块），第三个小章并入第二块
	big := strings.Repeat("甲乙丙丁。", 44000) // 每章约 660 KB：两章必然分到不同块
	const bigWords = 176000
	bookID, ids := seedStructBook(t, ctx, st, "delete-reap",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "大章一", Content: big, WordCount: bigWords},
			{Idx: 1, VolumeIdx: 0, Title: "大章二", Content: big, WordCount: bigWords},
			{Idx: 2, VolumeIdx: 0, Title: "小章", Content: "小章正文。", WordCount: 4},
		})
	if n := blockCount(t, ctx, st, bookID); n != 2 {
		t.Fatalf("初始块数 = %d，期望 2（本章测试依赖大章各占一块）", n)
	}
	before := mustMetrics(t, ctx, st)

	if err := st.DeleteChapters(ctx, bookID, []int64{ids[0]}); err != nil {
		t.Fatalf("DeleteChapters: %v", err)
	}
	if n := blockCount(t, ctx, st, bookID); n != 1 {
		t.Errorf("删掉独占块的章后块数 = %d，期望 1", n)
	}
	after := mustMetrics(t, ctx, st)
	if after.TextBytes >= before.TextBytes {
		t.Errorf("正文体积没有降：%d → %d", before.TextBytes, after.TextBytes)
	}
	if after.ChapterWordSum != before.ChapterWordSum-bigWords {
		t.Errorf("章节字数 = %d，期望 %d", after.ChapterWordSum, before.ChapterWordSum-bigWords)
	}
	got := chaptersOf(t, ctx, st, bookID)
	requireTitles(t, got, []string{"大章二", "小章"})
	requireIdxSequence(t, got)
}

// TestDeleteChaptersRejectsBadInput 空列表 / 重复 / 不存在 / 跨书 一律拒绝且不留改动。
func TestDeleteChaptersRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "delete-reject",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, fiveChapters())
	otherBook, otherIDs := seedStructBook(t, ctx, st, "delete-reject-other",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, fiveChapters())

	cases := []struct {
		name   string
		bookID int64
		cids   []int64
		want   error
	}{
		{"空列表", bookID, nil, ErrInvalidArgument},
		{"非正数", bookID, []int64{0}, ErrInvalidArgument},
		{"重复 id", bookID, []int64{ids[0], ids[0]}, ErrInvalidArgument},
		{"不存在的章", bookID, []int64{ids[0], 999999}, ErrNotFound},
		{"别的书的章", bookID, []int64{ids[0], otherIDs[0]}, ErrNotFound},
		{"书 id 非法", 0, []int64{ids[0]}, ErrInvalidArgument},
	}
	for _, tc := range cases {
		if err := st.DeleteChapters(ctx, tc.bookID, tc.cids); !errors.Is(err, tc.want) {
			t.Errorf("%s：err = %v，期望 %v", tc.name, err, tc.want)
		}
	}
	if n := len(chaptersOf(t, ctx, st, bookID)); n != 5 {
		t.Errorf("被拒的批量删除不该改目录，实际 %d 章", n)
	}
	if n := len(chaptersOf(t, ctx, st, otherBook)); n != 5 {
		t.Errorf("另一本书的目录被动过：%d 章", n)
	}
}
