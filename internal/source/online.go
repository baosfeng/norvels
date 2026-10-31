package source

// online 段的结构校验（本期只校验、不执行）：与 validate.go 拆开，
// 让「本地来源」与「在线书源」的规则各自一屏可读。

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// onlineBlock 是一个在线规则块（有序切片保证错误消息稳定，map 遍历顺序不定）。
type onlineBlock struct {
	name  string
	rules map[string]string
}

// blocks 按固定顺序返回五个在线规则块。
func (o *Online) blocks() []onlineBlock {
	return []onlineBlock{
		{"online.ruleSearch", o.RuleSearch},
		{"online.ruleExplore", o.RuleExplore},
		{"online.ruleBookInfo", o.RuleBookInfo},
		{"online.ruleToc", o.RuleToc},
		{"online.ruleContent", o.RuleContent},
	}
}

// validateOnline 只做 online 段的结构校验：本期不发起任何网络请求，URL 也不解析。
func (p *Profile) validateOnline() *Error {
	if p.Kind != KindOnline {
		if p.Online != nil {
			return paramErr("kind=%s 的规则包不能带 online 段", p.Kind)
		}
		return nil
	}
	o := p.Online
	if o == nil {
		return paramErr("kind=online 的规则包必须提供 online 段")
	}
	if err := validateOnlineURL(o.BookSourceURL); err != nil {
		return err
	}
	if err := validateOnlineExtras(o); err != nil {
		return err
	}
	if utf8.RuneCountInString(o.Group) > MaxOnlineGroupRunes {
		return paramErr("online.group 不能超过 %d 字", MaxOnlineGroupRunes)
	}
	for _, b := range o.blocks() {
		if err := validateRuleBlock(b.name, b.rules); err != nil {
			return err
		}
	}
	return validateOnlineHeader(o)
}

// validateOnlineExtras 校验来源类型与三个 URL 形态字段（不解析域名，只做护栏）。
func validateOnlineExtras(o *Online) *Error {
	if o.BookSourceType < 0 || o.BookSourceType > 3 {
		return paramErr("online.bookSourceType 必须是 0~3（0 文本 / 1 音频 / 2 图片 / 3 文件）")
	}
	if o.BookSourceType != 0 {
		return paramErr("online.bookSourceType=%d 的来源类型暂不支持（只支持文本书源）", o.BookSourceType)
	}
	for _, f := range []struct{ name, value string }{
		{"online.searchUrl", o.SearchURL},
		{"online.exploreUrl", o.ExploreURL},
		{"online.bookUrlPattern", o.BookURLPattern},
	} {
		if utf8.RuneCountInString(f.value) > MaxOnlineURLRunes {
			return paramErr("%s 不能超过 %d 字符", f.name, MaxOnlineURLRunes)
		}
	}
	return nil
}

// validateOnlineHeader 校验 header（只保存不使用）与 unsupportedFields 列表。
func validateOnlineHeader(o *Online) *Error {
	if err := validateRuleBlock("online.header", o.Header); err != nil {
		return err
	}
	if len(o.UnsupportedFields) > MaxOnlineRuleKeys {
		return paramErr("online.unsupportedFields 最多 %d 项（收到 %d 项）",
			MaxOnlineRuleKeys, len(o.UnsupportedFields))
	}
	for i, name := range o.UnsupportedFields {
		if name == "" {
			return paramErr("online.unsupportedFields 的第 %d 项不能为空", i+1)
		}
		if utf8.RuneCountInString(name) > MaxOnlineKeyRunes {
			return paramErr("online.unsupportedFields 的第 %d 项不能超过 %d 字", i+1, MaxOnlineKeyRunes)
		}
	}
	return nil
}

// validateOnlineURL 校验书源地址的写法（只保证能区分 http/https，不做 DNS 解析）。
func validateOnlineURL(raw string) *Error {
	if raw == "" {
		return paramErr("kind=online 的规则包必须填写 online.bookSourceUrl")
	}
	if utf8.RuneCountInString(raw) > MaxOnlineURLRunes {
		return paramErr("online.bookSourceUrl 不能超过 %d 字符", MaxOnlineURLRunes)
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return paramErr("online.bookSourceUrl 必须以 http:// 或 https:// 开头")
	}
	return nil
}

// validateRuleBlock 校验一个在线规则块：键值条数与长度（按 key 排序保证消息稳定）。
func validateRuleBlock(name string, rules map[string]string) *Error {
	if len(rules) > MaxOnlineRuleKeys {
		return paramErr("%s 最多 %d 条规则（收到 %d 条）", name, MaxOnlineRuleKeys, len(rules))
	}
	keys := make([]string, 0, len(rules))
	for k := range rules {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == "" {
			return paramErr("%s 里有空键", name)
		}
		if utf8.RuneCountInString(k) > MaxOnlineKeyRunes {
			return paramErr("%s 的键 %q 不能超过 %d 字", name, k, MaxOnlineKeyRunes)
		}
		if utf8.RuneCountInString(rules[k]) > MaxOnlineValRunes {
			return paramErr("%s.%s 的规则不能超过 %d 字", name, k, MaxOnlineValRunes)
		}
	}
	return nil
}
