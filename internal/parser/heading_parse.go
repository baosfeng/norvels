package parser

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// parseHeading 解析一行标题。返回 false 表示这行不是标题。
func parseHeading(raw string) (heading, bool) {
	s := stripDecorations(raw)
	if s == "" || len(s) > maxHeadingBytes {
		return heading{}, false
	}
	// 有的书用大量空格做「两端对齐/居中」，先折叠空白再判定长度
	s = collapseSpaces(s)
	if len(s) < minHeadingBytes {
		return heading{}, false
	}
	// 统一排除：真正的章节标题行不会很长，也不会以句末标点收尾。
	if runeLen(s) > 40 {
		return heading{}, false
	}
	if last, _ := utf8.DecodeLastRuneInString(s); isSentenceTail(last) {
		return heading{}, false
	}
	// 去掉「正文」「正文卷」「VIP」这类前缀
	volPre := ""
	if strings.HasPrefix(s, "正文卷") {
		volPre = "正文卷"
		s = s[len("正文卷"):]
		s = trimLeadSeps(s)
	} else if strings.HasPrefix(s, "正文") {
		s = s[len("正文"):]
		s = trimLeadSeps(s)
	}
	if s == "" {
		if volPre != "" {
			return heading{kind: headVolume, volTitle: volPre, strong: true}, true
		}
		return heading{}, false
	}

	if h, ok := parseVolumeHeading(s, volPre); ok {
		return h, true
	}
	if h, ok := parseChapter(s, volPre != ""); ok {
		if volPre != "" {
			h.volTitle = volPre
		}
		return h, true
	}
	return heading{}, false
}

// parseVolumeHeading 解析「第N卷/部/集」开头的标题，可能同时带章节部分。
func parseVolumeHeading(s, volPre string) (heading, bool) {
	num, numText, rest, ok := cutDiNum(s)
	if !ok {
		return heading{}, false
	}
	rest = trimLeadSeps(rest)
	if rest == "" {
		if volPre != "" {
			return heading{}, false
		}
		return heading{kind: headVolume, volNum: num, volTitle: "第" + numText + "卷", strong: true}, true
	}
	r, sz := utf8.DecodeRuneInString(rest)
	if !isVolumeUnit(r) {
		return heading{}, false
	}
	base := "第" + numText + string(r)
	after := trimLeadSeps(rest[sz:])
	// 卷 + 章 同行
	if h, ok := parseChapter(after, false); ok && h.kind == headChapter {
		h.volNum = num
		h.volTitle = base
		return h, true
	}
	// 卷名 + 章 同行：在前缀里找章标记
	if idx := findChapterMarker(after); idx > 0 {
		vt := strings.TrimSpace(after[:idx])
		if h, ok := parseChapter(after[idx:], false); ok && h.kind == headChapter {
			title := base
			if vt != "" {
				title = base + " " + trimTitleNoise(vt)
			}
			h.volNum = num
			h.volTitle = title
			return h, true
		}
	}
	// 纯卷标题
	title := base
	if t := trimTitleNoise(after); t != "" {
		title = base + " " + t
	}
	return heading{kind: headVolume, volNum: num, volTitle: title, strong: true}, true
}

// parseChapter 解析章节标题（不含卷前缀）。
func parseChapter(s string, afterZhengwen bool) (heading, bool) {
	if h, ok := parseDiUnit(s, afterZhengwen); ok {
		return h, true
	}
	if strings.HasPrefix(s, "第") {
		// 第N卷/部/集 由卷解析处理
		return heading{}, false
	}
	if h, ok := parseSpecial(s); ok {
		return h, true
	}
	if h, ok := parseNumUnit(s, afterZhengwen); ok {
		return h, true
	}
	if h, ok := parseEnChapter(s); ok {
		return h, true
	}
	if h, ok := parseNumTitle(s); ok {
		return h, true
	}
	return heading{}, false
}

func parseDiUnit(s string, strong bool) (heading, bool) {
	num, numText, rest, ok := cutDiNum(s)
	if !ok {
		return heading{}, false
	}
	r, sz := utf8.DecodeRuneInString(rest)
	if !isChapterUnit(r) {
		return heading{}, false
	}
	marker := "第" + numText + string(r)
	title, inline := splitTitle(rest[sz:])
	return heading{
		kind: headChapter, num: num, hasNum: true, strong: true,
		marker: marker, title: title, inline: inline,
	}, true
}

