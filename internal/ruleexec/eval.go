package ruleexec

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
)

// Eval 求值规则串：`||` 分支依次尝试，取第一个非空结果；`&&` 段同理。
// 降级规则（含 @js）恒返回空串，不发请求、不报错。
func (r *Rule) Eval(ctx *Context) (string, error) {
	if r.Degraded {
		return "", nil
	}
	for _, alt := range r.alts {
		for _, seg := range alt.segs {
			v, err := seg.eval(ctx)
			if err != nil {
				return "", err
			}
			if v != "" {
				return v, nil
			}
		}
	}
	return "", nil
}

// eval 求值单个 segment：选择器 → 后缀 → ## 正则 → | 管道。
func (seg segment) eval(ctx *Context) (string, error) {
	if seg.prefix == "js" {
		return "", nil
	}
	query := applyTemplate(seg.query, ctx.Vars)
	raw, err := seg.selectValue(ctx, query)
	if err != nil {
		return "", err
	}
	for _, rep := range seg.regex {
		re, cerr := compileRegex(rep.pattern)
		if cerr != nil {
			return "", cerr
		}
		raw = re.ReplaceAllString(raw, rep.repl)
	}
	val := pipeValue{s: raw}
	for _, p := range seg.pipes {
		val, err = applyPipe(ctx, p, val)
		if err != nil {
			return "", err
		}
	}
	return val.s, nil
}

// selectValue 按前缀选择求值方式。
func (seg segment) selectValue(ctx *Context, query string) (string, error) {
	switch seg.prefix {
	case "css":
		return cssValue(ctx, query, seg.suffix)
	case "xpath":
		return xpathValue(ctx, query, seg.suffix)
	case "json":
		return jsonValue(ctx, query)
	default:
		return "", fmt.Errorf("ruleexec: 未知前缀 %q", seg.prefix)
	}
}

// cssValue 用 goquery 求值并套用后缀。
func cssValue(ctx *Context, query string, sf suffix) (string, error) {
	root := ctx.Node
	if root == nil {
		if ctx.HTML == nil {
			return "", nil
		}
		root = ctx.HTML.Selection
	}
	n := root.Find(query).First()
	if n.Length() == 0 {
		return "", nil
	}
	switch sf.kind {
	case "html":
		h, err := n.Html()
		return h, err
	case "attr":
		return n.AttrOr(sf.arg, ""), nil
	case "ownText":
		return ownText(n), nil
	default:
		return n.Text(), nil
	}
}

// ownText 取节点自身的直接文本（不含子元素文本）。
func ownText(n *goquery.Selection) string {
	var b strings.Builder
	n.Contents().Each(func(_ int, c *goquery.Selection) {
		if goquery.NodeName(c) == "#text" {
			b.WriteString(c.Text())
		}
	})
	return b.String()
}

// xpathValue 用 htmlquery 求值；`/@attr` 结尾的路径按属性取值。
func xpathValue(ctx *Context, query string, sf suffix) (string, error) {
	node := ctx.HTML.Get(0)
	if ctx.Node != nil {
		node = ctx.Node.Get(0)
	}
	if node == nil {
		return "", nil
	}
	path, attr := query, ""
	if i := strings.LastIndex(query, "/@"); i >= 0 {
		path, attr = query[:i], query[i+2:]
	}
	nodes, err := htmlquery.QueryAll(node, path)
	if err != nil {
		return "", fmt.Errorf("ruleexec: XPath %q 无效: %w", query, err)
	}
	if len(nodes) == 0 {
		return "", nil
	}
	if attr != "" {
		return htmlquery.SelectAttr(nodes[0], attr), nil
	}
	if sf.kind == "html" {
		return htmlquery.OutputHTML(nodes[0], false), nil
	}
	return htmlquery.InnerText(nodes[0]), nil
}

// SelectAll 求值列表型规则（如 bookList），返回每个匹配节点的上下文。
func (r *Rule) SelectAll(ctx *Context) ([]*Context, error) {
	if r.Degraded {
		return nil, nil
	}
	for _, alt := range r.alts {
		for _, seg := range alt.segs {
			if seg.prefix != "css" {
				continue
			}
			root := ctx.Node
			if root == nil {
				root = ctx.HTML.Selection
			}
			var out []*Context
			root.Find(applyTemplate(seg.query, ctx.Vars)).Each(func(_ int, s *goquery.Selection) {
				out = append(out, &Context{HTML: ctx.HTML, Node: s, JSON: ctx.JSON, Vars: ctx.Vars})
			})
			if len(out) > 0 {
				return out, nil
			}
		}
	}
	return nil, nil
}

// applyTemplate 把 {{name}} 替换成上下文变量；未知变量替换为空串。
func applyTemplate(s string, vars map[string]string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	out := s
	for {
		start := strings.Index(out, "{{")
		if start < 0 {
			return out
		}
		end := strings.Index(out[start:], "}}")
		if end < 0 {
			return out
		}
		end += start
		name := strings.TrimSpace(out[start+2 : end])
		out = out[:start] + vars[name] + out[end+2:]
	}
}
