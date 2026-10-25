package parser

import (
	"strings"
	"unicode/utf8"
)

// cnDigitValue 是中文数字字符到数值的映射。
//
// 用查表而不是 strings.IndexRune 取序号：IndexRune 返回的是字节偏移，
// 直接当数值用会把「一」算成 5、「三」算成 11 这类灾难性错误。
var cnDigitValue = map[rune]int{
	'零': 0, '〇': 0,
	'一': 1, '二': 2, '三': 3, '四': 4, '五': 5,
	'六': 6, '七': 7, '八': 8, '九': 9, '两': 2,
}

// parseNumeral 解析行首数字（阿拉伯数字/全角数字/中文数字），返回数值与消耗的字节数。
//
// 三层依次尝试：ASCII 数字最便宜，其次全角数字，最后中文数字（最复杂）。
// 任一层命中即返回，全部不命中返回 ok=false。
func parseNumeral(s string) (num int, n int, ok bool) {
	if s == "" {
		return 0, 0, false
	}
	if c := s[0]; c >= '0' && c <= '9' {
		return parseASCIINumeral(s)
	}
	if s[0] == 0xEF {
		return parseFullWidthNumeral(s)
	}
	return parseCNDNumeral(s)
}

// parseASCIINumeral 解析行首的 ASCII 数字串（最多 7 位）。
func parseASCIINumeral(s string) (num int, n int, ok bool) {
	v, i := 0, 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		v = v*10 + int(s[i]-'0')
		i++
		if i >= 7 {
			break
		}
	}
	return v, i, v > 0
}

// parseFullWidthNumeral 解析行首的全角数字串 ０-９（UTF-8 为 EF BC 90 ~ EF BC 99）。
func parseFullWidthNumeral(s string) (num int, n int, ok bool) {
	v, i, cnt := 0, 0, 0
	for i+2 < len(s) && s[i] == 0xEF && s[i+1] == 0xBC && s[i+2] >= 0x90 && s[i+2] <= 0x99 {
		v = v*10 + int(s[i+2]-0x90)
		i += 3
		cnt++
		if cnt >= 7 {
			break
		}
	}
	if cnt == 0 {
		return 0, 0, false
	}
	return v, i, v > 0
}

// parseCNDNumeral 解析行首的中文数字（支持十/百/千/万的分段累加）。
func parseCNDNumeral(s string) (num int, n int, ok bool) {
	total, section, number, cnt := 0, 0, 0, 0
	i := 0
	for i < len(s) {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if d, hit := cnDigitValue[r]; hit {
			number = d
			i += sz
			cnt++
			continue
		}
		if u := cnUnitValue(r); u > 0 {
			if number == 0 {
				number = 1
			}
			section += number * u
			number = 0
			i += sz
			cnt++
			continue
		}
		if r == '万' {
			if number == 0 && section == 0 {
				number = 1
			}
			total += (section + number) * 10000
			section, number = 0, 0
			i += sz
			cnt++
			continue
		}
		break
	}
	if cnt == 0 {
		return 0, 0, false
	}
	total += section + number
	if total <= 0 || total > 100000 {
		return 0, 0, false
	}
	return total, i, true
}

// cnUnitValue 返回中文数位字符的权值（十/百/千），非数位返回 0。
func cnUnitValue(r rune) int {
	switch r {
	case '十':
		return 10
	case '百':
		return 100
	case '千':
		return 1000
	}
	return 0
}

// cutDiNum 解析行首的「第」+ 数字。
func cutDiNum(s string) (num int, numText string, rest string, ok bool) {
	if !strings.HasPrefix(s, "第") {
		return 0, "", "", false
	}
	r := s[len("第"):]
	// 第 与数字之间允许空格
	trimmed := 0
	for trimmed < len(r) {
		c := r[trimmed]
		if c == ' ' || c == '\t' {
			trimmed++
			continue
		}
		if c == 0xE3 && trimmed+2 < len(r) && r[trimmed+1] == 0x80 && r[trimmed+2] == 0x80 {
			trimmed += 3 // 全角空格
			continue
		}
		break
	}
	v, n, valid := parseNumeral(r[trimmed:])
	if !valid {
		return 0, "", "", false
	}
	return v, string(r[:trimmed+n]), r[trimmed+n:], true
}

// findChapterMarker 在字符串中查找「第N章/节/回」或特殊标记的起始下标；找不到返回 -1。
func findChapterMarker(s string) int {
	for i := 0; i < len(s); {
		if s[i] == 0xE7 { // 「第」的 UTF-8 首字节为 E7 AC AC
			if strings.HasPrefix(s[i:], "第") {
				r := s[i+len("第"):]
				if _, n, ok := parseNumeral(r); ok {
					if isChapterUnitAt(r, n) || isVolumeUnitAt(r, n) {
						return i
					}
				}
			}
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
	}
	return -1
}

// cutLeadingNumber 解析行首数字，返回数值、数字原文与剩余部分。
func cutLeadingNumber(s string) (num int, numText string, rest string, ok bool) {
	v, n, valid := parseNumeral(s)
	if !valid || n == 0 {
		return 0, "", "", false
	}
	return v, s[:n], s[n:], true
}
