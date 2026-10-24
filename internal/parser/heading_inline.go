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
//	"  第三章   女歌手   一位身穿白色短裙…"
//
// 返回解析出的标题以及「正文从本行第几个字节开始」。
func parseInlineHeading(line string) (heading, int, bool) {
	skip := 0
	for skip < len(line) {
		r, sz := utf8.DecodeRuneInString(line[skip:])
		if !isLeadDecor(r) {
			break
		}
		skip += sz
	}
	if skip >= len(line) {
		return heading{}, 0, false
	}
	rest := line[skip:]
	var h heading
	consumed := 0
	if num, numText, r2, ok := cutDiNum(rest); ok {
		if u, sz := utf8.DecodeRuneInString(r2); isVolumeUnit(u) {
			consumed = len("第") + len(numText) + sz
			h = heading{kind: headVolume, volNum: num, volTitle: "第" + numText + string(u), strong: true}
		} else if isChapterUnit(u) {
			consumed = len("第") + len(numText) + sz
			h = heading{kind: headChapter, num: num, hasNum: true, strong: true,
				marker: "第" + numText + string(u)}
		}
	}
	if consumed == 0 {
		for _, m := range specialMarkers {
			if strings.HasPrefix(rest, m) {
				consumed = len(m)
				h = heading{kind: headSpecial, strong: true, marker: m}
				break
			}
		}
	}
	if consumed == 0 {
		return heading{}, 0, false
	}

	pos := skip + consumed
	// 跳过标题前的填充
	for pos < len(line) {
		r, sz := utf8.DecodeRuneInString(line[pos:])
		if !isPadRune(r) && !isTitleSep(r) {
			break
		}
		pos += sz
	}
	titleStart := pos
	titleEnd := pos
	runes := 0
	for titleEnd < len(line) && runes < 20 {
		r, sz := utf8.DecodeRuneInString(line[titleEnd:])
		if isSentenceTail(r) {
			break
		}
		if isPadRune(r) || isTitleSep(r) {
			// 连续填充 → 标题结束
			if nr, _ := utf8.DecodeRuneInString(line[titleEnd+sz:]); isPadRune(nr) || isTitleSep(nr) {
				break
			}
		}
		titleEnd += sz
		runes++
	}
	h.title = strings.Trim(line[titleStart:titleEnd], " 	 　")
	if h.title != "" && strings.Contains(h.title, "。") {
		h.title = ""
	}
	bodyPos := titleEnd
	for bodyPos < len(line) {
		r, sz := utf8.DecodeRuneInString(line[bodyPos:])
		if !isPadRune(r) && !isTitleSep(r) {
			break
		}
		bodyPos += sz
	}
	if h.title == "" {
		h.title = ""
	}
	return h, bodyPos, true
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
