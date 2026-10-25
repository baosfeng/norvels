package store

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// timeLayout 是库内时间字段统一使用的 UTC 文本格式，与 schema.sql 的 strftime 默认值一致。
const timeLayout = "2006-01-02T15:04:05.000Z"

func nowUTC() time.Time { return time.Now().UTC() }

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

// parseTime 解析库内时间文本，兼容历史/手工写入的几种常见格式。
func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	layouts := []string{timeLayout, time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("store: bad time %q", s)
}

// normPage 归一化分页参数。
func normPage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	return page, size
}

// escapeLike 转义 LIKE 通配符，配合 SQL 里的 ESCAPE '!' 使用。
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "!", "!!")
	s = strings.ReplaceAll(s, "%", "!%")
	s = strings.ReplaceAll(s, "_", "!_")
	return s
}

// cjkRanges 是「按字计数」的字符区间（含端点）：
// 0x4E00-0x9FFF CJK 统一表意文字、0x3400-0x4DBF 扩展 A、0xF900-0xFAFF 兼容表意文字、
// 0x3040-0x30FF 日文假名、0xAC00-0xD7AF 谚文。
var cjkRanges = [...][2]rune{
	{0x4E00, 0x9FFF},
	{0x3400, 0x4DBF},
	{0xF900, 0xFAFF},
	{0x3040, 0x30FF},
	{0xAC00, 0xD7AF},
}

// isCJK 判断是否是需要逐字计数的 CJK 字符（汉字、假名、谚文）。
// 区间集中放在 cjkRanges，新增区系只改表不动判定逻辑。
func isCJK(r rune) bool {
	for _, rg := range cjkRanges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

// countWords 兜底字数统计：CJK 字符逐字计 1，连续非 CJK 词计 1。
func countWords(s string) int {
	n := 0
	inWord := false
	for _, r := range s {
		switch {
		case isCJK(r):
			n++
			inWord = false
		case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsControl(r):
			inWord = false
		default:
			if !inWord {
				n++
				inWord = true
			}
		}
	}
	return n
}
