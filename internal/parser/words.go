package parser

import "unicode"

// isCJK 判断是否为需要逐字计数的表意文字（汉字/假名/谚文）。
func isCJK(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: // CJK 统一表意文字
		return true
	case r >= 0x3400 && r <= 0x4DBF: // 扩展 A
		return true
	case r >= 0x20000 && r <= 0x3FFFF: // 扩展 B 及以后
		return true
	case r >= 0xF900 && r <= 0xFAFF: // 兼容表意文字
		return true
	case r >= 0x3040 && r <= 0x30FF: // 平假名/片假名
		return true
	case r >= 0xAC00 && r <= 0xD7AF: // 谚文音节
		return true
	}
	return false
}

// countWords 统计字数：CJK 字符逐字计 1，连续的拉丁字母/数字串计 1。
func countWords(s string) int {
	n := 0
	inWord := false
	for _, r := range s {
		switch {
		case r < 0x80:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				if !inWord {
					n++
					inWord = true
				}
				continue
			}
			inWord = false
		case isCJK(r):
			n++
			inWord = false
		default:
			// 其它语言的字母（如俄文）按词计
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				if !inWord {
					n++
					inWord = true
				}
				continue
			}
			inWord = false
		}
	}
	return n
}
