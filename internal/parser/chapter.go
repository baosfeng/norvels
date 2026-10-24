package parser

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"norvels/internal/model"
)

const (
	// maxHeadingBytes 标题行的字节上限（中文标题 40 字约 120 字节）。
	maxHeadingBytes = 400
	minHeadingBytes = 2
)

// headKind 是标题行的类型。
type headKind int

const (
	headChapter headKind = iota + 1 // 章
	headSpecial                     // 序章/楔子/番外/后记…
	headVolume                      // 卷/部/集
)

// heading 是一行标题的解析结果。
type heading struct {
	kind     headKind
	num      int    // 章号（有数字时）
	hasNum   bool   // 是否解析出章号
	strong   bool   // 是否为强特征（第N章、序章、Chapter N…）
	marker   string // 标记原文，如 "第一章"
	title    string // 章节标题（空表示标题与正文粘连，用 marker 兜底）
	inline   string // 标题行里被误并入标题的正文
	volNum   int    // 卷号
	volTitle string // 卷标题（完整，如 "第一卷 风云初起"）
}

// 特殊章节标记（按长度降序匹配，避免「番外篇」被「番外」抢先）。
var specialMarkers = func() []string {
	m := []string{
		"完本感言", "大结局", "最终章", "作品相关", "正文完", "番外篇", "番外",
		"序章", "楔子", "契子", "引子", "序幕", "序言", "前言", "后记", "後記",
		"终章", "終章", "尾声", "尾聲", "外传", "外傳", "附录", "附錄", "后序",
		"终篇", "终局", "感言", "序",
	}
	sort.SliceStable(m, func(i, j int) bool { return len(m[i]) > len(m[j]) })
	return m
}()

// 卷级单位与章级单位。
func isVolumeUnit(r rune) bool {
	switch r {
	case '卷', '部', '集', '篇':
		return true
	}
	return false
}

func isChapterUnit(r rune) bool {
	switch r {
	case '章', '節', '节', '回', '话', '話':
		return true
	}
	return false
}

// isLeadDecor 判断行首装饰字符（可成串出现）。
func isLeadDecor(r rune) bool {
	switch r {
	case ' ', '\t', '\u3000', '\u00a0', '=', '#', '|', '☆', '★', '◆', '◇', '●', '○',
		'■', '□', '▪', '▫', '·', '•', '~', '～', '_', '-', '—', '－', '*', '＊',
		'【', '】', '[', ']', '［', '］', '「', '」', '『', '』', '《', '》',
		'“', '”', '‘', '’', '"', '\'', '〔', '〕', '〖', '〗', '、', '，', ',', '.':
		return true
	}
	return false
}

// isPadRune 判断是否是排版用的填充空白（含不间断空格）。
func isPadRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\u3000' || r == '\u00a0'
}

// isTailDecor 判断行尾装饰字符（不含括号，避免误伤「（上）」）。
func isTailDecor(r rune) bool {
	switch r {
	case ' ', '\t', '\u3000', '\u00a0', '=', '#', '|', '☆', '★', '◆', '◇', '●', '○',
		'■', '□', '·', '•', '~', '～', '_', '-', '—', '－', '*', '＊', '、':
		return true
	}
	return false
}

// isTitleSep 判断标记与标题之间的分隔字符。
func isTitleSep(r rune) bool {
	switch r {
	case ' ', '\t', '\u3000', '\u00a0', '：', ':', '、', '，', ',', '.', '．', '·',
		'-', '—', '－', '☆', '★', '◆', '◇', '|', '~', '～', '_', '=', '#',
		'“', '”', '"', '‘', '’':
		return true
	}
	return false
}

