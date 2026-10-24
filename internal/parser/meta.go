package parser

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	reBookMark  = regexp.MustCompile("[《『]([^》』]{1,60})[》』]")
	reAuthorTag = regexp.MustCompile("(?:作\\s*者|著\\s*者|作\\s*家|编\\s*者)\\s*[:：]?\\s*([^\\s\\[\\]【】（）()《》,，]{1,24})")
	reTagBlock  = regexp.MustCompile("[\\[【(（][^\\]】)）]{0,60}[\\]】)）]")
	reLeadOrder = regexp.MustCompile("^\\d{1,5}(?:[.．\\-_]\\d{1,4})*[.．\\-_、\\s]*")
	reDigitSpan = regexp.MustCompile("\\d{1,5}\\s*[-~－至到_]\\s*\\d{1,5}")
	reTailSpan  = regexp.MustCompile("[\\s\\-_]*[（(【\\[]?\\d{1,5}\\s*[-~－至到_]\\s*\\d{1,5}\\s*[章节回集卷部]?\\s*[）)】\\]]?$")
	reTailNChap = regexp.MustCompile("[\\s\\-_]*[（(【\\[]?(?:全本)?\\d{1,5}\\s*[章节回集卷]\\s*[）)】\\]]?$")

	// 正文头部元信息。
	reBodyTitle        = regexp.MustCompile("^(?:书\\s*名|作品名|书名|标\\s*题)\\s*[:：]\\s*(.+)$")
	reBodyAuthor       = regexp.MustCompile("^(?:作\\s*者|著\\s*者|作\\s*家|编\\s*者)\\s*[:：]?\\s*(.+)$")
	reBodyAuthorSuffix = regexp.MustCompile("^(.{1,20})\\s*著$")
	reIntroMark        = regexp.MustCompile("^(?:【)?\\s*(?:内容简介|作品简介|内容介绍|简\\s*介|文\\s*案|简\\s*述)\\s*(?:】)?\\s*[:：]?\\s*(.*)$")
)

// 文件名里应当被剥掉的广告/标签关键词（用于括号码、尾部标签）。
var nameAdWords = []string{
	"搜书吧", "soushu", "Soushu", "SOUSHU", "未删节", "未刪節", "无删减", "無刪減",
	"连载", "連載", "网址", "網址", "www", "WWW", "TXT", "付费", "付費", "整理",
	"强鹿", "16K", "高H", "高h", "精校", "加料", "校对", "校对版",
}

// 表示「状态」而不是作者的词（出现在 X-Y 的 Y 位置时不当作者）。
var statusWords = []string{
	"全本", "完本", "完结", "连载", "連載", "精校", "更新", "未删节", "未刪節",
	"无删减", "無刪減", "高H", "整理", "收藏", "珍藏", "校对", "加料", "完结版",
	"完", "全", "章", "节", "卷", "集", "部",
}

// titleFromFilename 从文件名推断书名与作者。
//
// 规则（按优先级）：
//  1. 先剥装饰：数字前缀（640. / 10.1）、搜书吧/soushu@/TXT123、首尾空白；
//  2. 《…》 内即书名；
//  3. 整串被 【…】/[…] 包住时，内部按「X-Y」切分：X 为书名，Y 为作者
//     （Y 不能是「全本/完本/未删节」这类状态词，长度 ≤ 12）；
//  4. 「作者：xxx」作为作者；
//  5. 兜底：整串当书名，作者留空（由调用方落「佚名」）。
func titleFromFilename(name string) (title, author string, fromMark bool) {
	base := filepath.Base(name)
	if ext := filepath.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}
	if strings.TrimSpace(base) == "" {
		return "", "", false
	}

	s := stripFileDecor(base)
	if s == "" {
		return "", "", false
	}
	if loc := reAuthorTag.FindStringSubmatchIndex(s); loc != nil {
		author = cleanAuthorName(s[loc[2]:loc[3]])
		s = strings.TrimSpace(s[:loc[0]])
	}
	s = stripFileDecor(s)

	if m := reBookMark.FindStringSubmatch(s); m != nil {
		if t := cleanTitleText(m[1]); t != "" {
			return t, author, false
		}
	}

	// 整串被括号包住：【X-Y】最典型
	if inner, ok := unwrapWholeBrackets(s); ok {
		if t, a := splitTitleAuthor(inner); t != "" {
			if author == "" {
				author = a
			}
			return cleanTitleText(t), author, false
		}
	}

	s = stripTrailingTags(s)
	s = reLeadOrder.ReplaceAllString(s, "")
	s = reTailSpan.ReplaceAllString(s, "")
	s = reTailNChap.ReplaceAllString(s, "")
	s = stripTrailingTags(s)
	s = strings.TrimSpace(s)
	if t, a := splitTitleAuthor(s); t != "" {
		if author == "" {
			author = a
		}
		return cleanTitleText(t), author, false
	}
	return cleanTitleText(s), author, false
}

