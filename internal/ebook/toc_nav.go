package ebook

// EPUB3 的 nav.xhtml 目录解析。用 x/net/html（与正文抽取同一套容错）而不是
// encoding/xml：真实的 nav.xhtml 常带未转义实体、缺失自闭合标签这类 XML 不合法的写法。

import (
	"bytes"
	"errors"
	"strings"

	"golang.org/x/net/html"
)

// parseNavXHTML 解析 nav.xhtml，返回目录树（第 maxTOCLevel 层以下忽略）。
func parseNavXHTML(data []byte) ([]*tocNode, error) {
	root, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	nav := findTOCNav(root)
	if nav == nil {
		return nil, errors.New("nav.xhtml 里没有目录 nav")
	}
	list := firstChildTag(nav, "ol")
	if list == nil {
		return nil, errors.New("nav.xhtml 的目录 nav 里没有 ol 列表")
	}
	return navNodes(list, 1), nil
}

// findTOCNav 先找声明为目录的 nav（epub:type="toc"），找不到退回第一个 nav。
func findTOCNav(root *html.Node) *html.Node {
	if got := findNav(root, isTOCNav); got != nil {
		return got
	}
	return findNav(root, func(*html.Node) bool { return true })
}

// findNav 深度优先找第一个满足 match 的 nav 元素。
func findNav(n *html.Node, match func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "nav") && match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if got := findNav(c, match); got != nil {
			return got
		}
	}
	return nil
}

// isTOCNav 判断 nav 是否声明为目录：epub:type / type / role 里含 toc。
func isTOCNav(n *html.Node) bool {
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "epub:type", "type", "role":
			for _, f := range strings.Fields(a.Val) {
				if strings.EqualFold(f, "toc") || strings.EqualFold(f, "doc-toc") {
					return true
				}
			}
		}
	}
	return false
}

// navNodes 把 <ol> 的直接子 <li> 转成条目；第 maxTOCLevel 层以下直接丢弃。
func navNodes(list *html.Node, level int) []*tocNode {
	if level > maxTOCLevel {
		return nil
	}
	out := make([]*tocNode, 0, 8)
	for li := firstChildTag(list, "li"); li != nil; li = nextSiblingTag(li, "li") {
		link := firstChildTag(li, "a")
		if link == nil {
			link = firstChildTag(li, "span")
		}
		if link == nil {
			continue // 没有链接的条目无法与 spine 对应
		}
		n := &tocNode{
			Title: singleLine(nodeText(link)),
			Href:  attrValue(link, "href"),
			Level: level,
		}
		if sub := firstChildTag(li, "ol"); sub != nil {
			n.Children = navNodes(sub, level+1)
		}
		out = append(out, n)
	}
	return out
}

// firstChildTag 返回第一个直接子元素（tag 为空表示任意元素）。
func firstChildTag(n *html.Node, tag string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		if tag == "" || strings.EqualFold(c.Data, tag) {
			return c
		}
	}
	return nil
}

// nextSiblingTag 返回下一个同名兄弟元素。
func nextSiblingTag(n *html.Node, tag string) *html.Node {
	for c := n.NextSibling; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && strings.EqualFold(c.Data, tag) {
			return c
		}
	}
	return nil
}

// attrValue 取元素属性值（大小写不敏感）。
func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}
