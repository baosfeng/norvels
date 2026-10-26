package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// TestSaveBookDedupesVolumeTitles 同一本书里出现两个同名卷时，第二卷自动改名而不是整本失败。
func TestSaveBookDedupesVolumeTitles(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := &model.ParsedBook{
		Title:      "同名卷示例",
		Author:     "示例作者",
		SourcePath: "/books/dup-volumes.txt",
		SourceName: "dup-volumes.txt",
		FileSize:   128,
		SHA256:     strings.Repeat("b", 64),
		Volumes: []model.ParsedVolume{
			{Idx: 0, Title: "第一卷"},
			{Idx: 1, Title: "第一卷"},
			{Idx: 2, Title: "第二卷"},
		},
		Chapters: []model.ParsedChapter{
			{Idx: 0, Title: "第一章", VolumeIdx: 0, Content: "正文一"},
			{Idx: 1, Title: "第二章", VolumeIdx: 1, Content: "正文二"},
			{Idx: 2, Title: "第三章", VolumeIdx: 2, Content: "正文三"},
		},
	}
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	var titles []string
	for _, v := range detail.Volumes {
		if v.ID != 0 {
			titles = append(titles, v.Title)
		}
	}
	want := []string{"第一卷", "第一卷 (2)", "第二卷"}
	if len(titles) != len(want) {
		t.Fatalf("volume titles = %v, want %v", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("volume titles = %v, want %v", titles, want)
		}
	}
}