// collapseSpaces 把连续空白（含全角空格）折叠成一个空格。
func collapseSpaces(s string) string {
	if !strings.Contains(s, "  ") && !strings.Contains(s, "　") && !strings.Contains(s, "	") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	prev := false
	for _, r := range s {
		if r == ' ' || r == '	' || r == '　' {
			if !prev {
				b.WriteByte(' ')
				prev = true
			}
			continue
		}
		prev = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// isSentenceTail 判断是否句末标点（标题不会以这些结尾）。
func isSentenceTail(r rune) bool {
	switch r {
	case '，', '。', '！', '？', '；', '…', '”', '」', '『', '．':
		return true
	}
	return false
}

// stripDecorations 去掉行首行尾装饰字符。
func stripDecorations(s string) string {
	i := 0
	for i < len(s) {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if !isLeadDecor(r) {
			break
		}
		i += sz
	}
	s = s[i:]
	j := len(s)
	for j > 0 {
		r, sz := utf8.DecodeLastRuneInString(s[:j])
		if !isTailDecor(r) {
			break
		}
		j -= sz
	}
	return s[:j]
}

func trimLeadSeps(s string) string {
	i := 0
	for i < len(s) {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if !isTitleSep(r) && r != '】' && r != ']' && r != '］' && r != ')' && r != '）' {
			break
		}
		i += sz
	}
	return s[i:]
}

// parseNumeral 解析行首数字（阿拉伯数字/全角数字/中文数字），返回数值与消耗的字节数。
func parseNumeral(s string) (num int, n int, ok bool) {
	if s == "" {
		return 0, 0, false
	}
	c := s[0]
	if c >= '0' && c <= '9' {
		i, v := 0, 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			v = v*10 + int(s[i]-'0')
			i++
			if i >= 7 {
				break
			}
		}
		return v, i, v > 0
	}
	if c >= 0xEF && c <= 0xEF {
		// 全角数字 ０-９：UTF-8 为 EF BC 90 ~ EF BC 99
		i, v, cnt := 0, 0, 0
		for i+2 < len(s) && s[i] == 0xEF && s[i+1] == 0xBC && s[i+2] >= 0x90 && s[i+2] <= 0x99 {
			v = v*10 + int(s[i+2]-0x90)
			i += 3
			cnt++
			if cnt >= 7 {
				break
			}
		}
		if cnt > 0 {
			return v, i, v > 0
		}
		return 0, 0, false
	}
	// 中文数字
	const cnDigits = "零〇一二三四五六七八九两"
	value := func(r rune) (int, bool) {
		i := strings.IndexRune(cnDigits, r)
		if i < 0 {
			return 0, false
		}
		if i == 0 || i == 1 {
			return 0, true
		}
		if i == 10 { // 两
			return 2, true
		}
		return i - 1, true
	}
	unit := func(r rune) int {
		switch r {
		case '十':
			return 10
		case '百':
			return 100
		case '千':
			return 1000
		}
		return 0
	}
	total, section, number, cnt := 0, 0, 0, 0
	i := 0
	for i < len(s) {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if d, ok := value(r); ok {
			number = d
			i += sz
			cnt++
			continue
		}
		if u := unit(r); u > 0 {
			if number == 0 {
				number = 1
			}
			section += number * u
			number = 0
			i += sz
			cnt++
			continue
		}
		if r == '万' {
			if number == 0 && section == 0 {
				number = 1
			}
			total += (section + number) * 10000
			section, number = 0, 0
			i += sz
			cnt++
			continue
		}
		break
	}
	if cnt == 0 {
		return 0, 0, false
	}
	total += section + number
	if total <= 0 || total > 100000 {
		return 0, 0, false
	}
	return total, i, true
}

// cutDiNum 解析行首的「第」+ 数字。
func cutDiNum(s string) (num int, numText string, rest string, ok bool) {
	if !strings.HasPrefix(s, "第") {
		return 0, "", "", false
	}
	r := s[len("第"):]
	// 第 与数字之间允许空格
	trimmed := 0
	for trimmed < len(r) {
		c := r[trimmed]
		if c == ' ' || c == '\t' {
			trimmed++
			continue
		}
		if c == 0xE3 && trimmed+2 < len(r) && r[trimmed+1] == 0x80 && r[trimmed+2] == 0x80 {
			trimmed += 3 // 全角空格
			continue
		}
		break
	}
	v, n, valid := parseNumeral(r[trimmed:])
	if !valid {
		return 0, "", "", false
	}
	return v, string(r[:trimmed+n]), r[trimmed+n:], true
}

// parseHeading 解析一行标题。返回 false 表示这行不是标题。
func parseHeading(raw string) (heading, bool) {
	s := stripDecorations(raw)
	if s == "" || len(s) > maxHeadingBytes {
		return heading{}, false
	}
	// 有的书用大量空格做「两端对齐/居中」，先折叠空白再判定长度
	s = collapseSpaces(s)
	if len(s) < minHeadingBytes {
		return heading{}, false
	}
	// 统一排除：真正的章节标题行不会很长，也不会以句末标点收尾。
	if runeLen(s) > 40 {
		return heading{}, false
	}
	if last, _ := utf8.DecodeLastRuneInString(s); isSentenceTail(last) {
		return heading{}, false
	}
	// 去掉「正文」「正文卷」「VIP」这类前缀
	volPre := ""
	if strings.HasPrefix(s, "正文卷") {
		volPre = "正文卷"
		s = s[len("正文卷"):]
		s = trimLeadSeps(s)
	} else if strings.HasPrefix(s, "正文") {
		s = s[len("正文"):]
		s = trimLeadSeps(s)
	}
	if s == "" {
		if volPre != "" {
			return heading{kind: headVolume, volTitle: volPre, strong: true}, true
		}
		return heading{}, false
	}

	if h, ok := parseVolumeHeading(s, volPre); ok {
		return h, true
	}
	if h, ok := parseChapter(s, volPre != ""); ok {
		if volPre != "" {
			h.volTitle = volPre
		}
		return h, true
	}
	return heading{}, false
}

// parseVolumeHeading 解析「第N卷/部/集」开头的标题，可能同时带章节部分。
func parseVolumeHeading(s, volPre string) (heading, bool) {
	num, numText, rest, ok := cutDiNum(s)
	if !ok {
		return heading{}, false
	}
	rest = trimLeadSeps(rest)
	if rest == "" {
		if volPre != "" {
			return heading{}, false
		}
		return heading{kind: headVolume, volNum: num, volTitle: "第" + numText + "卷", strong: true}, true
	}
	r, sz := utf8.DecodeRuneInString(rest)
	if !isVolumeUnit(r) {
		return heading{}, false
	}
	base := "第" + numText + string(r)
	after := trimLeadSeps(rest[sz:])
	// 卷 + 章 同行
	if h, ok := parseChapter(after, false); ok && h.kind == headChapter {
		h.volNum = num
		h.volTitle = base
		return h, true
	}
	// 卷名 + 章 同行：在前缀里找章标记
	if idx := findChapterMarker(after); idx > 0 {
		vt := strings.TrimSpace(after[:idx])
		if h, ok := parseChapter(after[idx:], false); ok && h.kind == headChapter {
			title := base
			if vt != "" {
				title = base + " " + trimTitleNoise(vt)
			}
			h.volNum = num
			h.volTitle = title
			return h, true
		}
	}
	// 纯卷标题
	title := base
	if t := trimTitleNoise(after); t != "" {
		title = base + " " + t
	}
	return heading{kind: headVolume, volNum: num, volTitle: title, strong: true}, true
}

// findChapterMarker 在字符串中查找「第N章/节/回」或特殊标记的起始下标；找不到返回 -1。
func findChapterMarker(s string) int {
	for i := 0; i < len(s); {
		if s[i] == 0xE7 { // 「第」的 UTF-8 首字节为 E7 AC AC
			if strings.HasPrefix(s[i:], "第") {
				r := s[i+len("第"):]
				if _, n, ok := parseNumeral(r); ok {
					if rr, sz := utf8.DecodeRuneInString(r[n:]); isChapterUnit(rr) || isVolumeUnit(rr) {
						_ = sz
						return i
					}
				}
			}
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
	}
	return -1
}

// parseChapter 解析章节标题（不含卷前缀）。
func parseChapter(s string, afterZhengwen bool) (heading, bool) {
	if h, ok := parseDiUnit(s, afterZhengwen); ok {
		return h, true
	}
	if strings.HasPrefix(s, "第") {
		// 第N卷/部/集 由卷解析处理
		return heading{}, false
	}
	if h, ok := parseSpecial(s); ok {
		return h, true
	}
	if h, ok := parseNumUnit(s, afterZhengwen); ok {
		return h, true
	}
	if h, ok := parseEnChapter(s); ok {
		return h, true
	}
	if h, ok := parseNumTitle(s); ok {
		return h, true
	}
	return heading{}, false
}

func parseDiUnit(s string, strong bool) (heading, bool) {
	num, numText, rest, ok := cutDiNum(s)
	if !ok {
		return heading{}, false
	}
	r, sz := utf8.DecodeRuneInString(rest)
	if !isChapterUnit(r) {
		return heading{}, false
	}
	marker := "第" + numText + string(r)
	title, inline := splitTitle(rest[sz:])
	return heading{
		kind: headChapter, num: num, hasNum: true, strong: true,
		marker: marker, title: title, inline: inline,
	}, true
}

func parseSpecial(s string) (heading, bool) {
	for _, m := range specialMarkers {
		if !strings.HasPrefix(s, m) {
			continue
		}
		rest := s[len(m):]
		// 「序」这类短标记要求后面没有紧跟内容
		if m == "序" || m == "感言" || m == "作品相关" {
			if rest != "" {
				r, _ := utf8.DecodeRuneInString(rest)
				if !isTitleSep(r) {
					continue
				}
			}
		}
		title, inline := splitTitle(rest)
		return heading{kind: headSpecial, strong: true, marker: m, title: title, inline: inline}, true
	}
	return heading{}, false
}

// parseNumUnit 解析「05章」「001 标题」这类以数字开头的行。
func parseNumUnit(s string, strong bool) (heading, bool) {
	num, numText, rest, ok := cutLeadingNumber(s)
	if !ok {
		return heading{}, false
	}
	if rest != "" {
		if r, sz := utf8.DecodeRuneInString(rest); isChapterUnit(r) {
			title, inline := splitTitle(rest[sz:])
			return heading{
				kind: headChapter, num: num, hasNum: true, strong: true,
				marker: numText + string(r), title: title, inline: inline,
			}, true
		}
	}
	_ = strong
	return heading{}, false
}

// parseNumTitle 解析弱特征的「0001」「1、标题」形式。
func parseNumTitle(s string) (heading, bool) {
	num, numText, rest, ok := cutLeadingNumber(s)
	if !ok {
		return heading{}, false
	}
	if rest == "" {
		// 纯数字行
		if len(numText) > 5 {
			return heading{}, false
		}
		return heading{kind: headChapter, num: num, hasNum: true, marker: numText}, true
	}
	r, sz := utf8.DecodeRuneInString(rest)
	if !isTitleSep(r) {
		return heading{}, false
	}
	body := trimLeadSeps(rest[sz:])
	body = strings.Trim(body, " \t\u3000")
	if body == "" {
		return heading{}, false
	}
	// 句末标点说明这多半是正文
	if strings.ContainsAny(body, "。！？；") && runeLen(body) > 12 {
		return heading{}, false
	}
	title, inline := splitTitle(body)
	return heading{
		kind: headChapter, num: num, hasNum: true, strong: false,
		marker: numText, title: title, inline: inline,
	}, true
}

// cutLeadingNumber 解析行首数字，返回数值、数字原文与剩余部分。
func cutLeadingNumber(s string) (num int, numText string, rest string, ok bool) {
	v, n, valid := parseNumeral(s)
	if !valid || n == 0 {
		return 0, "", "", false
	}
	return v, s[:n], s[n:], true
}

func parseEnChapter(s string) (heading, bool) {
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "chapter") {
		return heading{}, false
	}
	rest := s[len("chapter"):]
	rest = strings.TrimLeft(rest, " \t")
	num, n, ok := parseNumeral(rest)
	if !ok {
		return heading{}, false
	}
	title, inline := splitTitle(rest[n:])
	return heading{
		kind: headChapter, num: num, hasNum: true, strong: true,
		marker: "Chapter " + fmt.Sprint(num), title: title, inline: inline,
	}, true
}

// splitTitle 把标记之后的文本切成「标题」与「误并入标题的正文」。
func splitTitle(rest string) (title, inline string) {
	raw := rest
	hadSep := false
	if raw != "" {
		if r, sz := utf8.DecodeRuneInString(raw); isTitleSep(r) {
			hadSep = true
			raw = raw[sz:]
		}
	}
	t := strings.Trim(raw, " \t\u3000\u00a0")
	t = trimTitleNoise(t)
	if t == "" {
		return "", ""
	}
	// 标题行里夹带正文：以句号切开
	if idx := strings.Index(t, "。"); idx > 0 {
		head := strings.TrimSpace(t[:idx])
		if runeLen(head) <= 60 {
			return head, strings.TrimSpace(t[idx+len("。"):])
		}
	}
	// 标记与正文粘连（无分隔符）且内容很长：整行视为正文
	if !hadSep && runeLen(t) > 14 {
		return "", t
	}
	if runeLen(t) > 40 {
		return truncateRunes(t, 40), ""
	}
	return t, ""
}

// trimTitleNoise 清理标题两端的装饰与内联广告。
func trimTitleNoise(t string) string {
	t = stripInlineNoiseFull(t)
	t = strings.Trim(t, " \t\u3000\u00a0")
	t = strings.TrimRight(t, "=#|☆★◆◇○●■□·•~～_—－*＊")
	return strings.TrimSpace(t)
}

// ---------- 切分 ----------

// candidate 是一个候选标题在全文中的位置。
type candidate struct {
	start     int // 标题行起始字节偏移
	bodyStart int // 正文起始字节偏移
	h         heading
}

// splitResult 是章节切分的中间结果。
type splitResult struct {
	volumes  []model.ParsedVolume
	chapters []model.ParsedChapter
	preamble string
	warnings []string
	restarts int // 章号重置次数（用于判断合集）
}

// splitBook 把规范化后的全文切成卷与章。
func (p *Parser) splitBook(text string) splitResult {
	cl := newCleaner(text)
	cands := scanCandidates(text)
	if len(cands) < 3 {
		// 换行极少的文件：章节标记可能埋在超长行内部
		if extra := scanEmbedded(text); len(extra) > 0 {
			cands = mergeCandidates(cands, extra)
		}
	}
	if len(cands) == 0 {
		return p.fallbackSplit(text, cl)
	}
	cands = validateCandidates(cands)
	if len(cands) == 0 {
		return p.fallbackSplit(text, cl)
	}
	res := p.buildFromCandidates(text, cands, cl)
	if len(res.chapters) == 0 {
		return p.fallbackSplit(text, cl)
	}
	// 切出来的章太少而正文很长（说明标题格式不规范、或本来就没有标题）→ 走兜底切分。
	total := 0
	for _, c := range res.chapters {
		total += runeLen(c.Content)
	}
	if total > 12000 && (len(res.chapters) <= 2 || total/len(res.chapters) > 30000) {
		fb := p.fallbackSplit(text, cl)
		if len(fb.chapters) > len(res.chapters) {
			fb.warnings = append(fb.warnings, "章节标记过少，已按长度兜底切分")
			return fb
		}
	}
	return res
}

// scanCandidates 逐行扫描候选标题行。
func scanCandidates(text string) []candidate {
	var out []candidate
	lineStart := 0
	for i := 0; i <= len(text); i++ {
		if i != len(text) && text[i] != '\n' {
			continue
		}
		lineEnd := i
		lineLen := lineEnd - lineStart
		if lineLen >= minHeadingBytes {
			line := text[lineStart:lineEnd]
			lead := headingLeadOK(line)
			handled := false
			if lead && lineLen <= maxHeadingBytes {
				if h, ok := parseHeading(line); ok {
					out = append(out, candidate{start: lineStart, bodyStart: min(lineEnd+1, len(text)), h: h})
					handled = true
				}
			}
			if !handled && !lead && lineLen <= 120 && strings.Contains(line, "第") {
				// 「书名 第001章 标题」这类带前缀的标题行
				if h, ok := parsePrefixedHeading(line); ok {
					out = append(out, candidate{start: lineStart, bodyStart: min(lineEnd+1, len(text)), h: h})
					handled = true
				}
			}
			if !handled && lead && (lineLen > maxHeadingBytes || runeLen(line) > 40) {
				// 标题与正文挤在同一行（常见于用大量空格/NBSP 排版的文件）
				pos := lineStart
				for pos < lineEnd {
					h, off, ok := parseInlineHeading(text[pos:lineEnd])
					if !ok || off <= 0 {
						break
					}
					out = append(out, candidate{start: pos, bodyStart: min(pos+off, lineEnd), h: h})
					pos += off
				}
			}
		}
		lineStart = i + 1
	}
	return out
}

// headingLeadOK 用首字符快速排除绝大多数正文行。
func headingLeadOK(s string) bool {
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if isLeadDecor(r) {
			i += sz
			continue
		}
		switch r {
		case '第', '序', '楔', '契', '引', '前', '后', '後', '终', '終', '尾', '番',
			'外', '附', '正', '文', '完', '感', '作', '最', '大', '结', '結', '局',
			'C', 'c':
			return true
		}
		if (r >= '0' && r <= '9') || (r >= '０' && r <= '９') {
			return true
		}
		return false
	}
	return false
}

