package parser

import (
	"strings"
	"unicode/utf8"
)

// headingLeadOK 用首字符快速排除绝大多数正文行。
func headingLeadOK(s string) bool {
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if isLeadDecor(r) {
			i += sz
			continue
		}
		switch r {
		case '第', '序', '楔', '契', '引', '前', '后', '後', '终', '終', '尾', '番',
			'外', '附', '正', '文', '完', '感', '作', '最', '大', '结', '結', '局',
			'C', 'c':
			return true
		}
		if (r >= '0' && r <= '9') || (r >= '０' && r <= '９') {
			return true
		}
		return false
	}
	return false
}

// parseInlineHeading 处理「标题与正文挤在同一行」的情况，例如：
//
//	"  第三章   女歌手   一位身穿白色短裙…"
//
// 返回解析出的标题以及「正文从本行第几个字节开始」。
func parseInlineHeading(line string) (heading, int, bool) {
	skip := skipLeadDecor(line)
	if skip >= len(line) {
		return heading{}, 0, false
	}
	h, consumed, ok := parseInlineMarker(line[skip:])
	if !ok {
		return heading{}, 0, false
	}
	pos := skip + consumed
	for pos < len(line) {
		r, sz := utf8.DecodeRuneInString(line[pos:])
		if !isPadRune(r) && !isTitleSep(r) {
			break
		}
		pos += sz
	}
	titleEnd := scanInlineTitle(line, pos)
	h.title = strings.Trim(line[pos:titleEnd], " \t\u3000")
	if strings.Contains(h.title, "。") {
		h.title = ""
	}
	return h, skipInlineSep(line, titleEnd), true
}

// skipLeadDecor 跳过行首装饰字符，返回正文开始的下标。
func skipLeadDecor(line string) int {
	skip := 0
	for skip < len(line) {
		r, sz := utf8.DecodeRuneInString(line[skip:])
		if !isLeadDecor(r) {
			break
		}
		skip += sz
	}
	return skip
}

// parseInlineMarker 解析行内标题的标记部分（第N章 / 序章 / 楔子…）。
func parseInlineMarker(rest string) (heading, int, bool) {
	if num, numText, after, ok := cutDiNum(rest); ok {
		u, sz := utf8.DecodeRuneInString(after)
		consumed := len("第") + len(numText) + sz
		switch {
		case isVolumeUnit(u):
			return heading{kind: headVolume, volNum: num, volTitle: "第" + numText + string(u), strong: true}, consumed, true
		case isChapterUnit(u):
			return heading{kind: headChapter, num: num, hasNum: true, strong: true,
				marker: "第" + numText + string(u)}, consumed, true
		}
	}
	for _, m := range specialMarkers {
		if strings.HasPrefix(rest, m) {
			return heading{kind: headSpecial, strong: true, marker: m}, len(m), true
		}
	}
	return heading{}, 0, false
}

// scanInlineTitle 从 start 开始扫标题的结束位置：遇句末标点或连续填充即止。
func scanInlineTitle(line string, start int) int {
	titleEnd, runes := start, 0
	for titleEnd < len(line) && runes < 20 {
		r, sz := utf8.DecodeRuneInString(line[titleEnd:])
		if isSentenceTail(r) {
			break
		}
		if isPadRune(r) || isTitleSep(r) {
			if nr, _ := utf8.DecodeRuneInString(line[titleEnd+sz:]); isPadRune(nr) || isTitleSep(nr) {
				break
			}
		}
		titleEnd += sz
		runes++
	}
	return titleEnd
}

// skipInlineSep 跳过标题与正文之间的分隔填充，返回正文起点。
func skipInlineSep(line string, from int) int {
	for from < len(line) {
		r, sz := utf8.DecodeRuneInString(line[from:])
		if !isPadRune(r) && !isTitleSep(r) {
			break
		}
		from += sz
	}
	return from
}

// parsePrefixedHeading 解析「书名/篇名 + 第N章」这类带短前缀的标题行。
func parsePrefixedHeading(line string) (heading, bool) {
	idx := strings.Index(line, "第")
	if idx <= 0 || idx > 48 {
		return heading{}, false
	}
	// 「第」前面必须是分隔符：否则多半是正文里的「（见第六章）」这类引用
	if !hasSepBeforeByte(line, idx) {
		return heading{}, false
	}
	prefix := strings.TrimSpace(line[:idx])
	if prefix == "" || runeLen(prefix) > 16 {
		return heading{}, false
	}
	if strings.ContainsAny(prefix, "。！？；，：") || strings.HasSuffix(prefix, "的") {
		return heading{}, false
	}
	lr, _ := utf8.DecodeLastRuneInString(line)
	switch lr {
	case '。', '！', '？', '；', '…', '”', '」':
		return heading{}, false
	}
	h, ok := parseChapter(line[idx:], false)
	if !ok || !h.strong || h.kind != headChapter {
		return heading{}, false
	}
	if h.title == "" {
		return heading{}, false
	}
	h.volTitle = trimTitleNoise(prefix)
	return h, true
}
