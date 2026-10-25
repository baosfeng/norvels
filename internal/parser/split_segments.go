package parser

import (
	"strings"

	"norvels/internal/model"
)

// 合集拆分口径与切章时统计 restarts 的代码保持一致（splitting 侧在 split_build.go）。
const (
	multiBookResetFrom = 10 // 上一章章号超过它
	multiBookResetTo   = 2  // 本章章号不超过它
	minSegChapters     = 3  // 已经切出的章数超过它
)

// Segment 是合集里的一段（一本书）的章节序列。
type Segment struct {
	// Title 是段内书名标记（「书名：X」或「《X》」行）；取不到为空串，由调用方兜底。
	Title string
	// Chapters 是这一段包含的章节，顺序与入参一致（切片为浅拷贝，不与入参共享底层数组）。
	Chapters []model.ParsedChapter
}

// SplitSegments 按章号重置点把章节序列切成多段。
//
// 未定义行为在此写死：
//   - 空入参 / 全是不可解析标题 → nil；
//   - 没有重置点 → 单段（Title 可能是空串，也可能取到段首的书名标记）；
//   - 只有章号可解析的章标题（第 N 章 / N 章 / Chapter N）参与判定，
//     番外、卷标题这类无章号章节原样跟随当前段；
//   - 段标题优先取书名标记：先看上一段末章尾部（合集里书名标记紧挨在下一本书
//     第一章之前，切章时会被算进上一章），再看本段首章正文开头与头部标题。
func SplitSegments(chapters []model.ParsedChapter) []Segment {
	if len(chapters) == 0 {
		return nil
	}
	starts := segmentStarts(chapters)
	segs := make([]Segment, 0, len(starts))
	for i, start := range starts {
		end := len(chapters)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		segs = append(segs, Segment{
			Title:    segmentTitle(chapters, start),
			Chapters: append([]model.ParsedChapter(nil), chapters[start:end]...),
		})
	}
	return segs
}

// segmentStarts 返回每一段的起始下标；首段恒为 0。
func segmentStarts(chapters []model.ParsedChapter) []int {
	starts := []int{0}
	last, hasLast := 0, false
	for i, c := range chapters {
		num, ok := chapterNum(c.Title)
		if !ok {
			continue
		}
		if hasLast && last > multiBookResetFrom && num <= multiBookResetTo && i > minSegChapters {
			starts = append(starts, i)
		}
		last, hasLast = num, true
	}
	return starts
}

// chapterNum 取章标题里的章号；不是带号章标题时 ok=false。
func chapterNum(title string) (int, bool) {
	h, ok := parseHeading(title)
	if !ok || h.kind != headChapter || !h.hasNum {
		return 0, false
	}
	return h.num, true
}

// segmentTitle 取段内书名标记；取不到返回空串。
func segmentTitle(chapters []model.ParsedChapter, start int) string {
	if start > 0 {
		if t := markInTail(chapters[start-1].Content, 12); t != "" {
			return t
		}
	}
	if t := markInHead(chapters[start].Content, 20); t != "" {
		return t
	}
	for i := start; i < len(chapters) && i < start+2; i++ {
		if t := markFromLine(chapters[i].Title); t != "" {
			return t
		}
	}
	return ""
}

// markInTail 从末行往前找书名标记：越靠近段边界越可能是下一本书的书名。
func markInTail(content string, n int) string {
	lines := lastLines(content, n)
	for i := len(lines) - 1; i >= 0; i-- {
		if t := markFromLine(lines[i]); t != "" {
			return t
		}
	}
	return ""
}

// markInHead 从首行往后找书名标记。
func markInHead(content string, n int) string {
	for _, line := range firstLines(content, n) {
		if t := markFromLine(line); t != "" {
			return t
		}
	}
	return ""
}

// markFromLine 解析单行书名标记：「书名：X」直接取，或以《X》单独成行（≤60 字）。
func markFromLine(line string) string {
	if m := reBodyTitle.FindStringSubmatch(line); m != nil {
		if t := cleanTitleText(m[1]); t != "" {
			return t
		}
	}
	if m := reBookMark.FindStringSubmatch(line); m != nil && runeLen(line) <= 60 {
		if t := cleanTitleText(m[1]); t != "" {
			return t
		}
	}
	return ""
}

// trimLines 切行去空白并丢空行。
func trimLines(s string) []string {
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, ln := range lines {
		if t := strings.TrimSpace(ln); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// lastLines 取末尾 n 个非空行。
func lastLines(s string, n int) []string {
	lines := trimLines(s)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// firstLines 取开头 n 个非空行。
func firstLines(s string, n int) []string {
	lines := trimLines(s)
	if len(lines) > n {
		lines = lines[:n]
	}
	return lines
}
