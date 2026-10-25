package parser

import (
	"regexp"
	"strings"

	"norvels/internal/model"
)

// Rule 是调用方注入的自定义章节识别规则。
//
// 语义：正文逐行与 Pattern 匹配（一行一判定，^ $ 锚定在行首行尾）；命中的行作为
// 章节边界，该行原文即章节标题。规则按数组顺序尝试，第一条命中的规则生效，
// 优先于内置切章策略；某条规则一章都没切出时记 warning 并继续下一条，全部落空
// 才退回内置策略。
//
// 未定义行为在此写死：
//   - Pattern 为 nil 的规则被跳过并记 warning，不会 panic；
//   - 单行超过 maxRuleLineBytes 不参与匹配（避免把整段正文当成标题）；
//   - 规则只按行切分，正文里没有换行的超长行不会命中任何带 ^ $ 的规则。
type Rule struct {
	Name    string
	Pattern *regexp.Regexp
}

// maxRuleLineBytes 是自定义规则参与匹配的单行字节上限（与标题行上限同量级）。
const maxRuleLineBytes = 400

// ParseBytesWithRules 用默认配置解析一段内存文本并套用自定义规则。
//
// 没有真实文件名的调用方（试切预览）用这个入口：name 固定为「预览.txt」，
// 只影响书名推断，不影响切章。需要按真实文件名推断书名时用
// (*Parser).ParseBytesWithRules。
func ParseBytesWithRules(data []byte, rules []Rule) (*model.ParsedBook, error) {
	return New(DefaultOptions()).ParseBytesWithRules("预览.txt", data, rules)
}

// runRuleSplitter 按顺序尝试自定义规则，命中一条即返回 true（不再跑内置策略）。
func runRuleSplitter(d *Doc, rules []Rule) bool {
	for _, r := range rules {
		if d.splitByRule(r) {
			return true
		}
	}
	return false
}

// splitByRule 用一条规则切章；零命中时记 warning 并返回 false 交给下一条规则。
func (d *Doc) splitByRule(r Rule) bool {
	name := ruleLabel(r)
	if r.Pattern == nil {
		d.Warn("自定义切章规则 %q 缺少正则，已忽略", name)
		return false
	}
	cands := ruleCandidates(d.Text, r.Pattern)
	if len(cands) == 0 {
		d.Warn("自定义切章规则 %q 零命中，已退回内置策略", name)
		return false
	}
	clearSplit(d)
	res := buildFromCandidates(d.Text, cands, d.cleanerOr())
	d.Volumes, d.Chapters = res.volumes, res.chapters
	d.restarts = res.restarts
	if res.preamble != "" {
		d.pre = res.preamble
	}
	d.Warnings = append(d.Warnings, res.warnings...)
	if !hasChapters(d) {
		clearSplit(d)
		d.Warn("自定义切章规则 %q 命中 %d 处但未切出章节，已退回内置策略", name, len(cands))
		return false
	}
	d.keepPreambleAsChapter(res.preamble)
	d.Stat("splitter", "custom:"+name)
	d.Stat("rule_matches", len(cands))
	return true
}

// ruleCandidates 返回规则命中的章节边界（一行一个候选）。
//
// 行尾的 \r 先剥掉再匹配，这样 ^...$ 在 CRLF 文本上同样成立。
func ruleCandidates(text string, re *regexp.Regexp) []candidate {
	var out []candidate
	start := 0
	for i := 0; i <= len(text); i++ {
		if i != len(text) && text[i] != '\n' {
			continue
		}
		line := strings.TrimSpace(text[start:i])
		bodyStart := min(i+1, len(text))
		if line != "" && len(line) <= maxRuleLineBytes && re.MatchString(line) {
			out = append(out, candidate{start: start, bodyStart: bodyStart,
				h: heading{kind: headChapter, strong: true, marker: line}})
		}
		start = i + 1
	}
	return out
}

// ruleLabel 返回规则的展示名（未命名规则用占位名，保证 warning 可定位）。
func ruleLabel(r Rule) string {
	if name := strings.TrimSpace(r.Name); name != "" {
		return name
	}
	return "未命名规则"
}
