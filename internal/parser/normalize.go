package parser

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// 需要全局剔除的零宽/格式字符。
var zeroWidths = []string{"\uFEFF", "\u200B", "\u200C", "\u200D", "\u2060", "\u00AD"}

// normalizeText 做与内容语义无关的全局规范化：CRLF/CR → LF、去 BOM 与零宽字符。
// 全角空格/不间断空格在 trimLine 里按行处理（避免整篇逐字符重构）。
//
// 实现上用 strings.ReplaceAll 而不是逐字符扫描：只有真正出现过的模式才付扫描成本。
func normalizeText(s string) string {
	if s == "" {
		return s
	}
	if strings.Contains(s, "\r\n") {
		s = strings.ReplaceAll(s, "\r\n", "\n")
	}
	if strings.Contains(s, "\r") {
		s = strings.ReplaceAll(s, "\r", "\n")
	}
	for _, zw := range zeroWidths {
		if strings.Contains(s, zw) {
			s = strings.ReplaceAll(s, zw, "")
		}
	}
	return s
}

// ---------- 噪声清洗 ----------
//
// 逐行清洗是解析热路径（一本书常有一百多万行），因此不用「每行跑几条正则」：
// 先整篇扫描一遍噪声词，记下「含噪声词的行」的起始偏移；逐行处理时只有这些行
// 走完整清洗，其余行只做 O(1) 判定。

// lineNoiseTokens 是「整行噪声」关键词（书源广告、群号、版权声明、更新时间等）。
var lineNoiseTokens = []string{
	"更多小说", "小说群", "中转", "免费资源", "搜书吧", "soushu", "qisuu", "强鹿",
	"本作品来自互联网", "作品仅供读者预览", "请支持正版", "版权归原作者",
	"章节内容", "本章字数", "更新时间", "TXT", "16K小说网", "www", "http",
}

// inlineRule 是行内广告的替换规则，gate 为启用该规则所需的前置关键词。
type inlineRule struct {
	gate string
	re   *regexp.Regexp
}

var inlineRules = []inlineRule{
	{"更多小说", regexp.MustCompile("更多小说(?:资源|更新)?[+＋\\-－]?\\s*[Qq]{0,2}群\\s*[:：]?\\s*[\\d'\"]+[\\d'\" ;,.?!:_\\-]*")},
	{"更多小说", regexp.MustCompile("更多小说资源尽在强鹿小说群\\s*[:：]?")},
	{"群", regexp.MustCompile("[QqＱｑ]{1,2}群\\s*[:：]?\\s*\\d{5,}")},
	{"群", regexp.MustCompile("(?:强鹿小说群|中转[一二三四五]?群|免费资源[一二三四五]?群)\\s*[:：]?\\s*\\d*")},
	{"搜书吧", regexp.MustCompile("[\\[【（(]?搜书吧(?:网址)?[\\]】）)]?")},
	{"http", regexp.MustCompile("(?:https?://|www\\.)[A-Za-z0-9._%\\-/?=&#:+~]+")},
	{"soushu", regexp.MustCompile("(?i)(?:soushu|qisuu)\\d*\\.[a-z]{2,4}")},
	{"TXT", regexp.MustCompile("TXT\\d{5,}")},
}

var (
	reHTMLTag   = regexp.MustCompile("</?[A-Za-z][^<>]{0,120}>")
	reCopyright = regexp.MustCompile("^(?:声明|郑重声明)\\s*[:：]")
)

// cleaner 保存一篇文档的噪声特征，供逐行清洗时快速判定。
type cleaner struct {
	lineToks []string     // 本文档真正出现过的整行噪声词
	rules    []inlineRule // 本文档真正可能命中的行内规则
	checkOb  bool         // 是否存在混淆水印的可能
	occs     []int        // 噪声词出现位置（升序，全文绝对偏移）
}

