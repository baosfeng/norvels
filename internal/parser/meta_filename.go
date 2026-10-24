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
	// reLeadMulti 匹配「10.1」「1-2」这类带分隔的多段序号：必然是人造序号，直接剥。
	reLeadMulti = regexp.MustCompile("^\\d{1,5}(?:[.．\\-_]\\d{1,4})+[.．\\-_、\\s]*")
	// reLeadOrder 匹配「640. 」「12 」这类纯数字序号：要求数字后紧跟分隔符，
	// 这样「2012末日狂欢」这类以数字开头的真书名不会被当成序号剥掉。
	reLeadOrder = regexp.MustCompile("^\\d{1,5}[.．\\-_、\\s]+")
	reDigitSpan = regexp.MustCompile("\\d{1,5}\\s*[-~－至到_]\\s*\\d{1,5}")
	reTailSpan  = regexp.MustCompile("[\\s\\-_]*[（(【\\[]?\\d{1,5}\\s*[-~－至到_]\\s*\\d{1,5}\\s*[章节回集卷部]?\\s*[）)】\\]]?$")
	reTailNChap = regexp.MustCompile("[\\s\\-_]*[（(【\\[]?(?:全本)?\\d{1,5}\\s*[章节回集卷]\\s*[）)】\\]]?$")
)

// 文件名里应当被剥掉的广告/标签关键词（用于括号码、尾部标签）。
var nameAdWords = []string{
	"搜书吧", "soushu", "Soushu", "SOUSHU", "未删节", "未刪節", "无删减", "無刪減",
	"连载", "連載", "网址", "網址", "www", "WWW", "TXT", "付费", "付費", "整理",
	"强鹿", "16K", "高H", "高h", "精校", "加料", "校对", "校对版",
	"推荐", "福利", "番外", "大结局", "精排", "封面", "实体",
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
//  1. 剥装饰：数字序号（640. / 10.1）、搜书吧/soushu@/TXT123、首尾空白；
//  2. 「作者：xxx」先摘出来，剩下的部分再定书名；
//  3. 《…》 内即书名；
//  4. 以括号开头时取第一个括号块的内容，再在块内剥标签与「书名-作者」后缀；
//  5. 否则剥掉所有编号/广告括号块，剩下的主体按「书名-作者」切；
//  6. 兜底：整串当书名，作者留空（由调用方落「佚名」）。
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
	s = stripLeadOrder(stripFileDecor(s))
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}

	if m := reBookMark.FindStringSubmatch(s); m != nil {
		if t := cleanTitleText(m[1]); t != "" {
			return t, author, false
		}
	}
	// 以括号开头：【书名】(…) / 【书名】（1-99）/ 【书名subtitle】
	if inner, ok := firstBracketBlock(s); ok {
		if t, a := titleFromInner(inner); t != "" {
			if author == "" {
				author = a
			}
			return t, author, false
		}
	}
	// 剥掉所有编号/广告括号块，主体再切书名与作者。
	if t, a := titleFromInner(stripTitleTagBlocks(s)); t != "" {
		if author == "" {
			author = a
		}
		return t, author, false
	}
	t := cleanTitleText(stripTitleTailTags(s))
	return t, author, false
}

// titleFromInner 从「已剥掉包装」的串里定书名与作者。
func titleFromInner(s string) (title, author string) {
	s = stripTitleTagBlocks(s)
	s = stripTailTags(s)
	if s == "" {
		return "", ""
	}
	// 标签堆「书名，标签，标签」→ 只有第一段是书名。
	if parts := strings.Split(s, "，"); len(parts) >= 3 {
		s = parts[0]
	}
	if t, a := splitTitleAuthor(s); t != "" {
		if tt := cleanTitleText(t); tt != "" {
			return tt, a
		}
	}
	return cleanTitleText(s), ""
}

// stripLeadOrder 剥掉行首的数字序号（640. / 10.1 / 12、）。
func stripLeadOrder(s string) string {
	for i := 0; i < 4; i++ {
		before := s
		if m := reLeadMulti.FindStringIndex(s); m != nil && m[0] == 0 {
			s = s[m[1]:]
		}
		if m := reLeadOrder.FindStringIndex(s); m != nil && m[0] == 0 {
			s = s[m[1]:]
		}
		s = strings.TrimLeft(s, " \t\u3000.．-_—、")
		if s == before {
			break
		}
	}
	return s
}

// titleBrackets 是会被当作「包装」处理的括号对。
var titleBrackets = map[rune]rune{'【': '】', '[': ']', '［': '］', '『': '』', '《': '》'}

// firstBracketBlock 取第一个括号块的内容。
//
// 配对规则：优先找同类型的闭括号（【…】）；找不到同类型闭括号时退化为
// 「最近的一个闭括号」——这样能正确处理「【书名》（…）」这类左右不配对的写法。
func firstBracketBlock(s string) (string, bool) {
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		closer, ok := titleBrackets[r]
		if !ok {
			i += sz
			continue
		}
		rest := s[i+sz:]
		if j := strings.IndexRune(rest, closer); j >= 0 {
			return rest[:j], true
		}
		return nearestClose(rest), true
	}
	return "", false
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

// leadAdBracket 判断串是否以「纯广告/标签的配对括号块」开头。
//
// 必须是配对括号（【…】）、且块后还有内容时才算：这样「【书名】（1-99）」
// 与「【书名》（…）」都不会被吃掉，而「【全本】书名」会被剥掉前缀。
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
