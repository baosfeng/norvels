package parser

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// 文件名里应当被剥掉的广告/标签关键词（用于括号码、尾部标签）。
var nameAdWords = []string{
	"搜书吧", "soushu", "Soushu", "SOUSHU", "未删节", "未刪節", "无删减", "無刪減",
	"连载", "連載", "网址", "網址", "www", "WWW", "TXT", "付费", "付費", "整理",
	"强鹿", "16K", "高H", "高h", "精校", "加料", "校对", "校对版",
	"推荐", "福利", "番外", "大结局", "精排", "封面", "实体",
}

// 表示「状态」而不是作者的词（出现在书名-作者 的右侧时不当作者）。
var statusWords = []string{
	"全本", "完本", "完结", "连载", "連載", "精校", "更新", "未删节", "未刪節",
	"无删减", "無刪減", "高H", "整理", "收藏", "珍藏", "校对", "加料", "完结版",
	"完", "全", "章", "节", "卷", "集", "部",
}

func stripFileDecor(s string) string {
	s = strings.TrimSpace(s)
	for i := 0; i < 6; i++ {
		changed := false
		if t := reLeadMulti.ReplaceAllString(s, ""); t != s {
			s, changed = t, true
		}
		if t := reLeadOrder.ReplaceAllString(s, ""); t != s {
			s, changed = t, true
		}
		if t := strings.TrimLeft(s, " \t\u3000☆★◆◇●○■□▲△·•|-_—=#*＊"); t != s {
			s, changed = t, true
		}
		if inner, rest, ok := leadAdBracket(s); ok {
			_ = inner
			s, changed = rest, true
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
	t = trimOrphanClosers(strings.TrimSpace(t))
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

// leadAdBracket 判断串是否以「纯广告/标签的配对括号块」开头。
//
// 必须是配对括号（【…】）、且块后还有内容时才算：这样「【书名】（1-99）」
// 与「【书名》（…）」都不会被吃掉，而「【全本】书名」会被剥掉前缀。
