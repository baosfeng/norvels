package parser

import (
	"sort"
	"strings"
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
