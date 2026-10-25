package importer

import (
	"unicode"

	"norvels/internal/model"
)

// totalWords 汇总全文字数。
func totalWords(chapters []model.ParsedChapter) int {
	total := 0
	for _, c := range chapters {
		total += c.WordCount
	}
	return total
}

// countTextWords 统计纯文本字数：CJK 逐字计 1，连续拉丁字母/数字计 1 个词。
// 与 parser 的字数口径一致，保证 EPUB 与 TXT 的统计数据可比。
func countTextWords(s string) int {
	n, inWord := 0, false
	for _, r := range s {
		switch {
		case isCJKRune(r):
			n++
			inWord = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inWord {
				n++
				inWord = true
			}
		default:
			inWord = false
		}
	}
	return n
}

// isCJKRune 判断是否为需要逐字计数的表意文字（汉字/假名/谚文）。
func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= 0x3040 && r <= 0x30FF) ||
		(r >= 0xAC00 && r <= 0xD7AF)
}
