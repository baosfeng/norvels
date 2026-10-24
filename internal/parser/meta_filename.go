package parser

import (
	"path/filepath"
	"regexp"
	"strings"
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
