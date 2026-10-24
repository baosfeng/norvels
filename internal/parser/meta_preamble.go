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

// reLooseAuthor 匹配「短行里的作者署名」。
//
// 与 reBodyAuthor 的差别：允许「作者」前有一个短前缀（常见于
// 「【书名】作者：某人」这种书名与署名同行的写法），但要求整行以署名结尾，
// 且前缀必须紧跟着闭括号——正文句子里的「作者」不会满足这两条。
var reLooseAuthor = regexp.MustCompile("^(?:[^\\s]{0,20}[】\\]）)>》])?\\s*(?:作\\s*者|著\\s*者|作\\s*家|编\\s*者)\\s*[:：]\\s*(\\S{1,24})$")

// reBracketAuthor 匹配「【作者：X】」这种署名被包在括号里的写法。
//
// 这类语料常把署名与章节标题写在同一行（「某书名【作者：某人】 第一章」），
// 所以不能要求整行以署名结尾，只能要求括号内就是署名。
var reBracketAuthor = regexp.MustCompile("【\\s*(?:作\\s*者|著\\s*者|作\\s*家|编\\s*者)\\s*[:：]\\s*([^】\\s]{1,24})\\s*】")

// authorFromEdges 在文档两端扫「像署名的短行」。
//
// 真实语料里署名有三种位置：正文头部元信息块、头部广告堆之后、文末版权块。
// 元信息块抽取只覆盖第一种，这里补后两种：
//   - 只看 ≤ 40 个字符的短行（正文段落几乎都更长）；
//   - 只认「作者：X」这类以署名收尾的行；
//   - 候选名要做状态词 / 广告词 / 数字过滤。
func authorFromEdges(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return ""
	}
	const window = 1500
	const maxScan = 4000
	head := lines
	if len(head) > maxScan {
		head = head[:maxScan]
	}
	if a := authorFromLines(head[:min(len(head), window)]); a != "" {
		return a
	}
	tail := lines
	if len(tail) > maxScan {
		tail = tail[len(tail)-maxScan:]
	}
	start := len(tail) - window
	if start < 0 {
		start = 0
	}
	return authorFromLines(tail[start:])
}

// authorFromLines 从一组行里取第一个可信的作者名。
func authorFromLines(lines []string) string {
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" || runeLen(ln) > 40 {
			continue
		}
		name := ""
		if mm := reLooseAuthor.FindStringSubmatch(ln); mm != nil {
			name = mm[1]
		} else if mm := reBracketAuthor.FindStringSubmatch(ln); mm != nil {
			name = mm[1]
		}
		if name == "" {
			continue
		}
		if name = cleanAuthorName(name); isPlausibleAuthor(name) {
			return name
		}
	}
	return ""
}

// isPlausibleAuthor 判断候选作者名是否可信。
func isPlausibleAuthor(name string) bool {
	if name == "" || runeLen(name) > 24 {
		return false
	}
	if containsAdWord(name) || isAdTagText(name) {
		return false
	}
	for _, w := range statusWords {
		if strings.Contains(name, w) {
			return false
		}
	}
	for _, r := range name {
		if r >= '0' && r <= '9' {
			return false
		}
		if strings.ContainsRune("。！？；，、：:", r) {
			return false
		}
	}
	return true
}
