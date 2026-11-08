package importer

// 导入内容过滤：按书名剔除女频 / 灵异悬疑类书。
//
// 判定发生在解析之后：书名取自解析结果，文件名上的广告、标签与装饰已被 parser 洗掉，
// 这里只需再做一次与匹配口径一致的归一化。命中只影响「这本书导不导入」——
// 计 Skipped、文件终态 skipped、写一条能区分 fem / supernatural 的备注；
// 不进 import_errors（它不是失败，与「章节数不足」走同一条记账路径）。
//
// 口径：
//   - 书名归一（去掉书名号、各类括号与全部空白）后包含女频特征词 → fem；
//   - 归一后出现负向词（恐怖病 / 恐怖片 / 惊悚片）→ 本次判定整体不作数，直接放行；
//   - 否则包含灵异悬疑特征词 → supernatural。
//
// 词表是包级变量：以后调口径只改这里，判定逻辑与测试都跟着这张表走。

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 内容过滤的两类原因：取值与生产 book_exclusions.reason 一致，便于反查口径是否一致。
const (
	TitleFilterReasonFem          = "fem"
	TitleFilterReasonSupernatural = "supernatural"
)

// FemTitleKeywords 是女频特征词（子串匹配，顺序即命中优先级）。
var FemTitleKeywords = []string{
	"嫡女", "庶女", "宫斗", "宅斗", "侧妃", "嬷嬷", "郡主", "长公主", "女配", "女尊",
	"贵女", "医妃", "毒妃", "宠妃", "小娘", "世婚", "侯门", "农家女", "良娣", "昭仪",
	"嫔妃", "女强",
}

// SupernaturalTitleKeywords 是灵异 / 悬疑特征词（子串匹配，顺序即命中优先级）。
var SupernaturalTitleKeywords = []string{
	"灵异", "悬疑", "惊悚", "怪谈", "盗墓", "凶宅", "殡仪", "降头", "诅咒", "招魂",
	"冥婚", "纸扎", "驱魔", "风水师", "算命先生", "凶案", "法医", "刑警", "破案", "悬案",
	"招鬼", "阴阳眼", "通灵",
}

// NegativeTitleKeywords 是负向词：命中即本次判定不作数。
//
// 「恐怖片」「惊悚片」「恐怖病」里的恐怖 / 惊悚不是小说题材，命中它们时宁可放行，
// 也不要把一本讲电影或疾病的书误挡在库外。
var NegativeTitleKeywords = []string{"恐怖病", "恐怖片", "惊悚片"}

// filterBrackets 是归一化时丢弃的括号字符（书名号 / 方头 / 圆括号）。
var filterBrackets = map[rune]bool{
	'《': true, '》': true, '〈': true, '〉': true,
	'「': true, '」': true, '『': true, '』': true,
	'【': true, '】': true, '〖': true, '〗': true,
	'（': true, '）': true, '(': true, ')': true,
	'[': true, ']': true, '［': true, '］': true,
	'{': true, '}': true, '｛': true, '｝': true,
}

// NormalizeFilterTitle 归一化书名：去掉书名号 / 括号与全部空白，只留可匹配的正文。
//
// 只删括号字符本身，不删括号里的内容：「《嫡女》（全本）」归成「嫡女全本」，
// 而不是把书名当成括号内容一起丢掉。
func NormalizeFilterTitle(title string) string {
	var b strings.Builder
	b.Grow(len(title))
	for _, r := range title {
		if unicode.IsSpace(r) || filterBrackets[r] {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TitleFilterMatch 判断书名是否属于要过滤的内容；命中返回原因（fem / supernatural）
// 与命中的特征词，未命中返回 ok=false。
func TitleFilterMatch(title string) (reason, matched string, ok bool) {
	norm := NormalizeFilterTitle(title)
	if norm == "" || containsAnyKeyword(norm, NegativeTitleKeywords) {
		return "", "", false
	}
	if w, hit := firstKeyword(norm, FemTitleKeywords); hit {
		return TitleFilterReasonFem, w, true
	}
	if w, hit := firstKeyword(norm, SupernaturalTitleKeywords); hit {
		return TitleFilterReasonSupernatural, w, true
	}
	return "", "", false
}

// firstKeyword 返回按词表顺序第一个命中的词（顺序固定，报告可复现）。
func firstKeyword(s string, words []string) (string, bool) {
	for _, w := range words {
		if w != "" && strings.Contains(s, w) {
			return w, true
		}
	}
	return "", false
}

// containsAnyKeyword 报告词表里是否有任一非空词被包含。
func containsAnyKeyword(s string, words []string) bool {
	for _, w := range words {
		if w != "" && strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// skipContentFiltered 按书名做内容过滤：命中就按 Skipped 记账并返回 true。
//
// 与「章节数不足」同一条路径：Skipped 计数 + 文件终态 skipped + 一句能区分
// fem / supernatural 的备注；不进 import_errors——它不是失败，重试也导不进来。
func (r *runState) skipContentFiltered(ctx context.Context, oc parseOutcome, book *model.ParsedBook) bool {
	reason, matched, blocked := TitleFilterMatch(book.Title)
	if !blocked {
		return false
	}
	r.res.Skipped++
	r.recordJobFile(ctx, oc.cand.source, oc.path, store.ImportFileStatusSkipped,
		fmt.Sprintf("内容过滤（%s）：命中「%s」", reason, matched), 0)
	return true
}
