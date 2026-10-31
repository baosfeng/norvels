package ruleexec

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// jsonToken 是 JSONPath 的一个路径段。
type jsonToken struct {
	kind string // field / index / any / descend
	name string
	idx  int
}

// parseJSONPath 解析最小 JSONPath 子集：$ / $.a / $.a.b / $.a[0] / $.a[*] / $..name。
func parseJSONPath(path string) ([]jsonToken, error) {
	p := strings.TrimSpace(path)
	if !strings.HasPrefix(p, "$") {
		return nil, fmt.Errorf("ruleexec: JSONPath 必须以 $ 开头: %q", path)
	}
	rest := p[1:]
	var toks []jsonToken
	for rest != "" {
		switch {
		case strings.HasPrefix(rest, ".."):
			name, next := readField(rest[2:])
			toks = append(toks, jsonToken{kind: "descend", name: name})
			rest = next
		case strings.HasPrefix(rest, "."):
			name, next := readField(rest[1:])
			toks = append(toks, jsonToken{kind: "field", name: name})
			rest = next
		case strings.HasPrefix(rest, "["):
			end := strings.Index(rest, "]")
			if end < 0 {
				return nil, fmt.Errorf("ruleexec: JSONPath 方括号未闭合: %q", path)
			}
			inner := rest[1:end]
			rest = rest[end+1:]
			if inner == "*" {
				toks = append(toks, jsonToken{kind: "any"})
				continue
			}
			n, err := strconv.Atoi(inner)
			if err != nil {
				return nil, fmt.Errorf("ruleexec: JSONPath 下标 %q 不是数字", inner)
			}
			toks = append(toks, jsonToken{kind: "index", idx: n})
		default:
			return nil, fmt.Errorf("ruleexec: JSONPath 片段无法识别: %q", rest)
		}
	}
	return toks, nil
}

// readField 读一个字段名，返回剩余部分。
func readField(s string) (string, string) {
	i := 0
	for i < len(s) && s[i] != '.' && s[i] != '[' {
		i++
	}
	return s[:i], s[i:]
}

// evalJSONPath 在 JSON 值上求值，返回第一个标量的字符串形式。
func evalJSONPath(root any, path string) (string, error) {
	toks, err := parseJSONPath(path)
	if err != nil {
		return "", err
	}
	cur := []any{root}
	for _, t := range toks {
		cur = jsonStep(cur, t)
		if len(cur) == 0 {
			return "", nil
		}
	}
	return scalarString(cur[0]), nil
}

// jsonStep 前进一步。
func jsonStep(cur []any, t jsonToken) []any {
	var out []any
	for _, v := range cur {
		out = append(out, jsonApply(v, t)...)
	}
	return out
}

// jsonApply 在一个值上应用一个路径段。
func jsonApply(v any, t jsonToken) []any {
	switch t.kind {
	case "field":
		return jsonField(v, t.name)
	case "descend":
		return descend(v, t.name)
	case "index":
		return jsonIndex(v, t.idx)
	case "any":
		return jsonAny(v)
	}
	return nil
}

// jsonField 取 map 字段。
func jsonField(v any, name string) []any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if nv, ok := m[name]; ok {
		return []any{nv}
	}
	return nil
}

// jsonIndex 取数组下标。
func jsonIndex(v any, idx int) []any {
	arr, ok := v.([]any)
	if !ok || idx < 0 || idx >= len(arr) {
		return nil
	}
	return []any{arr[idx]}
}

// jsonAny 展开数组全部元素。
func jsonAny(v any) []any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	return arr
}

// descend 深度优先找同名字段。
func descend(v any, name string) []any {
	var out []any
	switch t := v.(type) {
	case map[string]any:
		if nv, ok := t[name]; ok {
			out = append(out, nv)
		}
		for _, nv := range t {
			out = append(out, descend(nv, name)...)
		}
	case []any:
		for _, nv := range t {
			out = append(out, descend(nv, name)...)
		}
	}
	return out
}

// scalarString 把标量转成字符串；容器返回空。
func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	default:
		return ""
	}
}

// jsonValue 用于 @json: 前缀的字段求值。
func jsonValue(ctx *Context, path string) (string, error) {
	if ctx.JSON == nil {
		return "", nil
	}
	return evalJSONPath(ctx.JSON, path)
}

// parseJSON 把字符串解析成 JSON 值（供管道 json(path) 使用）。
func parseJSON(s string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("ruleexec: 内容不是合法 JSON: %w", err)
	}
	return v, nil
}
