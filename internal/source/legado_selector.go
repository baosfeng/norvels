package source

// Legado 原生选择器简写 → 标准 CSS 的转换器。
//
// 背景：Legado 书源的默认规则不是 CSS，而是「类型.名称.位置」的链式简写，例如
// class.grid@tr!0、tag.td.0@tag.a@text。ruleexec 只认标准 CSS：它的 peelSuffix 只剥
// 最后一个 @ 后缀，剩下的 tag.td.0@tag.a 交给 goquery 匹配为空，于是「导入 ok、实际
// 抽不出数据」。本文件在导入 / 加载书源时把简写翻译成等价的标准 CSS，求值器本身不改。
//
// 口径（与 Legado 文档一致；个别处按 goquery 的选择器能力近似，见下）：
//   - 基本选择器：tag.x → x；class.x → .x；id.x → #x；不带前缀的段按原样 CSS 保留；
//   - 索引 .N（0 起）：.0 省略——ruleexec 取值本就取 First()，与「匹配集第 0 个」等价，
//     且不会因为首元素不是首个子节点而选空；.N(N>0) → :nth-child(N+1)；
//     .-M → :nth-last-child(M)；
//   - 排除 !N：!0 → :not(:nth-child(1))，!N → :not(:nth-child(N+1))，
//     !-M → :not(:nth-last-child(M))；CSS 无法表达「匹配集里的第 N 个」，nth-child
//     按兄弟位置近似——对同一父节点下的表行 / 列表项（书源最常见的结构）与原语义一致；
//   - 分隔符 @：逐段用空格拼接（在前一段命中的元素内找后代）；
//   - 后缀：最后一段是 text / ownText / html 或属性名（与 ruleexec.peelSuffix 同口径）
//     时保留 @ 原样交给 ruleexec；最后一段是裸 HTML 标签名（a / li / span…）时按选择器
//     处理（Legado 的「最后一段为获取内容」不含标签名，id.x@li@a 的 a 是链接元素）；
//   - @css: / @xpath: / @json: / @js: 前缀与 ##正则##替换 段原样保留；
//   - 无法安全转换的写法（列表倒置 - 前缀、children.、text. 等）原样返回并给出降级
//     说明：宁可不转换，也不要把规则改坏、让整个源不可用。

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// 索引 / 排除的尾部写法：.0、.12、.-1；!0、!0:2、!-1。
var (
	reLegadoIndex   = regexp.MustCompile(`\.(-?\d+)$`)
	reLegadoExclude = regexp.MustCompile(`!(-?\d+(?::-?\d+)*)$`)
)

// enginePrefixes 是 ruleexec 原生前缀：带这些前缀的规则不归 Legado 简写管。
var enginePrefixes = []string{"@css:", "@xpath:", "@json:", "@js:"}

// ConvertLegadoSelector 把一条 Legado 简写规则翻译成 ruleexec 可求值的标准 CSS。
//
// 第二个返回值是降级说明：非空表示规则里有无法安全转换的写法，此时第一个返回值是
// 原文（调用方可据此记一条可见的降级信息）；空串表示已转换，或本来就不需要转换。
// 纯函数：不读环境、不写文件、不报错。
func ConvertLegadoSelector(rule string) (string, string) {
	s := strings.TrimSpace(rule)
	if s == "" || hasEnginePrefix(s) || notSelectorRule(s) {
		return rule, ""
	}
	conv, err := convertRuleString(s)
	if err != nil {
		return rule, err.Error()
	}
	return conv, ""
}

// ConvertLegadoRules 批量转换一组规则（字段 → 规则串），返回新 map 与降级说明
// （字段 → 原因）；降级字段的规则保持原文。空 map 原样返回 nil 降级表。
func ConvertLegadoRules(rules map[string]string) (map[string]string, map[string]string) {
	if len(rules) == 0 {
		return rules, nil
	}
	out := make(map[string]string, len(rules))
	var degraded map[string]string
	for field, raw := range rules {
		conv, warn := ConvertLegadoSelector(raw)
		out[field] = conv
		if warn == "" {
			continue
		}
		if degraded == nil {
			degraded = make(map[string]string, 1)
		}
		degraded[field] = warn
	}
	return out, degraded
}

