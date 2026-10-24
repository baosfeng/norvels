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

var titleBrackets = map[rune]rune{'【': '】', '[': ']', '［': '］', '『': '』', '《': '》'}

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
	for _, r := range "】]］』》（）)" {
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

// titleParens 是「普通」括号对（只用于块内标签剥离，不作为书名包装）。
var titleParens = map[rune]rune{'（': '）', '(': ')'}

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
