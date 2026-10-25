package parser

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// 以下正则与词表服务于「标签块识别 / 尾部编号剥离 / 广告词过滤」。
var (
	reDigitSpan = regexp.MustCompile("\\d{1,5}\\s*[-~－至到_]\\s*\\d{1,5}")
	reTailSpan  = regexp.MustCompile("[\\s\\-_]*[（(【\\[]?\\d{1,5}\\s*[-~－至到_]\\s*\\d{1,5}\\s*[章节回集卷部]?\\s*[）)】\\]]?$")
	reTailNChap = regexp.MustCompile("[\\s\\-_]*[（(【\\[]?(?:全本)?\\d{1,5}\\s*[章节回集卷]\\s*[）)】\\]]?$")
)

// titleBrackets 是「书名包装」用的开闭括号对（不含普通圆括号）。
var titleBrackets = map[rune]rune{
	'【': '】', '[': ']', '［': '］', '『': '』', '《': '》', '「': '」', '〈': '〉',
}

// titleParens 是普通圆括号对：只用于标签块判定，不作为书名包装。
var titleParens = map[rune]rune{'（': '）', '(': ')'}

// closerToOpener 是全部闭括号到开括号的反查表（含圆括号）。
var closerToOpener = func() map[rune]rune {
	m := make(map[rune]rune, 16)
	for o, c := range titleBrackets {
		m[c] = o
	}
	for o, c := range titleParens {
		m[c] = o
	}
	return m
}()

// isOpener 判断是否为任一开括号。
func isOpener(r rune) bool {
	if _, ok := titleBrackets[r]; ok {
		return true
	}
	_, ok := titleParens[r]
	return ok
}

// closerOf 返回开括号对应的闭括号。
func closerOf(r rune) (rune, bool) {
	if c, ok := titleBrackets[r]; ok {
		return c, true
	}
	c, ok := titleParens[r]
	return c, ok
}

// firstBracketBlock 取第一个括号块的内容。
//
// 配对规则：优先找同类型的闭括号（【…】）；找不到同类型闭括号时退化为
// 「最近的一个闭括号」——这样能正确处理「【书名》（…）」这类左右不配对的写法。
func bracketBlocks(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		closer, ok := titleBrackets[r]
		if !ok {
			i += sz
			continue
		}
		rest := s[i+sz:]
		j := strings.IndexRune(rest, closer)
		if j < 0 {
			// 没有同类型闭括号：退化为「到最近闭括号」，且不再继续往后找。
			if inner := strings.TrimSpace(nearestClose(rest)); inner != "" {
				out = append(out, inner)
			}
			return out
		}
		if inner := strings.TrimSpace(rest[:j]); inner != "" {
			out = append(out, inner)
		}
		i += sz + j + len(string(closer))
	}
	return out
}

// isUsableTitle 判断从括号块里抽出的书名是否可用（过滤纯广告/空串）。
func isUsableTitle(t string) bool {
	t = strings.TrimSpace(t)
	return t != "" && !containsAdWord(t) && !isAdTagText(t)
}

// nearestClose 返回从开头到最近一个闭括号之间的内容（找不到则整段）。
func nearestClose(s string) string {
	best := -1
	for _, r := range "】]］』》」〉（）)" {
		if j := strings.IndexRune(s, r); j >= 0 && (best < 0 || j < best) {
			best = j
		}
	}
	if best < 0 {
		return s
	}
	return s[:best]
}

// stripTitleTagBlocks 删掉串里所有被判定为编号/广告的括号块。
//
// 「书名（副标题）」这类真副标题不会被删——判定交给 isAdTagText。
func stripTitleTagBlocks(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		closer, ok := titleBrackets[r]
		if !ok {
			closer, ok = titleParens[r]
		}
		if !ok {
			b.WriteString(s[i : i+sz])
			i += sz
			continue
		}
		j := strings.IndexRune(s[i+sz:], closer)
		if j < 0 {
			i += sz
			continue
		}
		end := i + sz + j + len(string(closer))
		if end > len(s) {
			end = len(s)
		}
		inner := s[i+sz : i+sz+j]
		if isAdTagText(inner) {
			i = end
			continue
		}
		b.WriteString(s[i:end])
		i = end
	}
	return strings.TrimSpace(b.String())
}

// stripTailTags 反复剥掉串尾的标签块与数字区间。
func stripTailTags(s string) string {
	for i := 0; i < 8; i++ {
		before := s
		s = strings.TrimSpace(stripTrailingTags(s))
		if m := reTailSpan.FindStringIndex(s); m != nil && m[1] == len(s) {
			s = strings.TrimSpace(s[:m[0]])
		}
		if m := reTailNChap.FindStringIndex(s); m != nil && m[1] == len(s) {
			s = strings.TrimSpace(s[:m[0]])
		}
		if s == before {
			break
		}
	}
	return strings.TrimSpace(s)
}

