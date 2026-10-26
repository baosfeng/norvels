package importer

import (
	"testing"

	"norvels/internal/model"
)

// twoPlusSegmentBook 造一本「每段 12 章、段内章号从 1 重来」的合成书。
// withMark 为 true 时在第 2 段起首章正文前放书名标记。
func twoPlusSegmentBook(segments int, withMark bool) *model.ParsedBook {
	chapters := make([]model.ParsedChapter, 0, segments*12)
	for seg := 0; seg < segments; seg++ {
		for i := 1; i <= 12; i++ {
			content := "这是合成正文，与真实作品无关。\n"
			if withMark && seg > 0 && i == 1 {
				content = "书名：合成续卷" + itoa(seg) + "\n作者：某续\n\n" + content
			}
			chapters = append(chapters, model.ParsedChapter{
				Idx:       len(chapters),
				VolumeIdx: -1,
				Title:     "第" + itoa(i) + "章 段" + itoa(seg),
				Content:   content,
			})
		}
	}
	return &model.ParsedBook{Title: "合成多段", Chapters: chapters}
}

// TestSplitBooksRejectsTooManySegments 覆盖段数超过上限：按单本导入，不拆成几十本。
func TestSplitBooksRejectsTooManySegments(t *testing.T) {
	book := twoPlusSegmentBook(maxSplitSegments+1, true)
	if got := splitBooks(book); len(got) != 1 {
		t.Fatalf("段数 = %d，期望 1（超过 %d 段上限不拆）", len(got), maxSplitSegments)
	}
}

// TestSplitBooksRejectsUntitledSegments 覆盖第 2 段起没有书名标记：按单本导入。
func TestSplitBooksRejectsUntitledSegments(t *testing.T) {
	book := twoPlusSegmentBook(3, false)
	if got := splitBooks(book); len(got) != 1 {
		t.Fatalf("段数 = %d，期望 1（缺少书名标记不拆）", len(got))
	}
}

// TestSplitBooksKeepsTitledSegments 覆盖带书名标记的多段合集：正常拆分。
func TestSplitBooksKeepsTitledSegments(t *testing.T) {
	book := twoPlusSegmentBook(3, true)
	got := splitBooks(book)
	if len(got) != 3 {
		t.Fatalf("段数 = %d，期望 3", len(got))
	}
	if got[1].Title != "合成续卷1" || got[2].Title != "合成续卷2" {
		t.Fatalf("段标题 = %q / %q", got[1].Title, got[2].Title)
	}
}
