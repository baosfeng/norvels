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
)

// titleFromFilename 从文件名推断书名与作者。
//
// 规则（按优先级）：
//  1. 剥装饰：数字序号（640. / 10.1）、搜书吧/soushu@/TXT123、首尾空白；
//  2. 「作者：xxx」先摘出来，剩下的部分再定书名；
//  3. 《…》 内即书名；
//  4. 依次尝试每个括号块：跳过纯广告块（如「[搜书吧]」），取第一个抽得出书名的；
//  5. 剥掉所有编号/广告括号块后，主体按「书名-作者」切分；
//  6. 兜底：整串当书名，作者留空（由调用方落「佚名」）。
func titleFromFilename(name string) (title, author string, fromMark bool) {
	base := filepath.Base(name)
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	if strings.TrimSpace(base) == "" {
		return "", "", false
	}
	s := stripFileDecor(base)
	if s == "" {
		return "", "", false
	}
	// 来源/分类标记在前、书名在后：「【某标记】某书名…」。要在摘作者之前判断——
	// 摘掉「作者：xxx」后主体可能只剩两三个字，长度判据会失效。
	if t, a, ok := titleAfterLeadMark(stripLeadOrder(s)); ok {
		return t, a, false
	}
	return titleFromCleanName(s)
}

// titleFromCleanName 处理「剥完装饰、且不是来源标记开场」的文件名主干。
func titleFromCleanName(s string) (title, author string, fromMark bool) {
	if loc := reAuthorTag.FindStringSubmatchIndex(s); loc != nil {
		author = cleanAuthorName(s[loc[2]:loc[3]])
		s = strings.TrimSpace(s[:loc[0]])
	}
	s = stripLeadOrder(stripFileDecor(s))
	if s == "" {
		return "", "", false
	}

	if m := reBookMark.FindStringSubmatch(s); m != nil {
		if t := cleanTitleText(m[1]); t != "" {
			return t, author, false
		}
	}
	for _, inner := range bracketBlocks(s) {
		t, a := titleFromInner(inner)
		if !isUsableTitle(t) {
			continue
		}
		if author == "" {
			author = a
		}
		return t, author, false
	}
	if t, a := titleFromInner(stripTitleTagBlocks(s)); isUsableTitle(t) {
		if author == "" {
			author = a
		}
		return t, author, false
	}
	return cleanTitleText(stripTitleTailTags(s)), author, false
}

// titleAfterLeadMark 处理「来源/分类标记在前、书名在后」的文件名：「【某标记】某书名…」。
//
// 括号块本身不是书名包装（站点、作者名、分类），主体才是书名——语料里这种写法很多，
// 按「第一个括号块就是书名」处理会让整簇书都叫同一个标记名。
//
// 判据取保守组合：标记 ≤6 汉字、主体 ≥6 汉字、且主体至少是标记的两倍长。
// 这样「【某书名】重修加料版」（主体 5 字）仍按书名处理，不会被尾部标签顶掉。
const (
	leadMarkMaxHan = 6 // 标记（来源/分类/作者名）的汉字上限
	leadBodyMinHan = 4 // 主体的汉字下限：太短就不是书名
)

// reLeadMarkTail 匹配主体末尾的「1-530」「1-1100完本」这类连载区间记号。
var reLeadMarkTail = regexp.MustCompile("[\\s\\-_（(]*(?:\\d{1,5}\\s*[-~－至到_]\\s*\\d{1,5}|\\d{1,5})\\s*(?:完本|全本|完结)?[）)]?$")

func titleAfterLeadMark(s string) (title, author string, ok bool) {
	r, sz := utf8.DecodeRuneInString(s)
	closer, ok := titleBrackets[r]
	if !ok {
		return "", "", false
	}
	body := s[sz:]
	j := strings.IndexRune(body, closer)
	if j < 0 {
		return "", "", false
	}
	mark := strings.TrimSpace(body[:j])
	if hanLen(mark) == 0 || hanLen(mark) > leadMarkMaxHan {
		return "", "", false
	}
	rest := strings.TrimSpace(body[j+len(string(closer)):])
	if loc := reAuthorTag.FindStringSubmatchIndex(rest); loc != nil {
		author = cleanAuthorName(rest[loc[2]:loc[3]])
		rest = strings.TrimSpace(rest[:loc[0]])
	}
	rest = trimLeadMarkTail(stripTitleTagBlocks(rest))
	if !leadBodyLongEnough(rest, mark, author) {
		return "", "", false
	}
	// 不走 titleFromInner：它会把「书名，书名续段，续段」里的逗号当标签堆切掉，
	// 而这里的整段主体本来就是书名（作者已单独摘走）。
	t := cleanTitleText(trimLeadMarkTail(stripTailTags(rest)))
	if !leadBodyLongEnough(t, mark, author) || containsAdWord(t) {
		return "", "", false
	}
	return t, author, true
}

// leadBodyLongEnough 判断主体是否够长、不像尾部标签。
//
// 摘到了作者是强信号（「【某标记】某书名 作者：xxx」）：此时两个字的书名也认。
// 没有作者时保守：主体至少 4 个汉字，且不少于标记的两倍长——这样「【某书名】重修加料版」
// 仍按书名处理，不会被尾部标签顶掉。
func leadBodyLongEnough(body, mark, author string) bool {
	if hanLen(body) < 2 {
		return false
	}
	if author != "" {
		return true
	}
	return hanLen(body) >= leadBodyMinHan && hanLen(body) >= 2*hanLen(mark)
}

// trimLeadMarkTail 剥掉主体尾部的连载区间记号（「1-530完本」这类）。
func trimLeadMarkTail(s string) string {
	for i := 0; i < 4; i++ {
		m := reLeadMarkTail.FindStringIndex(s)
		if m == nil || m[1] != len(s) || m[0] == 0 {
			break
		}
		s = strings.TrimSpace(s[:m[0]])
	}
	return s
}

// hanLen 数汉字个数：判据只看汉字，标签里的数字与标点不参与。
func hanLen(s string) int {
	n := 0
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			n++
		}
	}
	return n
}

// titleFromInner 从「已剥掉包装」的串里定书名与作者。
func titleFromInner(s string) (title, author string) {
	s = trimUnclosedBracket(s)
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

// enforceCleanTitle 是入库前的不变量校验：书名不应带任何括号。
//
// 清洗规则没吃干净时（例如括号嵌套/不配对到了规则之外），取「最长无括号片段」
// 兜底，并记 warning 让人能从日志定位——比把带括号的书名写进库好得多。
func enforceCleanTitle(d *Doc, title string) string {
	if !hasBracket(title) {
		return title
	}
	if t := longestBracketFree(title); isUsableTitle(t) && runeLen(t) >= 2 {
		d.Warn("书名清洗后仍含括号，已取最长无括号片段：%q → %q", title, t)
		return t
	}
	d.Warn("书名清洗后仍含括号且无可用的无括号片段：%q", title)
	return title
}