// parseInlineHeading 处理「标题与正文挤在同一行」的情况，例如：
//
//	"  第三章   女歌手   一位身穿白色短裙…"
//
// 返回解析出的标题以及「正文从本行第几个字节开始」。
func parseInlineHeading(line string) (heading, int, bool) {
	skip := 0
	for skip < len(line) {
		r, sz := utf8.DecodeRuneInString(line[skip:])
		if !isLeadDecor(r) {
			break
		}
		skip += sz
	}
	if skip >= len(line) {
		return heading{}, 0, false
	}
	rest := line[skip:]
	var h heading
	consumed := 0
	if num, numText, r2, ok := cutDiNum(rest); ok {
		if u, sz := utf8.DecodeRuneInString(r2); isVolumeUnit(u) {
			consumed = len("第") + len(numText) + sz
			h = heading{kind: headVolume, volNum: num, volTitle: "第" + numText + string(u), strong: true}
		} else if isChapterUnit(u) {
			consumed = len("第") + len(numText) + sz
			h = heading{kind: headChapter, num: num, hasNum: true, strong: true,
				marker: "第" + numText + string(u)}
		}
	}
	if consumed == 0 {
		for _, m := range specialMarkers {
			if strings.HasPrefix(rest, m) {
				consumed = len(m)
				h = heading{kind: headSpecial, strong: true, marker: m}
				break
			}
		}
	}
	if consumed == 0 {
		return heading{}, 0, false
	}

	pos := skip + consumed
	// 跳过标题前的填充
	for pos < len(line) {
		r, sz := utf8.DecodeRuneInString(line[pos:])
		if !isPadRune(r) && !isTitleSep(r) {
			break
		}
		pos += sz
	}
	titleStart := pos
	titleEnd := pos
	runes := 0
	for titleEnd < len(line) && runes < 20 {
		r, sz := utf8.DecodeRuneInString(line[titleEnd:])
		if isSentenceTail(r) {
			break
		}
		if isPadRune(r) || isTitleSep(r) {
			// 连续填充 → 标题结束
			if nr, _ := utf8.DecodeRuneInString(line[titleEnd+sz:]); isPadRune(nr) || isTitleSep(nr) {
				break
			}
		}
		titleEnd += sz
		runes++
	}
	h.title = strings.Trim(line[titleStart:titleEnd], " 	 　")
	if h.title != "" && strings.Contains(h.title, "。") {
		h.title = ""
	}
	bodyPos := titleEnd
	for bodyPos < len(line) {
		r, sz := utf8.DecodeRuneInString(line[bodyPos:])
		if !isPadRune(r) && !isTitleSep(r) {
			break
		}
		bodyPos += sz
	}
	if h.title == "" {
		h.title = ""
	}
	return h, bodyPos, true
}

