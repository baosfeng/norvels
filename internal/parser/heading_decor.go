package parser

import (
	"strings"
	"unicode/utf8"
)

// isLeadDecor 判断行首装饰字符（可成串出现）。
func isLeadDecor(r rune) bool {
	switch r {
	case ' ', '\t', '\u3000', '\u00a0', '=', '#', '|', '☆', '★', '◆', '◇', '●', '○',
		'■', '□', '▪', '▫', '·', '•', '~', '～', '_', '-', '—', '－', '*', '＊',
		'【', '】', '[', ']', '［', '］', '「', '」', '『', '』', '《', '》',
		'“', '”', '‘', '’', '"', '\'', '〔', '〕', '〖', '〗', '、', '，', ',', '.':
		return true
	}
	return false
}

// isPadRune 判断是否是排版用的填充空白（含不间断空格）。
func isPadRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\u3000' || r == '\u00a0'
}

// isTailDecor 判断行尾装饰字符（不含括号，避免误伤「（上）」）。
func isTailDecor(r rune) bool {
	switch r {
	case ' ', '\t', '\u3000', '\u00a0', '=', '#', '|', '☆', '★', '◆', '◇', '●', '○',
		'■', '□', '·', '•', '~', '～', '_', '-', '—', '－', '*', '＊', '、':
		return true
	}
	return false
}

// isTitleSep 判断标记与标题之间的分隔字符。
func isTitleSep(r rune) bool {
	switch r {
	case ' ', '\t', '\u3000', '\u00a0', '：', ':', '、', '，', ',', '.', '．', '·',
		'-', '—', '－', '☆', '★', '◆', '◇', '|', '~', '～', '_', '=', '#',
		'“', '”', '"', '‘', '’':
		return true
	}
	return false
}

// collapseSpaces 把连续空白（含全角空格）折叠成一个空格。
func collapseSpaces(s string) string {
	if !strings.Contains(s, "  ") && !strings.Contains(s, "　") && !strings.Contains(s, "	") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	prev := false
	for _, r := range s {
		if r == ' ' || r == '	' || r == '　' {
			if !prev {
				b.WriteByte(' ')
				prev = true
			}
			continue
		}
		prev = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// isSentenceTail 判断是否句末标点（标题不会以这些结尾）。
func isSentenceTail(r rune) bool {
	switch r {
	case '，', '。', '！', '？', '；', '…', '”', '」', '『', '．':
		return true
	}
	return false
}

// stripDecorations 去掉行首行尾装饰字符。
func stripDecorations(s string) string {
	i := 0
	for i < len(s) {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if !isLeadDecor(r) {
			break
		}
		i += sz
	}
	s = s[i:]
	j := len(s)
	for j > 0 {
		r, sz := utf8.DecodeLastRuneInString(s[:j])
		if !isTailDecor(r) {
			break
		}
		j -= sz
	}
	return s[:j]
}

func trimLeadSeps(s string) string {
	i := 0
	for i < len(s) {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if !isTitleSep(r) && r != '】' && r != ']' && r != '］' && r != ')' && r != '）' {
			break
		}
		i += sz
	}
	return s[i:]
}

// trimTitleNoise 清理标题两端的装饰与内联广告。
func trimTitleNoise(t string) string {
	t = stripInlineNoiseFull(t)
	t = strings.Trim(t, " \t\u3000\u00a0")
	t = strings.TrimRight(t, "=#|☆★◆◇○●■□·•~～_—－*＊")
	return strings.TrimSpace(t)
}

// hasSepBeforeByte 判断 line[idx] 之前是否是分隔符（空格/全角空格/·/：/|）。
func hasSepBeforeByte(line string, idx int) bool {
	if idx <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(line[:idx])
	switch r {
	case ' ', '	', '　', '·', '：', ':', '|', '｜', '_', '-', '—':
		return true
	}
	return false
}

// joinTitle 拼出「标记 + 标题」，保留原始的「第N章 / 第一回 / 0001」写法。
func joinTitle(marker, title string) string {
	if title == "" {
		return marker
	}
	if marker == "" {
		return title
	}
	if strings.HasPrefix(title, marker) {
		return title
	}
	return marker + " " + title
}
