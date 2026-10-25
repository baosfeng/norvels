package parser

import (
	"strconv"
	"strings"
	"testing"
)

// 合成合集语料：书名/作者全部虚构（隐私红线：测试里不出现真实书名）。

// writeTestChapters 写 n 章「第N章 标签段」，每章两段正文。
func writeTestChapters(b *strings.Builder, n int, tag string) {
	for i := 1; i <= n; i++ {
		b.WriteString("第" + strconv.Itoa(i) + "章 " + tag + "段\n")
		b.WriteString(tag + "段的第一段正文，用来验证章号重置后的分段是否正确。\n")
		b.WriteString(tag + "段的第二段正文，内容与真实作品无关。\n\n")
	}
}

// collectionText 合成两书合集：第一本 20 章，第二本 12 章（章号从 1 重新开始）。
// withMark 为 true 时，第二本书的书名标记紧跟在其第一章之前。
func collectionText(withMark bool) string {
	var b strings.Builder
	b.WriteString("书名：示例合甲\n作者：某甲\n简介：合成语料，与真实作品无关。\n\n")
	writeTestChapters(&b, 20, "甲")
	if withMark {
		b.WriteString("书名：示例合乙\n作者：某乙\n\n")
	}
	writeTestChapters(&b, 12, "乙")
	return b.String()
}

// TestSplitSegmentsByRestart 覆盖两段章号重置：切成 2 段、段标题取书名标记。
func TestSplitSegmentsByRestart(t *testing.T) {
	book, err := New(DefaultOptions()).ParseBytes("合集.txt", []byte(collectionText(true)))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) != 32 {
		t.Fatalf("章数 = %d，期望 32（20 + 12）", len(book.Chapters))
	}
	segs := SplitSegments(book.Chapters)
	if len(segs) != 2 {
		t.Fatalf("段数 = %d，期望 2", len(segs))
	}
	if len(segs[0].Chapters) != 20 || len(segs[1].Chapters) != 12 {
		t.Errorf("各段章数 = %d / %d，期望 20 / 12", len(segs[0].Chapters), len(segs[1].Chapters))
	}
	if segs[1].Title != "示例合乙" {
		t.Errorf("第 2 段标题 = %q，期望取自书名标记「示例合乙」", segs[1].Title)
	}
	if segs[0].Title != "" {
		t.Errorf("第 1 段标题 = %q，期望空串（书名标记在正文头部，不在章节里）", segs[0].Title)
	}
	if got := segs[1].Chapters[0].Title; !strings.HasPrefix(got, "第1章") {
		t.Errorf("第 2 段首章标题 = %q，期望第 1 章", got)
	}
}

// TestSplitSegmentsSingle 覆盖单段文本：返回 1 段，章节原样保留。
func TestSplitSegmentsSingle(t *testing.T) {
	var b strings.Builder
	writeTestChapters(&b, 5, "单")
	book, err := New(DefaultOptions()).ParseBytes("单本.txt", []byte(b.String()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	segs := SplitSegments(book.Chapters)
	if len(segs) != 1 {
		t.Fatalf("段数 = %d，期望 1", len(segs))
	}
	if len(segs[0].Chapters) != len(book.Chapters) {
		t.Errorf("段内章数 = %d，期望 %d", len(segs[0].Chapters), len(book.Chapters))
	}
	if segs[0].Title != "" {
		t.Errorf("段标题 = %q，期望空串（无书名标记）", segs[0].Title)
	}
}

// TestSplitSegmentsTitleMissing 覆盖段标题缺失：返回空串，不猜书名。
func TestSplitSegmentsTitleMissing(t *testing.T) {
	book, err := New(DefaultOptions()).ParseBytes("合集.txt", []byte(collectionText(false)))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	segs := SplitSegments(book.Chapters)
	if len(segs) != 2 {
		t.Fatalf("段数 = %d，期望 2", len(segs))
	}
	if segs[1].Title != "" {
		t.Errorf("第 2 段标题 = %q，期望空串", segs[1].Title)
	}
}

// TestSplitSegmentsEmpty 覆盖空入参：返回 nil。
func TestSplitSegmentsEmpty(t *testing.T) {
	if got := SplitSegments(nil); got != nil {
		t.Errorf("空入参 = %v，期望 nil", got)
	}
}

// TestSplitSegmentsKeepsInputOrder 覆盖段内顺序与入参一致、且不与入参共享底层数组。
func TestSplitSegmentsKeepsInputOrder(t *testing.T) {
	var b strings.Builder
	writeTestChapters(&b, 12, "甲")
	writeTestChapters(&b, 12, "乙")
	book, err := New(DefaultOptions()).ParseBytes("合集.txt", []byte(b.String()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	segs := SplitSegments(book.Chapters)
	if len(segs) != 2 {
		t.Fatalf("段数 = %d，期望 2", len(segs))
	}
	for i := range segs[1].Chapters {
		if segs[1].Chapters[i].Title != book.Chapters[12+i].Title {
			t.Errorf("第 2 段第 %d 章标题 = %q，期望与入参同序 %q",
				i+1, segs[1].Chapters[i].Title, book.Chapters[12+i].Title)
		}
	}
	segs[1].Chapters[0].Title = "改段内标题"
	if book.Chapters[12].Title == "改段内标题" {
		t.Error("段内章节与入参共享底层数组，改段会污染入参")
	}
}