// stripTitleTailTags 剥掉书名末尾的编号括号，如「（01_99）」「(1-5卷27章)」。
func stripTitleTailTags(t string) string {
	for i := 0; i < 8; i++ {
		t = strings.TrimSpace(t)
		if t == "" {
			return t
		}
		last, lsz := utf8.DecodeLastRuneInString(t)
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
		default:
			return t
		}
		idx := strings.LastIndex(t, string(open))
		if idx <= 0 {
			return t
		}
		inner := t[idx+len(string(open)) : len(t)-lsz]
		if !isAdTagText(inner) && !reDigitSpan.MatchString(inner) && !reTailNChap.MatchString(inner) {
			return t
		}
		t = t[:idx]
	}
	return t
}

// stripFileDecor 去掉文件名开头的序号与广告装饰。
func leadAdBracket(s string) (inner, rest string, ok bool) {
	r, sz := utf8.DecodeRuneInString(s)
	closer, isPair := titleBrackets[r]
	if !isPair {
		return "", "", false
	}
	body := s[sz:]
	j := strings.IndexRune(body, closer)
	if j < 0 {
		return "", "", false
	}
	inner = strings.TrimSpace(body[:j])
	rest = strings.TrimSpace(body[j+len(string(closer)):])
	if rest == "" || !isAdTagText(inner) {
		return "", "", false
	}
	return inner, rest, true
}

// firstBracketBlock 返回第一个能抽出可用书名的括号块内容。
func firstBracketBlock(s string) (string, bool) {
	for _, inner := range bracketBlocks(s) {
		if t, _ := titleFromInner(inner); isUsableTitle(t) {
			return inner, true
		}
	}
	return "", false
}

// trimUnclosedBracket 截掉「未闭合的开括号」及其后全部内容。
//
// 语料里常见「【书名（1-99）【标签，标签】」这种嵌套未配对的写法：外层【的配对】
// 其实是内层那个，导致书名后面拖着一整段标签。截断比猜测更安全。
func trimUnclosedBracket(s string) string {
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		closer, ok := titleBrackets[r]
		if !ok {
			closer, ok = titleParens[r]
		}
		if !ok {
			i += sz
			continue
		}
		if j := strings.IndexRune(s[i+sz:], closer); j < 0 {
			return strings.TrimSpace(s[:i])
		}
		i += sz
	}
	return s
}

// trimOrphanClosers 删除所有「没有对应开括号」的孤立闭括号。
//
// 与 trimUnclosedBracket 严格对称：那个截掉孤立**开**括号及其后内容，
// 这个剥掉孤立**闭**括号本身。真实语料里「【书名》（1-99）》」这类写法
// 会在书名中间留下孤立的 》；只做两端 Trim 是清不掉的。
func trimOrphanClosers(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	// 各类开括号的未闭合计数：遇到开括号 +1，遇到对应闭括号 -1。
	open := make(map[rune]int, len(titleBrackets)+len(titleParens))
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if o, ok := closerToOpener[r]; ok {
			if open[o] > 0 {
				open[o]--
				b.WriteString(s[i : i+sz])
			}
			// 没有对应开括号 → 孤立闭括号，直接丢弃。
			i += sz
			continue
		}
		if isOpener(r) {
			open[r]++
		}
		b.WriteString(s[i : i+sz])
		i += sz
	}
	return strings.TrimSpace(b.String())
}

// longestBracketFree 取串里最长的「不含任何括号」子串。
//
// 用于入库前的最后兜底：书名清洗后仍带括号，说明包装规则没吃干净，
// 这时「最长纯文本段」比带着括号的串更像书名。
func longestBracketFree(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isOpener(r) {
			b.WriteRune('\n')
			continue
		}
		if _, ok := closerToOpener[r]; ok {
			b.WriteRune('\n')
			continue
		}
		b.WriteRune(r)
	}
	best := ""
	for _, seg := range strings.Split(b.String(), "\n") {
		if seg = strings.TrimSpace(seg); runeLen(seg) > runeLen(best) {
			best = seg
		}
	}
	return strings.TrimSpace(best)
}

// hasBracket 判断串里是否还残留任何括号字符。
func hasBracket(s string) bool {
	for _, r := range s {
		if isOpener(r) {
			return true
		}
		if _, ok := closerToOpener[r]; ok {
			return true
		}
	}
	return false
}
