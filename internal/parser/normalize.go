package parser

import (
	"strings"
	"unicode/utf8"
)

// 需要全局剔除的零宽/格式字符。
var zeroWidths = []string{"\uFEFF", "\u200B", "\u200C", "\u200D", "\u2060", "\u00AD"}

// normalizeText 做与内容语义无关的全局规范化：CRLF/CR → LF、去 BOM 与零宽字符。
// 全角空格/不间断空格在 trimLine 里按行处理（避免整篇逐字符重构）。
//
// 实现上用 strings.ReplaceAll 而不是逐字符扫描：只有真正出现过的模式才付扫描成本。
func normalizeText(s string) string {
	if s == "" {
		return s
	}
	if strings.Contains(s, "\r\n") {
		s = strings.ReplaceAll(s, "\r\n", "\n")
	}
	if strings.Contains(s, "\r") {
		s = strings.ReplaceAll(s, "\r", "\n")
	}
	for _, zw := range zeroWidths {
		if strings.Contains(s, zw) {
			s = strings.ReplaceAll(s, zw, "")
		}
	}
	return s
}

// trimLine 去掉行首行尾的空白（含全角空格与不间断空格）。
func trimLine(s string) string {
	i, j := 0, len(s)
	for i < j {
		switch {
		case s[i] == ' ' || s[i] == '\t':
			i++
		case s[i] == 0xE3 && i+2 < j && s[i+1] == 0x80 && s[i+2] == 0x80: // U+3000
			i += 3
		case s[i] == 0xC2 && i+1 < j && s[i+1] == 0xA0: // U+00A0
			i += 2
		default:
			goto tail
		}
	}
tail:
	for j > i {
		switch {
		case s[j-1] == ' ' || s[j-1] == '\t':
			j--
		case j >= 3 && s[j-3] == 0xE3 && s[j-2] == 0x80 && s[j-1] == 0x80:
			j -= 3
		case j >= 2 && s[j-2] == 0xC2 && s[j-1] == 0xA0:
			j -= 2
		default:
			return s[i:j]
		}
	}
	return s[i:j]
}

// runeLen 返回字符数（按 UTF-8 计算）。
func runeLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		n++
	}
	return n
}

// truncateRunes 按字符数截断。
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	i, cnt := 0, 0
	for i < len(s) && cnt < n {
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		cnt++
	}
	return s[:i]
}
