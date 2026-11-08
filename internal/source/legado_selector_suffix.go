package source

// Legado 选择器的「末段判定」：最后一段到底是取内容后缀，还是子元素选择器。
// 与转换主流程分文件，避免单文件超出门禁 5 的 400 行上限。

import (
	"regexp"
	"strings"
)

// reAttrName 与 ruleexec 的属性后缀判定同口径（见 ruleexec.peelSuffix）。
var reAttrName = regexp.MustCompile(`^[A-Za-z0-9:_-]+$`)

// htmlTags 是「最后一段裸名字按元素选择器处理」的白名单（HTML 标签名）。
// 不在表里的裸名字仍按属性后缀处理，例如 href / src / data-original；
// title 同时是标签名与属性名，按属性后缀处理（书源里 @title 取的是属性）。
var htmlTags = map[string]bool{
	"a": true, "abbr": true, "article": true, "aside": true, "audio": true, "b": true,
	"blockquote": true, "body": true, "br": true, "button": true, "caption": true,
	"code": true, "dd": true, "div": true, "dl": true, "dt": true, "em": true,
	"figcaption": true, "figure": true, "font": true, "footer": true, "form": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "head": true,
	"header": true, "hr": true, "html": true, "i": true, "iframe": true, "img": true,
	"input": true, "label": true, "li": true, "main": true, "mark": true, "nav": true,
	"ol": true, "option": true, "p": true, "pre": true, "section": true, "select": true,
	"small": true, "source": true, "span": true, "strong": true, "sub": true, "sup": true,
	"table": true, "tbody": true, "td": true, "textarea": true, "tfoot": true, "th": true,
	"thead": true, "time": true, "tr": true, "u": true, "ul": true, "video": true,
}

// isLegadoSuffix 判断 @ 链的最后一段是取内容后缀（而非选择器）。
func isLegadoSuffix(part string) bool {
	p := strings.TrimSpace(part)
	if p == "" {
		return false
	}
	switch lower := strings.ToLower(p); lower {
	case "text", "owntext", "html":
		return true
	default:
		if htmlTags[lower] {
			return false // 裸标签名是选择器，不是取内容后缀
		}
		return reAttrName.MatchString(p)
	}
}

// looksLegado 判断一段选择器是否含 Legado 简写特征；不含就按原样 CSS 放行。
func looksLegado(s string) bool {
	if strings.Contains(s, "tag.") || strings.Contains(s, "class.") || strings.Contains(s, "id.") {
		return true
	}
	if strings.ContainsAny(s, "[]") {
		return false // 可能是标准 CSS 属性选择器，别按 @ 拆
	}
	parts := strings.Split(s, "@")
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if strings.HasPrefix(p, "children.") || strings.HasPrefix(p, "text.") {
			return true // 识别为简写但不支持转换：交给 convertPart 报降级
		}
		if reLegadoIndex.MatchString(p) || reLegadoExclude.MatchString(p) {
			return true
		}
	}
	// 裸 @ 链（dt@a@href）：标准 ruleexec 规则至多一个 @ 后缀，两个及以上只可能是简写。
	return len(parts) >= 3
}

// notSelectorRule 判断规则是不是模板 / JS / 纯正则：这些不是选择器简写，
// 原样放行（转换它们只会把脚本或正则改坏）。
func notSelectorRule(s string) bool {
	return strings.Contains(s, "{{") || strings.Contains(s, "<js>") ||
		strings.HasPrefix(s, ":") || strings.HasPrefix(s, "@put:") || strings.HasPrefix(s, "@get:")
}
