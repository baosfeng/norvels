package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// TestRenumberedTitleRules 锁死标题改写规则：剥掉旧章号 → 拼「第 N 章」→ 余下为空就只留章号。
func TestRenumberedTitleRules(t *testing.T) {
	cases := []struct {
		idx   int
		title string
		want  string
	}{
		{0, "第十二章 风起", "第 1 章 风起"},
		{11, "第 12 章", "第 12 章"},
		{2, "第12回 夜行", "第 3 章 夜行"},
		{2, "第 3 节", "第 3 章"},
		{3, "第十二章、归途", "第 4 章 归途"},
		{4, "第 十二 章 风", "第 5 章 风"},
		{5, "楔子", "第 6 章 楔子"},
		{6, "第一次见面", "第 7 章 第一次见面"},
		{7, "", "第 8 章"},
		{8, "  第十二章   ", "第 9 章"},
	}
	for _, tc := range cases {
		if got := renumberedTitle(tc.idx, tc.title); got != tc.want {
			t.Errorf("renumberedTitle(%d, %q) = %q，期望 %q", tc.idx, tc.title, got, tc.want)
		}
	}
}

// TestRenumberChaptersWholeBook 全书重编号：标题对齐 idx+1，正文与 idx 一个都不许动。
func TestRenumberChaptersWholeBook(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "renumber-all",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "第十二章 风起", Content: "第一章正文。", WordCount: 6},
			{Idx: 1, VolumeIdx: 0, Title: "楔子", Content: "第二段正文。", WordCount: 5},
			{Idx: 2, VolumeIdx: 0, Title: "第 3 节 归途", Content: "第三段正文。", WordCount: 5},
		})
	before := chaptersOf(t, ctx, st, bookID)

	if err := st.RenumberChapters(ctx, bookID, RenumberScope{}); err != nil {
		t.Fatalf("RenumberChapters: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	requireTitles(t, got, []string{"第 1 章 风起", "第 2 章 楔子", "第 3 章 归途"})
	for i := range got {
		if got[i].Idx != before[i].Idx {
			t.Errorf("第 %d 章 idx = %d，期望不变 %d", i, got[i].Idx, before[i].Idx)
		}
		if got[i].WordCount != before[i].WordCount {
			t.Errorf("第 %d 章字数 = %d，期望不变 %d", i, got[i].WordCount, before[i].WordCount)
		}
	}
	requireChapterText(t, ctx, st, bookID, ids[2], "第三段正文。")
	if n := blockCount(t, ctx, st, bookID); n != 1 {
		t.Errorf("块数 = %d，期望 1（重编号不碰正文块）", n)
	}
	// 幂等：再跑一次标题不变
	if err := st.RenumberChapters(ctx, bookID, RenumberScope{}); err != nil {
		t.Fatalf("RenumberChapters 二次: %v", err)
	}
	requireTitles(t, chaptersOf(t, ctx, st, bookID), []string{"第 1 章 风起", "第 2 章 楔子", "第 3 章 归途"})
}

// TestRenumberChaptersSingleVolume 单卷范围：只改该卷标题，章号仍是全书序号。
func TestRenumberChaptersSingleVolume(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, _ := seedStructBook(t, ctx, st, "renumber-volume",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}, {Idx: 1, Title: "第二卷"}},
		[]model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "第一章 起", Content: "起。", WordCount: 1},
			{Idx: 1, VolumeIdx: 0, Title: "第二章 承", Content: "承。", WordCount: 1},
			{Idx: 2, VolumeIdx: 1, Title: "第一章 转", Content: "转。", WordCount: 1},
			{Idx: 3, VolumeIdx: 1, Title: "第二章 合", Content: "合。", WordCount: 1},
		})
	vols := volumeIDsOf(t, ctx, st, bookID)
	if len(vols) != 2 {
		t.Fatalf("卷数 = %d，期望 2", len(vols))
	}

	if err := st.RenumberChapters(ctx, bookID, RenumberScope{VolumeID: vols[1]}); err != nil {
		t.Fatalf("RenumberChapters: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	requireTitles(t, got, []string{"第一章 起", "第二章 承", "第 3 章 转", "第 4 章 合"})
	requireIdxSequence(t, got)
	if got[2].VolumeID != vols[1] {
		t.Errorf("第 3 章的 volume_id = %d，期望仍在卷 %d", got[2].VolumeID, vols[1])
	}
}

// TestRenumberChaptersRejects 别的书的卷 / 非法入参：拒绝且不写任何标题。
func TestRenumberChaptersRejects(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	chapters := []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "第一章 甲", Content: "甲。", WordCount: 1},
	}
	bookID, _ := seedStructBook(t, ctx, st, "renumber-reject",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, chapters)
	otherBook, _ := seedStructBook(t, ctx, st, "renumber-reject-other",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, chapters)
	otherVol := volumeIDsOf(t, ctx, st, otherBook)[0]

	if err := st.RenumberChapters(ctx, bookID, RenumberScope{VolumeID: otherVol}); !errors.Is(err, ErrNotFound) {
		t.Errorf("别的书的卷 err = %v，期望 ErrNotFound", err)
	}
	if err := st.RenumberChapters(ctx, 0, RenumberScope{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("书 id 非法 err = %v，期望 ErrInvalidArgument", err)
	}
	if err := st.RenumberChapters(ctx, bookID, RenumberScope{VolumeID: -1}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("卷 id 为负 err = %v，期望 ErrInvalidArgument", err)
	}
	if got := chaptersOf(t, ctx, st, bookID); !strings.HasPrefix(got[0].Title, "第一章") {
		t.Errorf("被拒的重编号改动了标题：%q", got[0].Title)
	}
}