// newCleaner 扫描整篇文本，得出这篇文档真正需要启用的噪声规则。
//
// 为了控制大文件的开销：先在前/中/后若干采样块里判断「哪些噪声词出现过」，
// 再只对出现过的那几个词做全文扫描，记下出现位置。
func newCleaner(text string) *cleaner {
	c := &cleaner{}
	regions := sampleRegions(len(text))
	present := func(tok string) bool {
		for _, r := range regions {
			if strings.Contains(text[r[0]:r[1]], tok) {
				return true
			}
		}
		return false
	}

	for _, tok := range lineNoiseTokens {
		if present(tok) {
			c.lineToks = append(c.lineToks, tok)
		}
	}
	for _, r := range inlineRules {
		if present(r.gate) {
			c.rules = append(c.rules, r)
		}
	}
	c.checkOb = present(";") && present("群")

	// 全文扫描：记录所有噪声词出现的位置，供逐行清洗时做区间判定。
	scans := make([]string, 0, len(c.lineToks)+len(c.rules)+2)
	scans = append(scans, c.lineToks...)
	for _, r := range c.rules {
		scans = append(scans, r.gate)
	}
	if present("声明") {
		scans = append(scans, "声明")
	}
	if c.checkOb {
		scans = append(scans, "群")
	}
	if len(scans) > 0 {
		seen := make(map[string]bool, len(scans))
		occs := make([]int, 0, 256)
		for _, tok := range scans {
			if tok == "" || seen[tok] {
				continue
			}
			seen[tok] = true
			for idx := 0; idx < len(text); {
				j := strings.Index(text[idx:], tok)
				if j < 0 {
					break
				}
				occs = append(occs, idx+j)
				idx += j + len(tok)
			}
		}
		sort.Ints(occs)
		out := occs[:0]
		last := -1
		for _, v := range occs {
			if v == last {
				continue
			}
			out = append(out, v)
			last = v
		}
		c.occs = out
	}
	return c
}

// sampleRegions 给出用于「噪声词是否出现」判断的采样区间。
// 小文件直接返回全文；大文件采样文件头、文件尾与若干等距块（约 1.4MB）。
func sampleRegions(n int) [][2]int {
	const small = 1 << 20
	if n <= small {
		return [][2]int{{0, n}}
	}
	const chunk = 128 << 10
	out := make([][2]int, 0, 10)
	add := func(start, size int) {
		if start < 0 {
			start = 0
		}
		if start+size > n {
			size = n - start
		}
		if size > 0 {
			out = append(out, [2]int{start, start + size})
		}
	}
	add(0, 256<<10)
	add(n-(256<<10), 256<<10)
	for i := 1; i < 8; i++ {
		add(n*i/8-chunk/2, chunk)
	}
	return out
}

// newFullCleaner 「所有规则都启用」的清洗器，用于小段文本的一次性清洗。
func newFullCleaner() *cleaner {
	c := &cleaner{checkOb: true}
	c.lineToks = append(c.lineToks, lineNoiseTokens...)
	c.rules = append(c.rules, inlineRules...)
	return c
}

var fullCleaner = newFullCleaner()

// hasLineNoise 判断一行是否命中已启用的整行噪声词。
func (c *cleaner) hasLineNoise(t string) bool {
	for _, tok := range c.lineToks {
		if strings.Contains(t, tok) {
			return true
		}
	}
	return false
}

// isNoiseLine 判断整行是否应丢弃（完整判定，供含噪声词的行使用）。
func (c *cleaner) isNoiseLine(t string) bool {
	if t == "" {
		return true
	}
	if isSepLine(t) {
		return true
	}
	if len(t) <= 600 && c.hasLineNoise(t) {
		return true
	}
	if strings.Contains(t, "本作品来自互联网") || strings.Contains(t, "作品仅供读者预览") ||
		strings.Contains(t, "请支持正版") || strings.Contains(t, "版权归原作者") {
		return true
	}
	if len(t) <= 120 && reCopyright.MatchString(t) {
		return true
	}
	if c.checkOb && looksObfuscated(t) {
		return true
	}
	return false
}

// stripInline 去掉行内广告片段与 HTML 标签。
func (c *cleaner) stripInline(t string) string {
	if strings.ContainsRune(t, '<') && reHTMLTag.MatchString(t) {
		t = reHTMLTag.ReplaceAllString(t, "")
	}
	hit := false
	for _, r := range c.rules {
		if !strings.Contains(t, r.gate) {
			continue
		}
		if r.re.MatchString(t) {
			t = r.re.ReplaceAllString(t, "")
			hit = true
		}
	}
	if hit {
		t = trimLine(t)
	}
	return t
}

// cleanContent 清洗一段正文（偏移未知时走完整判定，较慢）。
func (c *cleaner) cleanContent(s string) string {
	return c.cleanContentAt(s, -1)
}

