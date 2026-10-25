package importer

// 合集拆分：一本文件里装了两本书时，按章号重置点拆成多本分别入库。

import (
	"fmt"
	"strings"

	"norvels/internal/model"
	"norvels/internal/parser"
)

// minSegmentChapters 是合集拆分时每段的最少章节数：太短的分段更可能是章号误判。
const minSegmentChapters = 3

// splitBooks 按合集拆分点把一本书切成多本；不满足门控时原样返回单本。
//
// 门控（docs/specs/2026-10-25-import-p1.md §2.4）：
//   - parser.SplitSegments 返回 >1 段；
//   - 且每段章节数 ≥ minSegmentChapters，否则按单本导入并把「疑似合集未拆」写进 warnings。
//
// 拆分身份：第 1 段沿用原 source_path / file_sha256，第 N 段加 "#N" / ":N"，
// 重导同一文件能命中同一段；SplitOf 一律指向原 source_path。
func splitBooks(b *model.ParsedBook) []*model.ParsedBook {
	segs := parser.SplitSegments(b.Chapters)
	if len(segs) <= 1 {
		return []*model.ParsedBook{b}
	}
	if !eachSegmentBigEnough(segs) {
		b.Warnings = append(b.Warnings, fmt.Sprintf(
			"疑似多书合集（%d 段），但有段落不足 %d 章，已按单本导入", len(segs), minSegmentChapters))
		return []*model.ParsedBook{b}
	}
	out := make([]*model.ParsedBook, 0, len(segs))
	for i, seg := range segs {
		out = append(out, segmentBook(b, seg, i))
	}
	return out
}

// eachSegmentBigEnough 判断是否每段都够长。
func eachSegmentBigEnough(segs []parser.Segment) bool {
	for _, seg := range segs {
		if len(seg.Chapters) < minSegmentChapters {
			return false
		}
	}
	return true
}

// segmentBook 用原书与一段章节造出这一段的 ParsedBook（其余字段浅拷贝）。
func segmentBook(b *model.ParsedBook, seg parser.Segment, idx int) *model.ParsedBook {
	nb := *b
	nb.Chapters = seg.Chapters
	nb.WordCount = totalWords(seg.Chapters)
	nb.MultiBook = false // 拆完之后每本都是单本
	nb.SplitOf = b.SourcePath
	nb.Volumes = nil
	for i := range nb.Chapters {
		nb.Chapters[i].VolumeIdx = -1 // 段可能跨卷，卷分组在拆分后失效
	}
	nb.Warnings = append([]string(nil), b.Warnings...)
	if idx > 0 {
		nb.SourcePath = fmt.Sprintf("%s#%d", b.SourcePath, idx+1)
		nb.SHA256 = fmt.Sprintf("%s:%d", b.SHA256, idx+1)
		nb.Warnings = append(nb.Warnings, fmt.Sprintf("合集拆分第 %d 段（来源 %s）", idx+1, b.SourceName))
	}
	nb.Title = segmentTitleOf(b, seg, idx)
	if strings.TrimSpace(seg.Title) == "" && idx > 0 {
		nb.Warnings = append(nb.Warnings, fmt.Sprintf("第 %d 段未找到书名标记，已用「原标题（第 %d 段）」", idx+1, idx+1))
	}
	return &nb
}

// segmentTitleOf 决定这一段的书名：段内书名标记优先，其次沿用原标题，
// 第 2 段起取不到标记时用「原标题（第 N 段）」兜底。
func segmentTitleOf(b *model.ParsedBook, seg parser.Segment, idx int) string {
	if t := strings.TrimSpace(seg.Title); t != "" {
		return t
	}
	if idx == 0 {
		return b.Title
	}
	return fmt.Sprintf("%s（第 %d 段）", b.Title, idx+1)
}
