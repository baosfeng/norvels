package parser

import (
	"sort"
	"strings"

	"norvels/internal/model"
)

// minCandidateGap 是两个候选标题之间的最小字节间距（更近的视为同一行）。
const minCandidateGap = 8

// candidate 是一个候选标题在全文中的位置。
type candidate struct {
	start     int // 标题行起始字节偏移
	bodyStart int // 正文起始字节偏移
	h         heading
}

// splitResult 是章节切分的中间结果。
type splitResult struct {
	volumes  []model.ParsedVolume
	chapters []model.ParsedChapter
	preamble string
	warnings []string
	restarts int // 章号重置次数（用于判断合集）
}

// scanCandidates 逐行扫描候选标题行。
func scanCandidates(text string) []candidate {
	var out []candidate
	lineStart := 0
	for i := 0; i <= len(text); i++ {
		if i != len(text) && text[i] != '\n' {
			continue
		}
		lineEnd := i
		lineLen := lineEnd - lineStart
		if lineLen >= minHeadingBytes {
			line := text[lineStart:lineEnd]
			lead := headingLeadOK(line)
			handled := false
			if lead && lineLen <= maxHeadingBytes {
				if h, ok := parseHeading(line); ok {
					out = append(out, candidate{start: lineStart, bodyStart: min(lineEnd+1, len(text)), h: h})
					handled = true
				}
			}
			if !handled && !lead && lineLen <= 120 && strings.Contains(line, "第") {
				// 「书名 第001章 标题」这类带前缀的标题行
				if h, ok := parsePrefixedHeading(line); ok {
					out = append(out, candidate{start: lineStart, bodyStart: min(lineEnd+1, len(text)), h: h})
					handled = true
				}
			}
			if !handled && lead && (lineLen > maxHeadingBytes || runeLen(line) > 40) {
				// 标题与正文挤在同一行（常见于用大量空格/NBSP 排版的文件）
				pos := lineStart
				for pos < lineEnd {
					h, off, ok := parseInlineHeading(text[pos:lineEnd])
					if !ok || off <= 0 {
						break
					}
					out = append(out, candidate{start: pos, bodyStart: min(pos+off, lineEnd), h: h})
					pos += off
				}
			}
		}
		lineStart = i + 1
	}
	return out
}

// scanEmbedded 在超长行内部查找章节标记（用于「整篇几乎没有换行」的文件）。
func scanEmbedded(text string) []candidate {
	var out []candidate
	lineStart := 0
	for i := 0; i <= len(text); i++ {
		if i != len(text) && text[i] != '\n' {
			continue
		}
		lineEnd := i
		if lineEnd-lineStart > 400 {
			line := text[lineStart:lineEnd]
			pos := 0
			for {
				rel := strings.Index(line[pos:], "第")
				if rel < 0 {
					break
				}
				abs := lineStart + pos + rel
				h, ok := parseHeading(line[pos+rel : min(pos+rel+maxHeadingBytes, len(line))])
				if ok && h.kind == headChapter && h.title != "" {
					bodyStart := abs + len(h.marker)
					bodyStart = min(bodyStart, len(text))
					out = append(out, candidate{start: abs, bodyStart: bodyStart, h: h})
				}
				pos += rel + len("第")
			}
		}
		lineStart = i + 1
	}
	return out
}

func mergeCandidates(a, b []candidate) []candidate {
	if len(b) == 0 {
		return a
	}
	m := make([]candidate, 0, len(a)+len(b))
	m = append(m, a...)
	m = append(m, b...)
	sort.SliceStable(m, func(i, j int) bool { return m[i].start < m[j].start })
	// 去重：位置相同或过近。
	// 初值用 -minCandidateGap 而不是 -1，否则位于全文开头（offset 0）的标题
	// 会因为 0-(-1)=1 < 8 被误删，导致以「第一章」开头的书丢掉首章标题。
	out := m[:0]
	last := -minCandidateGap
	for _, c := range m {
		if c.start-last < minCandidateGap {
			continue
		}
		out = append(out, c)
		last = c.start
	}
	return out
}

// validateCandidates 的统计结果，用于「候选数守卫」判断与排障日志。
type validateStats struct {
	Before int // 进入校验的候选数
	After  int // 校验后保留的候选数
}

// validateCandidates 过滤误判，保证章号大体单调递增。
func validateCandidates(cands []candidate) []candidate {
	out, _ := validateCandidatesStats(cands)
	return out
}

// validateCandidatesStats 与 validateCandidates 同义，额外返回保留统计。
func validateCandidatesStats(cands []candidate) ([]candidate, validateStats) {
	st := validateStats{Before: len(cands)}
	out := validateCandidatesInner(cands)
	st.After = len(out)
	return out, st
}

// validateCandidatesInner 是实际的校验逻辑。
func validateCandidatesInner(cands []candidate) []candidate {
	strongCnt := 0
	for _, c := range cands {
		if c.h.kind == headVolume || c.h.strong {
			strongCnt++
		}
	}
	out := make([]candidate, 0, len(cands))
	if strongCnt >= 3 {
		// 强特征也可能来自正文里的「见第六章」这类引用：要求章号大体递增，
		// 允许从 1~5 重新开始（多卷/合集每卷重新编号）。
		prev := 0
		for _, c := range cands {
			if c.h.kind == headVolume {
				out = append(out, c)
				continue
			}
			if !c.h.strong {
				continue
			}
			if !c.h.hasNum {
				out = append(out, c) // 序章/番外/后记等无编号章节
				continue
			}
			n := c.h.num
			if prev == 0 || n > prev || n <= 5 {
				out = append(out, c)
				prev = n
			}
		}
		return out
	}
	// 全是弱特征：必须靠数字单调性筛选
	prev := 0
	for _, c := range cands {
		if c.h.kind == headVolume {
			out = append(out, c)
			continue
		}
		if !c.h.hasNum || c.h.num <= 0 {
			continue
		}
		if prev == 0 {
			out = append(out, c)
			prev = c.h.num
			continue
		}
		if c.h.num > prev && c.h.num <= prev+300 {
			out = append(out, c)
			prev = c.h.num
		}
	}
	return out
}