// parsePrefixedHeading 解析「书名/篇名 + 第N章」这类带短前缀的标题行。
func parsePrefixedHeading(line string) (heading, bool) {
	idx := strings.Index(line, "第")
	if idx <= 0 || idx > 48 {
		return heading{}, false
	}
	// 「第」前面必须是分隔符：否则多半是正文里的「（见第六章）」这类引用
	if !hasSepBeforeByte(line, idx) {
		return heading{}, false
	}
	prefix := strings.TrimSpace(line[:idx])
	if prefix == "" || runeLen(prefix) > 16 {
		return heading{}, false
	}
	if strings.ContainsAny(prefix, "。！？；，：") || strings.HasSuffix(prefix, "的") {
		return heading{}, false
	}
	lr, _ := utf8.DecodeLastRuneInString(line)
	switch lr {
	case '。', '！', '？', '；', '…', '”', '」':
		return heading{}, false
	}
	h, ok := parseChapter(line[idx:], false)
	if !ok || !h.strong || h.kind != headChapter {
		return heading{}, false
	}
	if h.title == "" {
		return heading{}, false
	}
	h.volTitle = trimTitleNoise(prefix)
	return h, true
}

// hasSepBeforeByte 判断 line[idx] 之前是否是分隔符（空格/全角空格/·/：/|）。
func hasSepBeforeByte(line string, idx int) bool {
	if idx <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(line[:idx])
	switch r {
	case ' ', '	', '　', '·', '：', ':', '|', '｜', '_', '-', '—':
		return true
	}
	return false
}

