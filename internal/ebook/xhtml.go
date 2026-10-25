package ebook

import (
	"bytes"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// maxTitleRunes 是章节标题的长度上限：超过就认为是把正文错标成了标题。
const maxTitleRunes = 80

// blockTags 是会产生段落边界的元素：正文按段分行，阅读体验与 txt 一致。
var blockTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true, "td": true, "th": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "section": true, "article": true, "pre": true, "hr": true,
	"table": true, "ul": true, "ol": true, "dl": true, "dt": true, "dd": true,
	"figure": true, "figcaption": true, "header": true, "footer": true, "aside": true, "nav": true,
}

// skipTags 是不进正文的元素：脚本/样式/文档头，以及 ruby 的注音（rt/rp）——
// 中文书的注音会打断阅读，只保留被注音的汉字本体。
var skipTags = map[string]bool{
	"script": true, "style": true, "head": true, "title": true,
	"noscript": true, "svg": true, "rt": true, "rp": true,
}

// htmlChapter 把一段 XHTML 转成章节标题与纯文本正文。
//
// 返回值 fromHeading 标记标题是否取自正文里的 h1~h3：<title> 在排版差的 epub 里
// 常常整本书都写成书名，调用方据此决定是否退回文件名。
func htmlChapter(data []byte, href string) (title, text string, fromHeading bool) {
	root, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return chapterTitleFromHref(href), "", false
	}
	text = textFromHTMLRoot(root)
	if heading := firstHeadingText(root); heading != "" {
		return heading, stripLeadingTitle(text, heading), true
	}
	title = documentTitle(root)
	if title == "" {
		title = chapterTitleFromHref(href)
	}
	return title, stripLeadingTitle(text, title), false
}

// textFromHTML 解析 HTML 片段并抽纯文本（简介等短文本用）。
func textFromHTML(data []byte) string {
	root, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return textFromHTMLRoot(root)
}

// textFromHTMLRoot 遍历 DOM 抽纯文本：块级元素断段、跳过不可见元素，
// 实体由 html 解析器解码，空白按 HTML 语义折叠。
func textFromHTMLRoot(root *html.Node) string {
	w := &textWalker{}
	w.walk(root)
	return flushLines(w.sb.String())
}

// textWalker 累积正文：段落边界只保留一个换行，连续空白折叠成一个空格。
type textWalker struct {
	sb     strings.Builder
	lastNL bool
	space  bool
}

// walk 深度优先遍历节点。
func (w *textWalker) walk(n *html.Node) {
	switch n.Type {
	case html.ElementNode:
		tag := strings.ToLower(n.Data)
		if skipTags[tag] {
			return
		}
		block := blockTags[tag]
		if block {
			w.newline()
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			w.walk(c)
		}
		if block {
			w.newline()
		}
	case html.TextNode:
		w.text(n.Data)
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			w.walk(c)
		}
	}
}

// text 写入文本节点：空白折叠成一个空格，非空字符前补回被折叠的空格。
func (w *textWalker) text(s string) {
	for _, r := range s {
		if unicode.IsSpace(r) {
			w.space = true
			continue
		}
		if w.space && w.sb.Len() > 0 && !w.lastNL {
			w.sb.WriteByte(' ')
		}
		w.space, w.lastNL = false, false
		w.sb.WriteRune(r)
	}
}

// newline 写入段落边界；连续边界与开头边界只算一个。
func (w *textWalker) newline() {
	if w.sb.Len() == 0 || w.lastNL {
		return
	}
	w.sb.WriteByte('\n')
	w.lastNL, w.space = true, false
}

// flushLines 逐行去空白、丢空行后重新拼接。
func flushLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// firstHeadingText 返回第一个非空 h1/h2/h3 的文本（章节标题首选来源）。
// 超长的一行（把整段正文错标成标题）不算标题，交给 <title>/文件名兜底。
func firstHeadingText(root *html.Node) string {
	text := findText(root, isHeadingTag)
	if utf8.RuneCountInString(text) > maxTitleRunes {
		return ""
	}
	return text
}

// documentTitle 返回 <title> 元素的文本。
func documentTitle(root *html.Node) string {
	return findText(root, func(tag string) bool { return tag == "title" })
}

// findText 深度优先找第一个满足 match 且有文字的元素，返回其单行文本。
func findText(n *html.Node, match func(string) bool) string {
	if n.Type == html.ElementNode && match(strings.ToLower(n.Data)) {
		if text := singleLine(nodeText(n)); text != "" {
			return text
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if text := findText(c, match); text != "" {
			return text
		}
	}
	return ""
}

// nodeText 取子树纯文本（跳过脚本/样式/注音），用于标题行。
// 从子节点开始走：元素自身可能就在 skipTags 里（如 <title>）。
func nodeText(n *html.Node) string {
	w := &textWalker{}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.walk(c)
	}
	return flushLines(w.sb.String())
}

// isHeadingTag 判断标签是否是 h1~h3。
func isHeadingTag(tag string) bool {
	return len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '3'
}

// singleLine 把多行标题压成一行（标题里出现 <br> 的情况）。
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// chapterTitleFromHref 用文档文件名兜底章节标题。
func chapterTitleFromHref(href string) string {
	base := path.Base(href)
	return strings.TrimSpace(strings.TrimSuffix(base, path.Ext(base)))
}

// stripLeadingTitle 去掉正文开头与标题重复的那一段，避免章节标题出现两次。
func stripLeadingTitle(text, title string) string {
	if title == "" || text == "" {
		return text
	}
	head, rest, found := strings.Cut(text, "\n")
	if strings.TrimSpace(head) != strings.TrimSpace(title) {
		return text
	}
	if !found {
		return ""
	}
	return strings.TrimSpace(rest)
}

// firstImageSrc 返回 HTML 里第一张 <img> 的 src（封面页常见写法）。
func firstImageSrc(data []byte) string {
	root, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return findImageSrc(root)
}

// findImageSrc 深度优先找第一个带非空 src 的 img。
func findImageSrc(n *html.Node) string {
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "img") {
		for _, attr := range n.Attr {
			if strings.EqualFold(attr.Key, "src") && strings.TrimSpace(attr.Val) != "" {
				return strings.TrimSpace(attr.Val)
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if src := findImageSrc(c); src != "" {
			return src
		}
	}
	return ""
}
