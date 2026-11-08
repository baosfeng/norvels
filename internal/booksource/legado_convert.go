package booksource

// Legado 规则块的导入期转换：把原生简写翻译成标准 CSS，并汇总降级字段。
// 转换函数本身在 internal/source（纯函数、无 IO）；这里只负责按规则块批量应用。

import (
	"sort"

	"norvels/internal/source"
)

// convertLegadoRules 把五个规则块里的 Legado 简写翻译成标准 CSS（导入时转换一次）。
//
// 返回降级字段（块.字段）与原因：转换失败的字段保持原文，只影响该字段能否抽出数据，
// 不阻断整条书源入库——分享站的书源包必然混着写法各异的规则，一条坏了不该整源作废。
func convertLegadoRules(ls *legadoSource) (names, reasons []string) {
	convert := func(prefix string, rules *map[string]string) {
		conv, degraded := source.ConvertLegadoRules(*rules)
		*rules = conv
		for field, why := range degraded {
			name := prefix + "." + field
			names = append(names, name)
			reasons = append(reasons, name+"（"+why+"）")
		}
	}
	convert("ruleSearch", &ls.RuleSearch)
	convert("ruleExplore", &ls.RuleExplore)
	convert("ruleBookInfo", &ls.RuleBookInfo)
	convert("ruleToc", &ls.RuleToc)
	convert("ruleContent", &ls.RuleContent)
	sort.Strings(names)
	sort.Strings(reasons)
	return names, reasons
}

// mergeNames 合并两组字段名并稳定排序去重（@js 降级与简写降级可能同时出现）。
func mergeNames(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	seen := make(map[string]bool, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, name := range list {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// joinReason 拼接两句原因，空串忽略。
func joinReason(first, second string) string {
	switch {
	case first == "":
		return second
	case second == "":
		return first
	default:
		return first + "；" + second
	}
}
