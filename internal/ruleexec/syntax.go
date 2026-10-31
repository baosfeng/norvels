package ruleexec

import (
	"fmt"
	"regexp"
	"strings"
)

// parseSegment 解析一段（&& 内的一段）：前缀 → 选择器 → 后缀 → ## 正则 → | 管道。
func parseSegment(text string) (segment, error) {
	seg := segment{prefix: "css"}
	s := strings.TrimSpace(text)
	for _, p := range []string{"@css:", "@xpath:", "@json:", "@js:"} {
		if len(s) >= len(p) && strings.EqualFold(s[:len(p)], p) {
			seg.prefix = strings.TrimSuffix(p[1:], ":")
			s = strings.TrimSpace(s[len(p):])
			break
		}
	}
	parts := splitTop(s, "|")
	head := strings.TrimSpace(parts[0])
	if isPipeCall(head) {
		// 纯管道规则（如 getvar(name) | prefix(...)）：没有选择器，从上下文变量起步。
		call, err := parsePipe(head)
		if err != nil {
			return seg, err
		}
		seg.pipes = append(seg.pipes, call)
	} else {
		query, err := parseQuery(head, seg.prefix)
		if err != nil {
			return seg, err
		}
		seg.query, seg.suffix, seg.regex = query.query, query.suffix, query.regex
	}
	for _, p := range parts[1:] {
		call, err := parsePipe(strings.TrimSpace(p))
		if err != nil {
			return seg, err
		}
		if call.name != "" {
			seg.pipes = append(seg.pipes, call)
		}
	}
	return seg, nil
}

// isPipeCall 判断一段文本是否形如 fn(...) 且 fn 在白名单里。
func isPipeCall(s string) bool {
	if !strings.HasSuffix(s, ")") {
		return false
	}
	open := strings.Index(s, "(")
	if open <= 0 {
		return false
	}
	_, ok := pipeFuncs[strings.TrimSpace(s[:open])]
	return ok
}

// queryParts 是选择器段的解析结果。
type queryParts struct {
	query  string
	suffix suffix
	regex  []replacement
}

// parseQuery 拆出选择器、后缀与 ## 正则替换；正则在此处预编译校验（坏正则编译期报错）。
func parseQuery(text, prefix string) (queryParts, error) {
	q := queryParts{suffix: suffix{kind: "text"}}
	parts := strings.Split(strings.TrimSpace(text), "##")
	q.query = strings.TrimSpace(parts[0])
	for i := 1; i+1 < len(parts); i += 2 {
		rep := replacement{pattern: parts[i], repl: parts[i+1]}
		if _, err := compileRegex(rep.pattern); err != nil {
			return q, err
		}
		q.regex = append(q.regex, rep)
	}
	if q.query == "" {
		return q, fmt.Errorf("ruleexec: 规则缺少选择器")
	}
	q.query, q.suffix = peelSuffix(q.query, prefix)
	return q, nil
}

// peelSuffix 从选择器末尾剥离 @text / @ownText / @html / @attr。
func peelSuffix(query, prefix string) (string, suffix) {
	idx := strings.LastIndex(query, "@")
	if idx < 0 {
		return query, suffix{kind: "text"}
	}
	tail := query[idx+1:]
	base := strings.TrimSpace(query[:idx])
	switch {
	case strings.EqualFold(tail, "text"):
		return base, suffix{kind: "text"}
	case strings.EqualFold(tail, "ownText"):
		return base, suffix{kind: "ownText"}
	case strings.EqualFold(tail, "html"):
		return base, suffix{kind: "html"}
	}
	// XPath 自带 @attr 语法，不做通用属性后缀，避免把 //a/@href 拆坏。
	if prefix == "xpath" || prefix == "json" || !isAttrName(tail) {
		return query, suffix{kind: "text"}
	}
	return base, suffix{kind: "attr", arg: tail}
}

// reAttrName 匹配合法属性名（用于把 \".a@href\" 的 href 识别成属性后缀）。
var reAttrName = regexp.MustCompile("^[A-Za-z0-9:_-]+$")

func isAttrName(s string) bool {
	return s != "" && reAttrName.MatchString(s)
}

// parsePipe 解析一次管道调用：fn(a, b) / fn(/re/, "x")。
func parsePipe(text string) (pipeCall, error) {
	if text == "" {
		return pipeCall{}, nil
	}
	open := strings.Index(text, "(")
	if open < 0 || !strings.HasSuffix(text, ")") {
		return pipeCall{}, fmt.Errorf("ruleexec: 管道调用缺少括号: %q", text)
	}
	name := strings.TrimSpace(text[:open])
	if _, ok := pipeFuncs[name]; !ok {
		return pipeCall{}, fmt.Errorf("ruleexec: 未知管道函数 %q", name)
	}
	args, err := splitArgs(text[open+1 : len(text)-1])
	if err != nil {
		return pipeCall{}, err
	}
	if err := validateRegexArgs(args); err != nil {
		return pipeCall{}, err
	}
	return pipeCall{name: name, args: args}, nil
}

// validateRegexArgs 在编译期校验 /re/ 形式的参数。
func validateRegexArgs(args []string) error {
	for _, a := range args {
		if !strings.HasPrefix(a, "/") || !strings.HasSuffix(a, "/") || len(a) <= 1 {
			continue
		}
		if _, err := compileRegex(strings.Trim(a, "/")); err != nil {
			return err
		}
	}
	return nil
}

// splitArgs 按逗号切参数，引号与 /regex/ 内的逗号不切。
func splitArgs(s string) ([]string, error) {
	cuts, err := argCuts(s)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(cuts)+1)
	start := 0
	for _, c := range cuts {
		out = append(out, strings.TrimSpace(s[start:c]))
		start = c + 1
	}
	if tail := strings.TrimSpace(s[start:]); tail != "" {
		out = append(out, tail)
	}
	return out, nil
}

// argCuts 返回顶层逗号位置。
func argCuts(s string) ([]int, error) {
	var cuts []int
	st := argState{}
	for i := 0; i < len(s); i++ {
		if st.consume(s[i]) {
			cuts = append(cuts, i)
		}
	}
	if st.quote || st.re {
		return nil, fmt.Errorf("ruleexec: 管道参数引号或正则未闭合")
	}
	return cuts, nil
}

// argState 跟踪参数扫描时的引号 / 正则状态。
type argState struct {
	quote bool
	re    bool
}

// consume 吃掉一个字符，返回它是否是「此刻的顶层逗号」。
func (a *argState) consume(c byte) bool {
	a.quote = a.quote != (c == '"' && !a.re)
	a.re = a.re != (c == '/' && !a.quote)
	return c == ',' && !a.quote && !a.re
}

// compileRegex 编译 ## 正则（统一走 RE2）。
func compileRegex(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("ruleexec: 正则 %q 无效: %w", pattern, err)
	}
	return re, nil
}
