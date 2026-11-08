package source

// Legado URL 模板的选项块解析。
//
// Legado 允许在 searchUrl / exploreUrl 末尾带一个 JSON 选项块，用逗号与 URL 分隔：
//
//	https://example.com/search?k={{key}},{"charset":"gbk"}
//
// 这里把选项块拆出来，只消费 charset；webView / headers / method 等本期不实现，
// 原样回显给调用方记降级（不认识就忽略，不让整条规则失效）。

import (
	"encoding/json"
	"sort"
	"strings"
)

// URLOptions 是 URL 模板末尾选项块的解析结果。
type URLOptions struct {
	// Charset 是指定的页面编码（小写，如 gbk / gb18030 / utf-8）；空表示未指定。
	Charset string
	// Ignored 是识别出但当前未使用的键（webView / headers / method …），排序稳定。
	Ignored []string
	// Warning 表示末尾形似选项块但 JSON 解析失败：此时 URL 原样返回，不吞掉内容。
	Warning string
}

// ParseURLOptions 拆出 URL 模板末尾的 ,{...} 选项块，返回纯 URL 与选项。
// 没有选项块时原样返回（只去首尾空白）。
func ParseURLOptions(raw string) (string, URLOptions) {
	s := strings.TrimSpace(raw)
	for i := len(s) - 1; i > 0; i-- {
		if s[i] != ',' {
			continue
		}
		rest := strings.TrimSpace(s[i+1:])
		if rest == "" || rest[0] != '{' {
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(rest), &obj); err != nil {
			// 末尾确实以「,{」开头却解析不了：保留原串，让调用方记降级。
			return s, URLOptions{Warning: "URL 选项块无法解析：" + err.Error()}
		}
		return strings.TrimSpace(s[:i]), parseURLOptions(obj)
	}
	return s, URLOptions{}
}

// parseURLOptions 从 JSON 对象里取 charset，其余键记进 Ignored。
func parseURLOptions(obj map[string]json.RawMessage) URLOptions {
	var opts URLOptions
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.EqualFold(key, "charset") {
			var v string
			if err := json.Unmarshal(obj[key], &v); err == nil {
				if cs := strings.ToLower(strings.TrimSpace(v)); cs != "" {
					opts.Charset = cs
					continue
				}
			}
		}
		opts.Ignored = append(opts.Ignored, key)
	}
	return opts
}