// scanEmbedded 在超长行内部查找章节标记（用于「整篇几乎没有换行」的文件）。
func scanEmbedded(text string) []candidate {
	var out []candidate
	lineStart := 0
	for i := 0; i <= len(text); i++ {
		if i != len(text) && text[i] != '\n' {
			continue
		}
		lineEnd := i
		if lineEnd-lineStart > 400 {
			line := text[lineStart:lineEnd]
			pos := 0
			for {
				rel := strings.Index(line[pos:], "第")
				if rel < 0 {
					break
				}
				abs := lineStart + pos + rel
				h, ok := parseHeading(line[pos+rel : min(pos+rel+maxHeadingBytes, len(line))])
				if ok && h.kind == headChapter && h.title != "" {
					bodyStart := abs + len(h.marker)
					bodyStart = min(bodyStart, len(text))
					out = append(out, candidate{start: abs, bodyStart: bodyStart, h: h})
				}
				pos += rel + len("第")
			}
		}
		lineStart = i + 1
	}
	return out
}

func mergeCandidates(a, b []candidate) []candidate {
	if len(b) == 0 {
		return a
	}
	m := make([]candidate, 0, len(a)+len(b))
	m = append(m, a...)
	m = append(m, b...)
	sort.SliceStable(m, func(i, j int) bool { return m[i].start < m[j].start })
	// 去重：位置相同或过近
	out := m[:0]
	last := -1
	for _, c := range m {
		if c.start-last < 8 {
			continue
		}
		out = append(out, c)
		last = c.start
	}
	return out
}

