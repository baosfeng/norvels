package parser

import (
	"regexp"
	"strings"
	"unicode"
)

// 正文头部元信息。
var (
	reBodyTitle        = regexp.MustCompile("^(?:书\\s*名|作品名|书名|标\\s*题)\\s*[:：]\\s*(.+)$")
	reBodyAuthor       = regexp.MustCompile("^(?:作\\s*者|著\\s*者|作\\s*家|编\\s*者)\\s*[:：]?\\s*(.+)$")
	reBodyAuthorSuffix = regexp.MustCompile("^(.{1,20})\\s*著$")
	reIntroMark        = regexp.MustCompile("^(?:【)?\\s*(?:内容简介|作品简介|内容介绍|简\\s*介|文\\s*案|简\\s*述)\\s*(?:】)?\\s*[:：]?\\s*(.*)$")
)

// metaFromPreamble 从正文头部（第一个章节标题之前的内容）抽取书名、作者与简介。
func metaFromPreamble(pre string) (title, author, intro string) {
	if strings.TrimSpace(pre) == "" {
		return "", "", ""
	}
	lines := make([]string, 0, 96)
	for _, ln := range strings.Split(pre, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || isNoiseLineFull(t) {
			continue
		}
		lines = append(lines, t)
		if len(lines) >= 96 {
			break
		}
	}
	introStart := -1
	for i, ln := range lines {
		if title == "" {
			if m := reBodyTitle.FindStringSubmatch(ln); m != nil {
				title = cleanTitleText(m[1])
			} else if m := reBookMark.FindStringSubmatch(ln); m != nil && runeLen(ln) <= 60 {
				title = cleanTitleText(m[1])
			}
		}
		if author == "" {
			if m := reBodyAuthor.FindStringSubmatch(ln); m != nil {
				author = cleanAuthorName(m[1])
			} else if m := reBodyAuthorSuffix.FindStringSubmatch(ln); m != nil {
				author = cleanAuthorName(m[1])
			}
		}
		if introStart < 0 {
			if m := reIntroMark.FindStringSubmatch(ln); m != nil {
				introStart = i
				intro = strings.TrimSpace(m[1])
			}
		}
		if i > 40 && title != "" && author != "" && introStart >= 0 {
			break
		}
	}
	if introStart >= 0 {
		var b strings.Builder
		b.WriteString(intro)
		for j := introStart + 1; j < len(lines) && j < introStart+12; j++ {
			if !isLikelyIntroLine(lines[j]) {
				break
			}
			if b.Len() > 0 {
				b.WriteString(" ")
			}
			b.WriteString(lines[j])
			if runeLen(b.String()) >= 500 {
				break
			}
		}
		intro = strings.TrimSpace(b.String())
	} else {
		for _, ln := range lines {
			if runeLen(ln) >= 40 && !isLikelyTitleLine(ln) {
				intro = ln
				break
			}
		}
	}
	if runeLen(intro) > 500 {
		intro = truncateRunes(intro, 500)
	}
	return title, author, intro
}

// isLikelyIntroLine 判断一行是否适合继续并入简介。
func isLikelyIntroLine(s string) bool {
	if s == "" || isNoiseLineFull(s) || isLikelyTitleLine(s) {
		return false
	}
	return runeLen(s) >= 8
}

// isLikelyTitleLine 判断一行是否像「作者：」「第一章」这类头部字段，不应并入简介。
func isLikelyTitleLine(s string) bool {
	if reBodyTitle.MatchString(s) || reBodyAuthor.MatchString(s) || reIntroMark.MatchString(s) {
		return true
	}
	if _, ok := parseHeading(s); ok {
		return true
	}
	return false
}

// detectMultiBook 判断是否为多书合集：头部出现多个书名标记，或章号反复重置。
func detectMultiBook(pre string, restarts int, chapters int) bool {
	if restarts >= 2 {
		return true
	}
	if strings.TrimSpace(pre) == "" {
		return false
	}
	cnt := 0
	for _, ln := range strings.Split(pre, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || runeLen(t) > 60 {
			continue
		}
		if reBookMark.MatchString(t) || reBodyAuthor.MatchString(t) {
			cnt++
			if cnt >= 3 {
				return true
			}
		}
	}
	return false
}

// hasHan 判断字符串是否含汉字。
func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