// cleanContentAt 清洗一段正文；base 是这段文本在全文中的起始偏移（-1 表示未知）。
// 已知偏移时可以只对「含噪声词的行」走完整清洗，其余行走快路径。
func (c *cleaner) cleanContentAt(s string, base int) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	occ := 0
	if base >= 0 {
		occ = sort.SearchInts(c.occs, base)
	}
	lineStart := 0
	for i := 0; i <= len(s); i++ {
		if i != len(s) && s[i] != '\n' {
			continue
		}
		line := trimLine(s[lineStart:i])
		absStart, absEnd := base+lineStart, base+i
		lineStart = i + 1
		if line == "" {
			continue
		}
		if isSepLine(line) {
			continue
		}
		noisy := base < 0
		if base >= 0 {
			for occ < len(c.occs) && c.occs[occ] < absStart {
				occ++
			}
			noisy = occ < len(c.occs) && c.occs[occ] < absEnd
		}
		if noisy {
			if len(line) <= 600 && c.hasLineNoise(line) {
				continue
			}
			if strings.Contains(line, "本作品来自互联网") || strings.Contains(line, "作品仅供读者预览") ||
				strings.Contains(line, "请支持正版") || strings.Contains(line, "版权归原作者") {
				continue
			}
			if len(line) <= 120 && reCopyright.MatchString(line) {
				continue
			}
			if c.checkOb && looksObfuscated(line) {
				continue
			}
			if t := c.stripInline(line); t != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(t)
			}
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

// 分隔字符集合（==== ---- **** 等）。
func isSepRune(r rune) bool {
	switch r {
	case '=', '-', '*', '＊', '☆', '★', '◇', '◆', '○', '●', '■', '□', '▪', '▫',
		'~', '～', '_', '—', '－', '·', '•', '#', '|':
		return true
	}
	return false
}

// isSepLine 判断整行是否只有分隔符。
func isSepLine(t string) bool {
	if len(t) < 3 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(t)
	if !isSepRune(r) {
		return false
	}
	n := 0
	for _, r := range t {
		if !isSepRune(r) {
			return false
		}
		n++
	}
	return n >= 3
}

// looksObfuscated 判断一行是否像被插入了混淆水印（大量 ; ' " ? 之类的标点）。
func looksObfuscated(t string) bool {
	if len(t) < 12 || len(t) > 400 {
		return false
	}
	punct, total := 0, 0
	for _, r := range t {
		total++
		switch r {
		case ';', '\'', '"', '?', ':', ',', '!':
			punct++
		}
	}
	return punct >= 8 && punct*3 >= total
}

// isNoiseLineFull 对所有噪声规则都检查（用于简介/标题这类短文本）。
func isNoiseLineFull(t string) bool { return fullCleaner.isNoiseLine(t) }

// stripInlineNoiseFull 对所有行内广告规则都检查（用于简介/标题这类短文本）。
func stripInlineNoiseFull(t string) string { return fullCleaner.stripInline(t) }

// trimLine 去掉行首行尾的空白（含全角空格与不间断空格）。
func trimLine(s string) string {
	i, j := 0, len(s)
	for i < j {
		switch {
		case s[i] == ' ' || s[i] == '\t':
			i++
		case s[i] == 0xE3 && i+2 < j && s[i+1] == 0x80 && s[i+2] == 0x80: // U+3000
			i += 3
		case s[i] == 0xC2 && i+1 < j && s[i+1] == 0xA0: // U+00A0
			i += 2
		default:
			goto tail
		}
	}
tail:
	for j > i {
		switch {
		case s[j-1] == ' ' || s[j-1] == '\t':
			j--
		case j >= 3 && s[j-3] == 0xE3 && s[j-2] == 0x80 && s[j-1] == 0x80:
			j -= 3
		case j >= 2 && s[j-2] == 0xC2 && s[j-1] == 0xA0:
			j -= 2
		default:
			return s[i:j]
		}
	}
	return s[i:j]
}

// runeLen 返回字符数（按 UTF-8 计算）。
func runeLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		n++
	}
	return n
}

// truncateRunes 按字符数截断。
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	i, cnt := 0, 0
	for i < len(s) && cnt < n {
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		cnt++
	}
	return s[:i]
}
