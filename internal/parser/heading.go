package parser

import (
	"sort"
	"strings"
	"unicode/utf8"
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

// chapterUnitReject 是「章级单位」的否定预查后缀。
//
// 没有预查时「第二节课」「三回头看」「第二回合」都会被当成章节标题——
// 这类词在正文里出现频率极高，是章节误判的主要来源。参考 legado 与
// novel-processor 的做法：单位字后面紧跟这些字时，判定为普通词语而非标记。
var chapterUnitReject = map[rune][]rune{
	'节': {'课', '气', '日', '目', '省', '能', '约', '奏', '点', '假', '食'},
	'節': {'課', '氣', '日', '目', '省', '能', '約', '奏', '點', '假', '食'},
	'集': {'合', '和', '团', '團', '体', '體', '市', '资', '資', '中', '邮', '郵'},
	'部': {'分', '队', '隊', '落', '门', '門', '位', '长', '長', '署', '件', '族'},
	'篇': {'张', '張', '幅', '目', '章'},
	'回': {'合', '访', '訪', '忆', '憶', '顾', '顧', '应', '應', '答', '音', '到',
		'头', '頭', '来', '來', '去', '过', '過', '家', '国', '國', '事', '想', '首'},
	'话': {'说', '說', '题', '題', '别', '別', '筒', '费', '費', '音', '剧', '劇'},
	'話': {'說', '題', '別', '筒', '費', '音', '劇'},
}

// isChapterUnitAt 判断 s[idx:] 是否以「合法的章级单位」开头（含否定预查）。
//
// 这是章节识别的唯一入口：所有需要判章单位的地方都必须走它，
// 直接用 isChapterUnit 会漏掉正文误判防护。
func isChapterUnitAt(s string, idx int) bool {
	r, sz := utf8.DecodeRuneInString(s[idx:])
	if !isChapterUnit(r) {
		return false
	}
	next := s[idx+sz:]
	if next == "" {
		return true
	}
	nr, _ := utf8.DecodeRuneInString(next)
	for _, bad := range chapterUnitReject[r] {
		if nr == bad {
			return false
		}
	}
	return true
}

// isVolumeUnitAt 判断 s[idx:] 是否以「合法的卷级单位」开头（含否定预查）。
func isVolumeUnitAt(s string, idx int) bool {
	r, sz := utf8.DecodeRuneInString(s[idx:])
	if !isVolumeUnit(r) {
		return false
	}
	next := s[idx+sz:]
	if next == "" {
		return true
	}
	nr, _ := utf8.DecodeRuneInString(next)
	for _, bad := range chapterUnitReject[r] {
		if nr == bad {
			return false
		}
	}
	return true
}

// isMarkerBoundary 判断裸标记词（序章/前言/正文/番外…）之后是否到了边界。
//
// 「前言不搭后语」「正文开始了」这类句子如果被当成标题，会凭空切断正文，
// 所以裸标记词后面必须要么结束、要么是分隔符/标点/括号/数字/章标记字。
func isMarkerBoundary(s string) bool {
	if s == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s)
	if isTitleSep(r) || isPadRune(r) {
		return true
	}
	switch r {
	case '：', ':', '，', ',', '、', '．', '.', '·', '-', '—', '（', '(', '【', '[', '「', '《',
		'】', ']', '）', ')', '」', '》', '。', '！', '？', '；':
		return true
	}
	if (r >= '0' && r <= '9') || (r >= '０' && r <= '９') {
		return true
	}
	// 中文数字与章级单位字也属于边界（如「序章 第一章」这种连写）。
	if _, ok := cnDigitValue[r]; ok {
		return true
	}
	return isChapterUnit(r) || isVolumeUnit(r) || r == '第'
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
