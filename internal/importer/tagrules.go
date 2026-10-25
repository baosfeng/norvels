package importer

import (
	"strings"
	"unicode"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 词表匹配口径（P1-B §2.4）。
const (
	// tagMatchHeadRunes 是参与匹配的正文前缀长度（按字符计）。
	tagMatchHeadRunes = 5000
	// tagMatchChapterTitles 是参与匹配的章节标题条数（前 N 章）。
	tagMatchChapterTitles = 3
	// maxAutoTags 是单书自动标签上限（同一标签命中多个关键词只打一次）。
	maxAutoTags = 10
)

// matchTags 按词表规则算一本书的自动标签（规则顺序无关，取并集）。
//
// 匹配文本 = 书名 + 作者 + 前 3 章标题 + 正文前 5000 字；两边都去空白后小写比较，
// 关键词是子串包含匹配（不做正则、不做分词）。
func matchTags(rules []store.TagRule, b *model.ParsedBook) []string {
	if len(rules) == 0 || b == nil {
		return nil
	}
	text := normalizeTagText(bookMatchText(b))
	out := make([]string, 0, 4)
	seen := make(map[string]bool, 4)
	for _, rule := range rules {
		tag := strings.TrimSpace(rule.Tag)
		if tag == "" || seen[tag] {
			continue
		}
		if ruleMatched(rule, text) {
			seen[tag] = true
			out = append(out, tag)
		}
		if len(out) >= maxAutoTags {
			break
		}
	}
	return out
}

// ruleMatched 判断一条规则的任一关键词是否命中（命中即整条规则生效）。
func ruleMatched(rule store.TagRule, text string) bool {
	for _, kw := range rule.Keywords {
		key := normalizeTagText(kw)
		if key != "" && strings.Contains(text, key) {
			return true
		}
	}
	return false
}

// bookMatchText 拼接参与匹配的文本（书名 + 作者 + 前 3 章标题 + 正文前 5000 字）。
func bookMatchText(b *model.ParsedBook) string {
	var sb strings.Builder
	sb.WriteString(b.Title)
	sb.WriteString(b.Author)
	for i, ch := range b.Chapters {
		if i >= tagMatchChapterTitles {
			break
		}
		sb.WriteString(ch.Title)
	}
	sb.WriteString(headContent(b, tagMatchHeadRunes))
	return sb.String()
}

// headContent 取正文前 n 个字符（跨章拼接，不切半个 rune）。
func headContent(b *model.ParsedBook, n int) string {
	var sb strings.Builder
	count := 0
	for _, ch := range b.Chapters {
		need := n - count
		if need <= 0 {
			break
		}
		rs := []rune(ch.Content)
		if len(rs) > need {
			rs = rs[:need]
		}
		sb.WriteString(string(rs))
		count += len(rs)
	}
	return sb.String()
}

// normalizeTagText 去掉全部空白并转小写（中英文都适用的比较口径）。
func normalizeTagText(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		sb.WriteRune(unicode.ToLower(r))
	}
	return sb.String()
}
