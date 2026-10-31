package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// declFilter 是声明式规则包编译出的过滤器：一条正则 + 替换文本，作用于全文。
//
// 它的执行位置由 runFilters 决定：内置过滤器之后、按注入顺序执行。规则包里的
// priority 只用于「注入项之间」排序（见 importer 的编译层），不改变与内置过滤器
// 的相对顺序——本切片不动解析核心的既有排序。
type declFilter struct {
	name     string
	priority int
	pattern  *regexp.Regexp
	replace  string
}

// NewDeclFilter 把一条声明式清洗规则编译成 Filter。
//
// 与内置过滤器共享同一条降级语义：runFilters 用 guard 包住 Apply，单条出错只记
// warning。正则编译失败在这里返回 error，由调用方决定跳过整条规则还是拒绝整份
// 规则包（importer 编译层选择跳过整份并给出 warning）。
func NewDeclFilter(name string, priority int, pattern, replace string) (Filter, error) {
	re, err := regexp.Compile(withLineAnchors(pattern))
	if err != nil {
		return nil, fmt.Errorf("parser: 声明式过滤器 %q 的正则非法: %w", name, err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "decl-filter"
	}
	return &declFilter{name: name, priority: priority, pattern: re, replace: replace}, nil
}

// withLineAnchors 给规则包正则补上多行模式：规则包里的 ^ $ 按行锚定，
// 与 split.rules「一行一判定」同口径；显式写了 (?m) 时不重复加。
func withLineAnchors(pattern string) string {
	if strings.Contains(pattern, "(?m") {
		return pattern
	}
	return "(?m)" + pattern
}

// Name 返回规则包里的展示名（用于 DisableFilters 与诊断）。
func (f *declFilter) Name() string { return f.name }

// Priority 返回规则包里的优先级（注入项之间升序）。
func (f *declFilter) Priority() int { return f.priority }

// Apply 对全文做一次全局替换；没有命中时直接返回，避免无谓的分配。
func (f *declFilter) Apply(d *Doc) error {
	if !f.pattern.MatchString(d.Text) {
		return nil
	}
	d.Text = f.pattern.ReplaceAllString(d.Text, f.replace)
	return nil
}
