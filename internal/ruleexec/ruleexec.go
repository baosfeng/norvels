// Package ruleexec 求值在线书源的声明式规则串（CSS / XPath / JSONPath / 正则 / 模板 / 受限管道）。
//
// 一条规则串 = 前缀 + 选择器 + 可选后缀 + 可选正则 + 可选管道，`||` 与 `&&` 做跨规则组合。
// 语法与优先级见 docs/adr/0004-online-book-source.md §4 与 /tmp/online-rule-executor.md。
//
// 本包**不执行任何 JS**：@js 规则在编译期标记降级（见 degrade.go），运行期不发请求。
package ruleexec

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Context 是一次求值的上下文。
type Context struct {
	HTML *goquery.Document  // HTML 文档（CSS/XPath 用）
	Node *goquery.Selection // 当前节点（列表迭代时非空）
	JSON any                // JSON 文档（@json 用）
	Vars map[string]string  // 模板变量：baseUrl / key / page / bookUrl …
}

// Rule 是一条编译后的规则串。
type Rule struct {
	alts     []alternative
	Degraded bool // 命中 @js:，运行期返回空
	Raw      string
}

// alternative 是 || 分隔的一支：内部 && 顺序尝试，取第一个非空。
type alternative struct {
	segs     []segment
	degraded bool
}

// segment 是一次「选择器 + 后缀 + 正则 + 管道」求值。
type segment struct {
	prefix string        // css / xpath / json / js
	query  string        // 选择器（已做模板替换）
	suffix suffix        // 取文本 / 属性
	regex  []replacement // ##查找##替换### 列表
	pipes  []pipeCall    // | fn(...) 列表
}

// suffix 决定从节点上取什么。
type suffix struct {
	kind string // text / ownText / html / attr
	arg  string // attr 名
}

// replacement 是一条 ##查找##替换### 规则。
type replacement struct {
	pattern string
	repl    string
}

// pipeCall 是一次管道调用。
type pipeCall struct {
	name string
	args []string
}

// Compile 编译一条规则串；语法错误（未知前缀 / 坏正则 / 未知管道函数）直接返回错误。
func Compile(raw string) (*Rule, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("ruleexec: 规则为空")
	}
	r := &Rule{Raw: trimmed}
	for _, altText := range splitTop(trimmed, "||") {
		alt, err := parseAlt(altText)
		if err != nil {
			return nil, err
		}
		if alt.degraded {
			r.Degraded = true
		}
		r.alts = append(r.alts, alt)
	}
	return r, nil
}

// parseAlt 解析 && 分隔的一段。
func parseAlt(text string) (alternative, error) {
	out := alternative{}
	for _, segText := range splitTop(text, "&&") {
		seg, err := parseSegment(segText)
		if err != nil {
			return out, err
		}
		if seg.prefix == "js" {
			out.degraded = true
		}
		out.segs = append(out.segs, seg)
	}
	return out, nil
}

// splitTop 按分隔符切分，但跳过 ## 正则段内部（正则里的 | && || 不算分隔符）。
func splitTop(s, sep string) []string {
	var out []string
	inRe, start := false, 0
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "##") {
			inRe = !inRe
			i += 2
			continue
		}
		if !inRe && strings.HasPrefix(s[i:], sep) {
			out = append(out, s[start:i])
			i += len(sep)
			start = i
			continue
		}
		i++
	}
	return append(out, s[start:])
}