// convertRuleString 按 ruleexec 的分隔层级逐段转换：|| / && 切分支，| 切管道，
// 每个分支只转换选择器头部；## 正则段原样保留。
func convertRuleString(s string) (string, error) {
	var b strings.Builder
	start := 0
	inRe := false
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "##") {
			inRe = !inRe
			i += 2
			continue
		}
		if !inRe && (strings.HasPrefix(s[i:], "||") || strings.HasPrefix(s[i:], "&&")) {
			seg, err := convertSegment(s[start:i])
			if err != nil {
				return "", err
			}
			b.WriteString(seg)
			b.WriteString(s[i : i+2])
			i += 2
			start = i
			continue
		}
		i++
	}
	seg, err := convertSegment(s[start:])
	if err != nil {
		return "", err
	}
	b.WriteString(seg)
	return b.String(), nil
}

// convertSegment 转换一个 && 分支：只改选择器头部，管道调用原样保留。
func convertSegment(seg string) (string, error) {
	cut := topPipeIndex(seg)
	if cut < 0 {
		head, changed, err := convertHead(seg)
		if err != nil || !changed {
			return seg, err
		}
		return head, nil
	}
	// 把管道前的空白留在尾部，转换后仍保持「选择器 | 管道」的可读形态。
	for cut > 0 && (seg[cut-1] == ' ' || seg[cut-1] == '\t') {
		cut--
	}
	head, changed, err := convertHead(seg[:cut])
	if err != nil || !changed {
		return seg, err
	}
	return head + seg[cut:], nil
}

// convertHead 转换「选择器 + 后缀 + ##正则」里的选择器部分；changed=false 表示原文可用。
func convertHead(head string) (string, bool, error) {
	trimmed := strings.TrimSpace(head)
	if trimmed == "" || hasEnginePrefix(trimmed) || notSelectorRule(trimmed) || !looksLegado(trimmed) {
		return head, false, nil
	}
	sel, regexTail := trimmed, ""
	if i := strings.Index(sel, "##"); i >= 0 {
		regexTail = sel[i:]
		sel = strings.TrimSpace(sel[:i])
	}
	if sel == "" {
		return head, false, nil
	}
	conv, err := convertChain(sel)
	if err != nil {
		return "", false, err
	}
	return conv + regexTail, true, nil
}

// convertChain 转换一条 @ 链：逐段选择器用空格拼接，末尾后缀保留 @。
func convertChain(s string) (string, error) {
	if strings.HasPrefix(s, "-") {
		return "", errors.New("暂不支持列表倒置前缀 -")
	}
	parts := strings.Split(s, "@")
	if len(parts) > 0 && strings.TrimSpace(parts[0]) == "" {
		parts = parts[1:] // 前导 @ 表示「从当前节点向下找」
	}
	if len(parts) == 0 {
		return "", errors.New("缺少选择器")
	}
	suffix := ""
	if len(parts) > 1 && isLegadoSuffix(parts[len(parts)-1]) {
		suffix = strings.TrimSpace(parts[len(parts)-1])
		parts = parts[:len(parts)-1]
	}
	segs := make([]string, 0, len(parts))
	for _, part := range parts {
		sel, err := convertPart(part)
		if err != nil {
			return "", err
		}
		segs = append(segs, sel)
	}
	out := strings.Join(segs, " ")
	if suffix != "" {
		out += "@" + suffix
	}
	return out, nil
}

// convertPart 转换单个 @ 段：基本选择器 + 尾部索引 / 排除。
func convertPart(part string) (string, error) {
	p := strings.TrimSpace(part)
	if p == "" {
		return "", errors.New("空的 @ 段")
	}
	if strings.HasPrefix(p, "children.") || strings.HasPrefix(p, "text.") {
		return "", fmt.Errorf("暂不支持的 Legado 类型 %q", strings.SplitN(p, ".", 2)[0])
	}
	base, dot, excl, err := peelIndexes(p)
	if err != nil {
		return "", err
	}
	sel, err := baseSelector(base)
	if err != nil {
		return "", err
	}
	sel += dotPseudo(dot)
	for _, n := range excl {
		sel += excludePseudo(n)
	}
	return sel, nil
}