func parseSpecial(s string) (heading, bool) {
	for _, m := range specialMarkers {
		if !strings.HasPrefix(s, m) {
			continue
		}
		rest := s[len(m):]
		// 「序」这类短标记要求后面没有紧跟内容
		if m == "序" || m == "感言" || m == "作品相关" {
			if rest != "" {
				r, _ := utf8.DecodeRuneInString(rest)
				if !isTitleSep(r) {
					continue
				}
			}
		}
		title, inline := splitTitle(rest)
		return heading{kind: headSpecial, strong: true, marker: m, title: title, inline: inline}, true
	}
	return heading{}, false
}

// parseNumUnit 解析「05章」「001 标题」这类以数字开头的行。
func parseNumUnit(s string, strong bool) (heading, bool) {
	num, numText, rest, ok := cutLeadingNumber(s)
	if !ok {
		return heading{}, false
	}
	if rest != "" {
		if r, sz := utf8.DecodeRuneInString(rest); isChapterUnit(r) {
			title, inline := splitTitle(rest[sz:])
			return heading{
				kind: headChapter, num: num, hasNum: true, strong: true,
				marker: numText + string(r), title: title, inline: inline,
			}, true
		}
	}
	_ = strong
	return heading{}, false
}

// parseNumTitle 解析弱特征的「0001」「1、标题」形式。
func parseNumTitle(s string) (heading, bool) {
	num, numText, rest, ok := cutLeadingNumber(s)
	if !ok {
		return heading{}, false
	}
	if rest == "" {
		// 纯数字行
		if len(numText) > 5 {
			return heading{}, false
		}
		return heading{kind: headChapter, num: num, hasNum: true, marker: numText}, true
	}
	r, sz := utf8.DecodeRuneInString(rest)
	if !isTitleSep(r) {
		return heading{}, false
	}
	body := trimLeadSeps(rest[sz:])
	body = strings.Trim(body, " \t\u3000")
	if body == "" {
		return heading{}, false
	}
	// 句末标点说明这多半是正文
	if strings.ContainsAny(body, "。！？；") && runeLen(body) > 12 {
		return heading{}, false
	}
	title, inline := splitTitle(body)
	return heading{
		kind: headChapter, num: num, hasNum: true, strong: false,
		marker: numText, title: title, inline: inline,
	}, true
}

func parseEnChapter(s string) (heading, bool) {
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "chapter") {
		return heading{}, false
	}
	rest := s[len("chapter"):]
	rest = strings.TrimLeft(rest, " \t")
	num, n, ok := parseNumeral(rest)
	if !ok {
		return heading{}, false
	}
	title, inline := splitTitle(rest[n:])
	return heading{
		kind: headChapter, num: num, hasNum: true, strong: true,
		marker: "Chapter " + fmt.Sprint(num), title: title, inline: inline,
	}, true
}

// splitTitle 把标记之后的文本切成「标题」与「误并入标题的正文」。
func splitTitle(rest string) (title, inline string) {
	raw := rest
	hadSep := false
	if raw != "" {
		if r, sz := utf8.DecodeRuneInString(raw); isTitleSep(r) {
			hadSep = true
			raw = raw[sz:]
		}
	}
	t := strings.Trim(raw, " \t\u3000\u00a0")
	t = trimTitleNoise(t)
	if t == "" {
		return "", ""
	}
	// 标题行里夹带正文：以句号切开
	if idx := strings.Index(t, "。"); idx > 0 {
		head := strings.TrimSpace(t[:idx])
		if runeLen(head) <= 60 {
			return head, strings.TrimSpace(t[idx+len("。"):])
		}
	}
	// 标记与正文粘连（无分隔符）且内容很长：整行视为正文
	if !hadSep && runeLen(t) > 14 {
		return "", t
	}
	if runeLen(t) > 40 {
		return truncateRunes(t, 40), ""
	}
	return t, ""
}
