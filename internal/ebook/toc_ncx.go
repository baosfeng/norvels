package ebook

// EPUB2 的 toc.ncx 目录解析（encoding/xml，本地名匹配，兼容命名空间写法差异）。

import (
	"encoding/xml"
	"errors"
	"strings"
)

// ncxMediaType 是清单里 ncX 目录的媒体类型。
const ncxMediaType = "application/x-dtbncx+xml"

// ncxDoc 只取 navMap 一棵树。
type ncxDoc struct {
	NavMap struct {
		Points []ncxPoint `xml:"navPoint"`
	} `xml:"navMap"`
}

// ncxPoint 是 navMap 的一项；navPoint 可嵌套。
type ncxPoint struct {
	Label   ncxLabel   `xml:"navLabel"`
	Content ncxContent `xml:"content"`
	Points  []ncxPoint `xml:"navPoint"`
}

// ncxLabel / ncxContent 是 navPoint 的文字标签与目标 href。
type ncxLabel struct {
	Text string `xml:"text"`
}

type ncxContent struct {
	Src string `xml:"src,attr"`
}

// parseNCX 解析 toc.ncx，返回目录树（第 maxTOCLevel 层以下忽略）。
func parseNCX(data []byte) ([]*tocNode, error) {
	var doc ncxDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.NavMap.Points) == 0 {
		return nil, errors.New("toc.ncx 没有 navPoint")
	}
	return ncxNodes(doc.NavMap.Points, 1), nil
}

// ncxNodes 把 navPoint 列表转成节点，递归子级。
func ncxNodes(points []ncxPoint, level int) []*tocNode {
	if level > maxTOCLevel {
		return nil
	}
	out := make([]*tocNode, 0, len(points))
	for _, p := range points {
		out = append(out, &tocNode{
			Title:    singleLine(p.Label.Text),
			Href:     strings.TrimSpace(p.Content.Src),
			Level:    level,
			Children: ncxNodes(p.Points, level+1),
		})
	}
	return out
}

// navHref 返回清单里声明为导航文档的条目路径（EPUB3 properties 含 nav）。
func navHref(doc *opfDoc) string {
	for _, item := range doc.order {
		if hasProp(item.Properties, "nav") && isHTMLHref(item.Href) {
			return resolveHref(doc.dir, item.Href)
		}
	}
	return ""
}

// hasProp 判断空格分隔的清单属性里是否含某个词。
func hasProp(props, want string) bool {
	for _, f := range strings.Fields(props) {
		if strings.EqualFold(f, want) {
			return true
		}
	}
	return false
}

// ncxHref 返回 toc.ncx 的条目路径：先按清单 media-type，再退回第一个 .ncx。
func ncxHref(a *archive, doc *opfDoc) string {
	for _, item := range doc.order {
		if strings.EqualFold(strings.TrimSpace(item.MediaType), ncxMediaType) {
			return resolveHref(doc.dir, item.Href)
		}
	}
	return a.firstByExt(".ncx")
}