// validateCandidates 过滤误判，保证章号大体单调递增。
func validateCandidates(cands []candidate) []candidate {
	strongCnt := 0
	for _, c := range cands {
		if c.h.kind == headVolume || c.h.strong {
			strongCnt++
		}
	}
	out := make([]candidate, 0, len(cands))
	if strongCnt >= 3 {
		// 强特征也可能来自正文里的「见第六章」这类引用：要求章号大体递增，
		// 允许从 1~5 重新开始（多卷/合集每卷重新编号）。
		prev := 0
		for _, c := range cands {
			if c.h.kind == headVolume {
				out = append(out, c)
				continue
			}
			if !c.h.strong {
				continue
			}
			if !c.h.hasNum {
				out = append(out, c) // 序章/番外/后记等无编号章节
				continue
			}
			n := c.h.num
			if prev == 0 || n > prev || n <= 5 {
				out = append(out, c)
				prev = n
			}
		}
		return out
	}
	// 全是弱特征：必须靠数字单调性筛选
	prev := 0
	for _, c := range cands {
		if c.h.kind == headVolume {
			out = append(out, c)
			continue
		}
		if !c.h.hasNum || c.h.num <= 0 {
			continue
		}
		if prev == 0 {
			out = append(out, c)
			prev = c.h.num
			continue
		}
		if c.h.num > prev && c.h.num <= prev+300 {
			out = append(out, c)
			prev = c.h.num
		}
	}
	return out
}