// stripFileDecor 去掉文件名开头的序号与广告装饰。
func stripFileDecor(s string) string {
	s = strings.TrimSpace(s)
	for i := 0; i < 6; i++ {
		changed := false
		if t := reLeadOrder.ReplaceAllString(s, ""); t != s {
			s, changed = t, true
		}
		if t := strings.TrimLeft(s, " \t\u3000☆★◆◇●○■□▲△·•|-_—=#*＊"); t != s {
			s, changed = t, true
		}
		if m := reTagBlock.FindStringIndex(s); m != nil && m[0] == 0 {
			inner := strings.Trim(s[m[0]:m[1]], "[]【】()（）")
			if isAdTagText(inner) {
				s, changed = s[m[1]:], true
			}
		}
		for _, ad := range []string{"soushu", "Soushu", "SOUSHU", "搜书吧", "强鹿", "www.", "http://", "https://"} {
			idx := strings.Index(s, ad)
			if idx <= 0 {
				continue
			}
			if strings.TrimSpace(s[:idx]) == "" || strings.HasPrefix(s, "@") {
				s, changed = strings.TrimLeft(s[idx+len(ad):], "0123456789.@-_ "), true
				break
			}
		}
		s = strings.TrimPrefix(s, "@")
		if !changed {
			break
		}
	}
	return strings.TrimSpace(s)
}

// isAdTagText 判断括号内的文本是否是广告/状态标签（而非书名的一部分）。
func isAdTagText(inner string) bool {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return true
	}
	if containsAdWord(inner) {
		return true
	}
	if reDigitSpan.MatchString(inner) {
		return true
	}
	onlyTag := true
	for _, r := range inner {
		if !unicode.IsDigit(r) && !strings.ContainsRune(" \t.．-_~～至到全本完结章集卷部第", r) {
			onlyTag = false
			break
		}
	}
	return onlyTag
}

func containsAdWord(s string) bool {
	for _, w := range nameAdWords {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// unwrapWholeBrackets 判断整串是否被一对括号完整包住，返回内部内容。
func unwrapWholeBrackets(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	pairs := map[rune]rune{'【': '】', '[': ']', '［': '］', '（': '）', '(': ')', '『': '』', '《': '》'}
	r, sz := utf8.DecodeRuneInString(s)
	closeRune, ok := pairs[r]
	if !ok {
		return "", false
	}
	last, lsz := utf8.DecodeLastRuneInString(s)
	if last != closeRune {
		return "", false
	}
	inner := s[sz : len(s)-lsz]
	if strings.ContainsRune(inner, r) {
		return "", false
	}
	return inner, true
}

// splitTitleAuthor 把「书名-作者」拆开；拆不出来时整串都是书名。
func splitTitleAuthor(s string) (title, author string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	idx, size := -1, 0
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if r == '-' || r == '－' || r == '—' || r == '–' {
			idx, size = i, sz
		}
		i += sz
	}
	if idx <= 0 || idx+size >= len(s) {
		return s, ""
	}
	left := strings.TrimSpace(s[:idx])
	right := strings.TrimSpace(s[idx+size:])
	if left == "" || right == "" {
		return s, ""
	}
	if runeLen(right) > 12 {
		return s, ""
	}
	for _, w := range statusWords {
		if strings.Contains(right, w) {
			return s, ""
		}
	}
	if strings.ContainsAny(right, "（）()【】[]，。：:、") {
		return s, ""
	}
	return left, right
}

// stripTrailingTags 反复剥掉末尾的「标签块」（(1-1990)、【全本】、全本完结 之类）。
func stripTrailingTags(s string) string {
	for i := 0; i < 8; i++ {
		s = strings.TrimSpace(s)
		if s == "" {
			return s
		}
		last, lsz := utf8.DecodeLastRuneInString(s)
		var open rune
		switch last {
		case '】':
			open = '【'
		case ']':
			open = '['
		case '）':
			open = '（'
		case ')':
			open = '('
		}
		if open != 0 {
			if idx := strings.LastIndex(s, string(open)); idx > 0 {
				inner := s[idx+len(string(open)) : len(s)-lsz]
				if isAdTagText(inner) {
					s = s[:idx]
					continue
				}
			}
		}
		if m := reAuthorTag.FindStringIndex(s); m != nil && m[1] == len(s) && m[0] > 0 {
			s = s[:m[0]]
			continue
		}
		if t := reTailSpan.ReplaceAllString(s, ""); t != s {
			s = t
			continue
		}
		if t := reTailNChap.ReplaceAllString(s, ""); t != s {
			s = t
			continue
		}
		return s
	}
	return s
}

// cleanTitleText 清理书名两端的装饰与多余空白。
func cleanTitleText(t string) string {
	t = strings.TrimSpace(t)
	t = strings.Trim(t, " \t\u3000★☆◆◇●○■□▲△·•|-_—=#*＊")
	t = strings.Join(strings.Fields(t), " ")
	if runeLen(t) > 80 {
		t = truncateRunes(t, 80)
	}
	return t
}

// cleanAuthorName 清理作者名。
func cleanAuthorName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, " \t\u3000:：,，、-—·•|()（）[]【】")
	s = strings.Join(strings.Fields(s), "")
	if runeLen(s) > 24 {
		s = truncateRunes(s, 24)
	}
	return s
}

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
