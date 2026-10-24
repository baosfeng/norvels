package store

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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

// isCJK 判断是否是需要逐字分词的 CJK 字符（汉字、假名、谚文）。
func isCJK(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: // CJK 统一表意文字
		return true
	case r >= 0x3400 && r <= 0x4DBF: // 扩展 A
		return true
	case r >= 0xF900 && r <= 0xFAFF: // 兼容表意文字
		return true
	case r >= 0x3040 && r <= 0x30FF: // 日文假名
		return true
	case r >= 0xAC00 && r <= 0xD7AF: // 谚文
		return true
	}
	return false
}

// Segment 把文本转换为 FTS5 unicode61 可用的「字级分词」串：
// CJK 字符逐字输出并在其后追加一个空格，其余字符原样输出（不额外加空格）。
//
// 例：Segment("华夏国，午后") == "华 夏 国 ，午 后 "
//
// 该函数同时供写入（镜像列）与查询（构造 phrase）使用，必须保持确定性。
func Segment(s string) string {
	if s == "" {
		return ""
	}
	n := len(s)
	for _, r := range s {
		if isCJK(r) {
			n++
		}
	}
	buf := make([]byte, 0, n)
	for _, r := range s {
		buf = utf8.AppendRune(buf, r)
		if isCJK(r) {
			buf = append(buf, ' ')
		}
	}
	return string(buf)
}

// Unsegment 把 Segment 生成的串还原为原文：删除每个 CJK 字符后紧跟的那个空格。
// 用于把 FTS snippet() 的结果转回可读文本。
func Unsegment(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	buf := make([]byte, 0, len(s))
	for i := 0; i < len(runes); i++ {
		buf = utf8.AppendRune(buf, runes[i])
		if isCJK(runes[i]) && i+1 < len(runes) && runes[i+1] == ' ' {
			i++
		}
	}
	return string(buf)
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

// htmlEscaper 转义文本节点中的 & < >，用于 snippet 输出。
var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

func escapeHTML(s string) string { return htmlEscaper.Replace(s) }

// markOpen / markClose 是 snippet() 的高亮占位符（U+0001 / U+0002），
// 转义为 HTML 后再替换成 <b>，从而保证其余内容全部被转义。
var (
	markOpen  = string(rune(1))
	markClose = string(rune(2))
)

// renderSnippet 把 snippet() 返回的片段（占位符高亮）转义为只含 <b> 的安全 HTML。
func renderSnippet(seg string) string {
	if seg == "" {
		return ""
	}
	out := escapeHTML(Unsegment(seg))
	out = strings.ReplaceAll(out, markOpen, "<b>")
	out = strings.ReplaceAll(out, markClose, "</b>")
	return out
}