// buildFromCaniates 依据候选标题构建卷与章。
func (p *Parser) buildFromCandidates(text string, cands []candidate, cl *cleaner) splitResult {
	res := splitResult{}
	if len(cands) > 0 {
		res.preamble = text[:cands[0].start]
	}
	curVol := -1
	var curTitle, curInline string
	curBody := 0
	started := false
	lastNum := 0
	flush := func(end int) {
		if !started {
			return
		}
		body := text[curBody:min(end, len(text))]
		content := cl.cleanContentAt(body, curBody)
		if curInline != "" {
			if head := cl.cleanContent(curInline); head != "" {
				if content == "" {
					content = head
				} else {
					content = head + "\n" + content
				}
			}
		}
		title := curTitle
		if title == "" {
			title = "正文"
		}
		res.chapters = append(res.chapters, model.ParsedChapter{
			Title: title, VolumeIdx: curVol, Content: content, WordCount: countWords(content),
		})
		started = false
	}
	for _, c := range cands {
		h := c.h
		if h.kind == headVolume {
			flush(c.start)
			vt := h.volTitle
			if vt == "" {
				vt = "未命名卷"
			}
			res.volumes = append(res.volumes, model.ParsedVolume{Idx: len(res.volumes), Title: vt})
			curVol = len(res.volumes) - 1
			continue
		}
		// 章号重置检测（合集/多部作品）
		if h.hasNum {
			if lastNum > 10 && h.num <= 2 && len(res.chapters) > 3 {
				res.restarts++
			}
			lastNum = h.num
		}
		flush(c.start)
		curTitle = joinTitle(h.marker, h.title)
		curInline = h.inline
		curBody = c.bodyStart
		started = true
	}
	flush(len(text))
	return res
}

