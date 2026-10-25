package parser

import (
	"regexp"
	"strings"
	"testing"

	"norvels/internal/model"
)

// ruleSampleText 造一本「内置策略切不开、自定义 ^=== 第N 节 ===$ 能切开」的合成书。
func ruleSampleText() string {
	var b strings.Builder
	b.WriteString("书名：合成试切\n作者：青川客\n\n")
	for i := 1; i <= 3; i++ {
		b.WriteString("=== 第" + string(rune('0'+i)) + " 节 ===\n")
		b.WriteString("风起于青萍之末，合成段落用于验证自定义规则的切章结果。\n")
		b.WriteString("这一段正文没有任何内置策略认识的标题形态。\n\n")
	}
	return b.String()
}

// sectionRule 是样本书的自定义规则。
func sectionRule() Rule {
	return Rule{Name: "节标记", Pattern: regexp.MustCompile("^=== 第\\d+ 节 ===$")}
}

// TestParseBytesWithRulesCustomSplit 覆盖自定义正则切开内置切不开的书。
func TestParseBytesWithRulesCustomSplit(t *testing.T) {
	text := []byte(ruleSampleText())
	plain, err := New(DefaultOptions()).ParseBytes("合成试切.txt", text)
	if err != nil {
		t.Fatalf("内置解析: %v", err)
	}
	if len(plain.Chapters) != 1 {
		t.Fatalf("内置策略章数 = %d，期望 1（这个样本本应切不开）", len(plain.Chapters))
	}

	book, err := New(DefaultOptions()).ParseBytesWithRules("合成试切.txt", text, []Rule{sectionRule()})
	if err != nil {
		t.Fatalf("自定义规则解析: %v", err)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("自定义规则章数 = %d，期望 3（%v）", len(book.Chapters), chapterTitles(book.Chapters))
	}
	if got := book.Chapters[1].Title; got != "=== 第2 节 ===" {
		t.Errorf("第 2 章标题 = %q，期望命中行原文", got)
	}
	if !strings.Contains(book.Chapters[1].Content, "合成段落") {
		t.Errorf("第 2 章正文丢了：%q", book.Chapters[1].Content)
	}
}

// TestParseBytesWithRulesZeroHitFallsBack 覆盖零命中退回内置并记 warning。
func TestParseBytesWithRulesZeroHitFallsBack(t *testing.T) {
	text := []byte(ruleSampleText())
	plain, err := New(DefaultOptions()).ParseBytes("合成试切.txt", text)
	if err != nil {
		t.Fatalf("内置解析: %v", err)
	}
	p := New(DefaultOptions())
	zero := Rule{Name: "零命中规则", Pattern: regexp.MustCompile("^@@@ \\d+ @@@$")}
	book, err := p.ParseBytesWithRules("合成试切.txt", text, []Rule{zero, sectionRule()})
	if err != nil {
		t.Fatalf("解析: %v", err)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("第二条规则章数 = %d，期望 3", len(book.Chapters))
	}
	if !warningsContainAll(book.Warnings, "零命中规则", "退回内置策略") {
		t.Errorf("零命中规则未记 warning：%v", book.Warnings)
	}

	// 单条规则全部零命中：章节结构退回内置策略。
	only := Rule{Name: "全零规则", Pattern: regexp.MustCompile("^@@@ \\d+ @@@$")}
	fell, err := p.ParseBytesWithRules("合成试切.txt", text, []Rule{only})
	if err != nil {
		t.Fatalf("解析: %v", err)
	}
	if len(fell.Chapters) != len(plain.Chapters) {
		t.Errorf("零命中后章数 = %d，期望退回内置的 %d", len(fell.Chapters), len(plain.Chapters))
	}
	if !warningsContainAll(fell.Warnings, "全零规则", "退回内置策略") {
		t.Errorf("零命中规则未记 warning：%v", fell.Warnings)
	}
}

// TestParseBytesWithRulesNilPattern 覆盖 nil 正则：不 panic、记 warning、继续下一条。
func TestParseBytesWithRulesNilPattern(t *testing.T) {
	rules := []Rule{{Name: "空正则"}, sectionRule()}
	book, err := New(DefaultOptions()).ParseBytesWithRules("合成试切.txt", []byte(ruleSampleText()), rules)
	if err != nil {
		t.Fatalf("解析: %v", err)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("章数 = %d，期望 3", len(book.Chapters))
	}
	if !warningsContainAll(book.Warnings, "空正则", "缺少正则") {
		t.Errorf("nil 正则未记 warning：%v", book.Warnings)
	}
}

// TestParseBytesWithRulesPackageEntry 覆盖无文件名的试切入口。
func TestParseBytesWithRulesPackageEntry(t *testing.T) {
	book, err := ParseBytesWithRules([]byte(ruleSampleText()), []Rule{sectionRule()})
	if err != nil {
		t.Fatalf("试切入口: %v", err)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("章数 = %d，期望 3", len(book.Chapters))
	}
}

// hasWarning 判断 warnings 里是否存在同时包含全部关键词的一条。
func warningsContainAll(warnings []string, keys ...string) bool {
	for _, w := range warnings {
		hit := true
		for _, k := range keys {
			if !strings.Contains(w, k) {
				hit = false
				break
			}
		}
		if hit {
			return true
		}
	}
	return false
}

// chapterTitles 抽出章节标题（用于失败时的可读输出）。
func chapterTitles(chs []model.ParsedChapter) []string {
	out := make([]string, 0, len(chs))
	for _, c := range chs {
		out = append(out, c.Title)
	}
	return out
}