// baseSelector 把 tag./class./id. 前缀换成 CSS；不带前缀的段按原样 CSS 保留。
func baseSelector(base string) (string, error) {
	switch {
	case strings.HasPrefix(base, "tag."):
		return checkedName(base, "tag.")
	case strings.HasPrefix(base, "class."):
		name, err := checkedName(base, "class.")
		return "." + name, err
	case strings.HasPrefix(base, "id."):
		name, err := checkedName(base, "id.")
		return "#" + name, err
	default:
		if strings.TrimSpace(base) == "" {
			return "", errors.New("缺少选择器")
		}
		return base, nil
	}
}

// checkedName 校验并返回前缀后的名字：只允许字母 / 数字 / 下划线 / 连字符，
// 名字不合法时宁可降级退回原文，也不产出无法求值的 CSS。
func checkedName(base, prefix string) (string, error) {
	name := base[len(prefix):]
	if !isSelectorName(name) {
		return "", fmt.Errorf("非法的 %s 名 %q", strings.TrimSuffix(prefix, "."), name)
	}
	return name, nil
}

// isSelectorName 判断名字能否直接拼进 CSS（字母 / 数字 / _ / -，不以数字开头）。
func isSelectorName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || r == '-' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			if i == 0 && unicode.IsDigit(r) {
				return false
			}
			continue
		}
		return false
	}
	return true
}

// peelIndexes 从段尾反复剥掉 .N / !N 索引，返回剩余选择器与解析出的索引。
func peelIndexes(s string) (string, *int, []int, error) {
	base := s
	var dot *int
	var excl []int
	for {
		if m := reLegadoExclude.FindStringSubmatch(base); m != nil {
			vals, err := parseExcludes(m[1])
			if err != nil {
				return "", nil, nil, err
			}
			excl = append(excl, vals...)
			base = base[:len(base)-len(m[0])]
			continue
		}
		if m := reLegadoIndex.FindStringSubmatch(base); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				return "", nil, nil, fmt.Errorf("非法的索引 %q", m[1])
			}
			if dot != nil {
				return "", nil, nil, errors.New("一条规则里出现多个正索引")
			}
			dot = &n
			base = base[:len(base)-len(m[0])]
			continue
		}
		return base, dot, excl, nil
	}
}

// parseExcludes 解析 !0:2 形式的多项排除。
func parseExcludes(raw string) ([]int, error) {
	out := make([]int, 0, 2)
	for _, tok := range strings.Split(raw, ":") {
		n, err := strconv.Atoi(tok)
		if err != nil {
			return nil, fmt.Errorf("非法的排除索引 %q", raw)
		}
		out = append(out, n)
	}
	return out, nil
}

// dotPseudo 把正索引映射成位置伪类；.0 省略（等价于 ruleexec 的 First()）。
func dotPseudo(n *int) string {
	if n == nil || *n == 0 {
		return ""
	}
	if *n > 0 {
		return fmt.Sprintf(":nth-child(%d)", *n+1)
	}
	return fmt.Sprintf(":nth-last-child(%d)", -*n)
}

// excludePseudo 把排除索引映射成 :not(...)；负索引从末尾数。
func excludePseudo(n int) string {
	if n >= 0 {
		return fmt.Sprintf(":not(:nth-child(%d))", n+1)
	}
	return fmt.Sprintf(":not(:nth-last-child(%d))", -n)
}

// hasEnginePrefix 判断规则是否以 ruleexec 原生前缀开头（与 ruleexec 同口径，大小写不敏感）。
func hasEnginePrefix(s string) bool {
	for _, p := range enginePrefixes {
		if len(s) >= len(p) && strings.EqualFold(s[:len(p)], p) {
			return true
		}
	}
	return false
}

// topPipeIndex 返回第一个不在 ## 正则段内的管道分隔符位置（|| 已在上一层切开）。
func topPipeIndex(s string) int {
	inRe := false
	for i := 0; i < len(s); i++ {
		if strings.HasPrefix(s[i:], "##") {
			inRe = !inRe
			i++
			continue
		}
		if !inRe && s[i] == '|' {
			return i
		}
	}
	return -1
}
