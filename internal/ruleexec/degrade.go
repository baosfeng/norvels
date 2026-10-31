package ruleexec

import (
	"fmt"
	"sort"
	"strings"
)

// DegradedField 描述一个因 @js: 被降级的字段。
type DegradedField struct {
	Block  string // 例如 ruleToc
	Field  string // 例如 chapterList
	Reason string // 目前只有 js_unsupported
	Hard   bool   // true 表示该源不可用
}

// FieldSet 是一组字段规则的编译结果（key 形如 "ruleToc.chapterList"）。
type FieldSet struct {
	Rules    map[string]*Rule
	Degraded []DegradedField
}

// hardFields 是缺失或降级就导致整源不可用的字段名。
var hardFields = map[string]bool{
	"bookList":    true,
	"chapterList": true,
	"content":     true,
	"searchUrl":   true,
	"exploreUrl":  true,
}

// CompileFields 编译整组字段规则；按 key 排序保证告警顺序稳定。
func CompileFields(fields map[string]string) (*FieldSet, error) {
	fs := &FieldSet{Rules: make(map[string]*Rule, len(fields))}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		r, err := Compile(fields[key])
		if err != nil {
			return nil, fmt.Errorf("ruleexec: 字段 %s: %w", key, err)
		}
		fs.Rules[key] = r
		if !r.Degraded {
			continue
		}
		block, field := splitFieldKey(key)
		fs.Degraded = append(fs.Degraded, DegradedField{
			Block: block, Field: field, Reason: "js_unsupported", Hard: hardFields[field],
		})
	}
	return fs, nil
}

// splitFieldKey 把 "ruleToc.chapterList" 拆成 (ruleToc, chapterList)。
func splitFieldKey(key string) (string, string) {
	if i := strings.LastIndex(key, "."); i >= 0 {
		return key[:i], key[i+1:]
	}
	return "", key
}

// Status 汇总降级情况：unusable（有 hard）/ partial（只有 soft）/ ok。
func (fs *FieldSet) Status() string {
	if fs == nil || len(fs.Degraded) == 0 {
		return "ok"
	}
	for _, d := range fs.Degraded {
		if d.Hard {
			return "unusable"
		}
	}
	return "partial"
}