// fallbackSplit 在没有任何章节标记时兜底：整篇单章或等长切分。
func (p *Parser) fallbackSplit(text string, cl *cleaner) splitResult {
	res := splitResult{preamble: headOfText(text, 4000)}
	content := cl.cleanContentAt(text, 0)
	if content == "" {
		return res
	}
	chunk := p.Opts.FallbackChunkChars
	if chunk <= 0 {
		chunk = 8000
	}
	if !p.Opts.AllowFallbackSplit || runeLen(content) <= chunk*3/2 {
		res.chapters = append(res.chapters, model.ParsedChapter{
			Title: "全文", VolumeIdx: -1, Content: content, WordCount: countWords(content),
		})
		res.warnings = append(res.warnings, "未发现章节标记，按单章处理")
		return res
	}
	var b strings.Builder
	b.Grow(chunk * 3)
	n := 0
	flush := func() {
		if b.Len() == 0 {
			return
		}
		c := b.String()
		n++
		res.chapters = append(res.chapters, model.ParsedChapter{
			Title: fmt.Sprintf("第%d节", n), VolumeIdx: -1, Content: c, WordCount: countWords(c),
		})
		b.Reset()
	}
	for _, line := range strings.Split(content, "\n") {
		if line == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		if runeLen(b.String()) >= chunk {
			flush()
		}
	}
	flush()
	res.warnings = append(res.warnings, "未发现章节标记，按长度等分切分")
	return res
}

// postProcess 合并过短/空章节、修正卷下标、重排章节序号。
func (p *Parser) postProcess(res *splitResult) {
	minLen := p.Opts.MinChapterLen
	if minLen < 0 {
		minLen = 0
	}
	merged := make([]model.ParsedChapter, 0, len(res.chapters))
	for _, c := range res.chapters {
		if c.Content == "" {
			continue
		}
		if len(merged) > 0 && len(c.Content) < minLen && !isSpecialTitle(c.Title) {
			prev := &merged[len(merged)-1]
			if c.Title != "" && c.Title != "正文" {
				prev.Content += "\n" + c.Title
				prev.WordCount += countWords(c.Title)
			}
			prev.Content += "\n" + c.Content
			prev.WordCount += c.WordCount
			continue
		}
		merged = append(merged, c)
	}
	if len(merged) == 0 {
		res.chapters = nil
		res.volumes = nil
		return
	}
	// 卷裁剪与重排
	used := make(map[int]bool, len(res.volumes))
	for _, c := range merged {
		if c.VolumeIdx >= 0 && c.VolumeIdx < len(res.volumes) {
			used[c.VolumeIdx] = true
		}
	}
	remap := make(map[int]int, len(res.volumes))
	vols := make([]model.ParsedVolume, 0, len(res.volumes))
	for i, v := range res.volumes {
		if !used[i] {
			continue
		}
		remap[i] = len(vols)
		v.Idx = len(vols)
		vols = append(vols, v)
	}
	for i := range merged {
		merged[i].Idx = i
		if merged[i].VolumeIdx >= 0 {
			if nv, ok := remap[merged[i].VolumeIdx]; ok {
				merged[i].VolumeIdx = nv
			} else {
				merged[i].VolumeIdx = -1
			}
		}
	}
	res.chapters = merged
	res.volumes = vols
}

// headOfText 取文本开头 n 字节（不足则全文），用于元数据/合集判定。
func headOfText(text string, n int) string {
	if len(text) <= n {
		return text
	}
	cut := n
	if i := strings.LastIndexByte(text[:n], '\n'); i > 0 {
		cut = i
	}
	return text[:cut]
}

// joinTitle 拼出「标记 + 标题」，保留原始的「第N章 / 第一回 / 0001」写法。
func joinTitle(marker, title string) string {
	if title == "" {
		return marker
	}
	if marker == "" {
		return title
	}
	if strings.HasPrefix(title, marker) {
		return title
	}
	return marker + " " + title
}

func isSpecialTitle(t string) bool {
	if t == "" {
		return false
	}
	for _, m := range specialMarkers {
		if strings.HasPrefix(t, m) {
			return true
		}
	}
	return false
}
