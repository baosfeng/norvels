package source

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Code 是校验失败对应的错误码。值域与 api 的错误码一致，可直接下发：
// 参数类问题 invalid_param，正则编译失败 invalid_pattern（前端据此标到具体那一行）。
type Code string

const (
	CodeParam   Code = "invalid_param"
	CodePattern Code = "invalid_pattern"
)

// 预编译的字符集口径：id 只允许小写字母/数字/连字符（与 Legado 的 bookSourceUrl 唯一键同思路）。
var (
	idPattern       = regexp.MustCompile(`^[a-z0-9-]+$`)
	extPattern      = regexp.MustCompile(`^[a-z0-9+]+$`)
	encodingPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

// Error 是一条校验失败：Code 供前端分支，Message 面向用户并带「第几份/第几条」。
type Error struct {
	Code    Code
	Message string
}

// Error 实现 error，便于 api 直接交给 badRequest。
func (e *Error) Error() string { return e.Message }

// Issue 是逐条校验结果（POST /api/import/source-profiles/validate 的响应元素）。
type Issue struct {
	Index  int    `json:"index"`
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
}

// CheckAll 校验整批（已归一化的）规则包，返回逐条结果与第一个错误。
//
// 返回值约定（api 据此分流）：
//   - issues == nil 表示「整批拒绝」（份数超限），此时 err 一定非 nil；
//   - 否则 issues 与输入等长，逐份给出 ok/reason；err 是第一个错误（可能来自
//     逐份校验，也可能是 id 重复），PUT 直接用它的消息回 400。
//
// id 重复只把后出现的那份标红，所以前端能定位到具体哪一份要改；第一个错误带
// 「第几份」前缀，PUT 时直接作为 400 的 message。
func CheckAll(profiles []Profile) ([]Issue, *Error) {
	if len(profiles) > MaxProfiles {
		return nil, paramErr("来源规则包最多 %d 份（收到 %d 份）", MaxProfiles, len(profiles))
	}
	issues := make([]Issue, 0, len(profiles))
	seen := make(map[string]bool, len(profiles))
	var first *Error
	for i := range profiles {
		p := &profiles[i]
		issue := Issue{Index: i, ID: p.ID, OK: true}
		err := p.Validate()
		if err == nil && p.ID != "" {
			if seen[p.ID] {
				err = paramErr("规则包 id 与前面的规则包重复")
			}
			seen[p.ID] = true
		}
		if err != nil {
			issue.OK, issue.Reason = false, err.Message
			if first == nil {
				first = &Error{Code: err.Code, Message: fmt.Sprintf("第 %d 份规则包（%s）：%s",
					i+1, displayID(p.ID), err.Message)}
			}
		}
		issues = append(issues, issue)
	}
	return issues, first
}

// Validate 校验一份规则包，返回第一个错误（校验顺序：先身份、再各层、最后在线段）。
//
// 期望输入已由 Normalize 归一化（补全 schemaVersion/kind、trim）；对未归一化的输入
// 仍然宽容：schemaVersion=0 与 kind="" 视为默认值。
func (p *Profile) Validate() *Error {
	checks := []func() *Error{
		p.validateIdentity, p.validateMatch, p.validateDecode, p.validateFilters,
		p.validateMeta, p.validateSplit, p.validateTags, p.validateLimits, p.validateOnline,
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

// validateIdentity 校验规则包身份：id 字符集、name、schemaVersion、kind。
func (p *Profile) validateIdentity() *Error {
	if err := p.validateID(); err != nil {
		return err
	}
	if err := p.validateName(); err != nil {
		return err
	}
	if err := validateSchemaVersion(p.SchemaVersion); err != nil {
		return err
	}
	return validateKind(p.Kind)
}

// validateID 校验 id 的字符集与长度（id 是唯一键，也是规则包之间互相引用的锚点）。
func (p *Profile) validateID() *Error {
	if p.ID == "" {
		return paramErr("规则包的 id 不能为空")
	}
	if utf8.RuneCountInString(p.ID) > MaxIDRunes {
		return paramErr("规则包的 id 不能超过 %d 字符", MaxIDRunes)
	}
	if !idPattern.MatchString(p.ID) {
		return paramErr("规则包的 id %q 只能包含小写字母、数字和连字符", p.ID)
	}
	return nil
}

// validateName 校验规则包展示名（前端列表的主标题）。
func (p *Profile) validateName() *Error {
	if p.Name == "" {
		return paramErr("规则包 %q 的 name 不能为空", p.ID)
	}
	if utf8.RuneCountInString(p.Name) > MaxNameRunes {
		return paramErr("规则包 %q 的 name 不能超过 %d 字", p.ID, MaxNameRunes)
	}
	return nil
}

// validateSchemaVersion 只接受当前版本；0 视为未填（归一化会补 1），更高版本整包拒绝。
func validateSchemaVersion(v int) *Error {
	if v != 0 && v != SchemaVersion {
		return paramErr("规则包的 schemaVersion 只支持 %d（收到 %d）", SchemaVersion, v)
	}
	return nil
}

// validateKind 只接受 local / online；空串视为 local（归一化会补）。
func validateKind(kind string) *Error {
	if kind != "" && kind != KindLocal && kind != KindOnline {
		return paramErr("规则包的 kind 必须是 %s 或 %s", KindLocal, KindOnline)
	}
	return nil
}

// validateMatch 校验 match：扩展名字符集、glob 语法与两个正则。
func (p *Profile) validateMatch() *Error {
	for i, ext := range p.Match.Ext {
		if ext == "" {
			return paramErr("match.ext 的第 %d 项不能为空", i+1)
		}
		if utf8.RuneCountInString(ext) > MaxNameRunes {
			return paramErr("match.ext 的第 %d 项不能超过 %d 字符", i+1, MaxNameRunes)
		}
		if !extPattern.MatchString(ext) {
			return paramErr("match.ext 的第 %d 项 %q 只能包含小写字母、数字和加号", i+1, ext)
		}
	}
	if glob := p.Match.PathGlob; glob != "" {
		if _, err := path.Match(glob, "probe"); err != nil {
			return paramErr("match.pathGlob %q 不是合法的 glob：%v", glob, err)
		}
	}
	if err := validateOptionalPattern(p.Match.NameRegex, "match.nameRegex 的正则"); err != nil {
		return err
	}
	return validateOptionalPattern(p.Match.ZipNameRegex, "match.zipNameRegex 的正则")
}

// validateDecode 校验强制编码名（值域由 parser 注册表决定，这里只保证写法合法）。
func (p *Profile) validateDecode() *Error {
	enc := p.Decode.ForceEncoding
	if enc == "" {
		return nil
	}
	if utf8.RuneCountInString(enc) > MaxNameRunes {
		return paramErr("decode.forceEncoding 不能超过 %d 字符", MaxNameRunes)
	}
	if !encodingPattern.MatchString(enc) {
		return paramErr("decode.forceEncoding %q 只能包含小写字母、数字、下划线和连字符", enc)
	}
	return nil
}

// validateFilters 校验过滤层：禁用名单、extra 规则（名字/正则/替换文本）。
func (p *Profile) validateFilters() *Error {
	for i, name := range p.Filters.Disable {
		if strings.TrimSpace(name) == "" {
			return paramErr("filters.disable 的第 %d 项不能为空", i+1)
		}
		if utf8.RuneCountInString(name) > MaxNameRunes {
			return paramErr("filters.disable 的第 %d 项不能超过 %d 字", i+1, MaxNameRunes)
		}
	}
	for i := range p.Filters.Extra {
		r := &p.Filters.Extra[i]
		label := fmt.Sprintf("第 %d 条过滤规则", i+1)
		if err := checkRuleName(r.Name, label); err != nil {
			return err
		}
		if err := validatePattern(r.Pattern, label+"的正则"); err != nil {
			return err
		}
		if utf8.RuneCountInString(r.Replace) > MaxPatternRunes {
			return paramErr("%s的 replace 不能超过 %d 字", label, MaxPatternRunes)
		}
	}
	return p.validateRuleCount()
}

// validateRuleCount 限制单份规则包的规则总数（过滤 + 切章 + 元数据）。
func (p *Profile) validateRuleCount() *Error {
	total := len(p.Filters.Extra) + len(p.Split.Rules) + len(p.Meta)
	if total > MaxRulesPerProfile {
		return paramErr("每份规则包的规则总数（filters.extra + split.rules + meta）最多 %d 条（收到 %d 条）",
			MaxRulesPerProfile, total)
	}
	return nil
}

// validateMeta 校验元数据规则：正则、捕获组序号、至少指定一个目标字段。
func (p *Profile) validateMeta() *Error {
	for i := range p.Meta {
		m := &p.Meta[i]
		label := fmt.Sprintf("第 %d 条元数据规则", i+1)
		if err := checkRuleName(m.Name, label); err != nil {
			return err
		}
		if err := validatePattern(m.Pattern, label+"的正则"); err != nil {
			return err
		}
		if m.Title == 0 && m.Author == 0 && m.Intro == 0 {
			return paramErr("%s至少要指定 title/author/intro 其中一个捕获组", label)
		}
		if bad := badCapture(m); bad != "" {
			return paramErr("%s的 %s 捕获组序号必须在 0~%d 之间", label, bad, maxCaptureIndex)
		}
	}
	return nil
}

// badCapture 返回越界的捕获组字段名（没有越界返回空串）。
func badCapture(m *MetaRule) string {
	switch {
	case m.Title < 0 || m.Title > maxCaptureIndex:
		return "title"
	case m.Author < 0 || m.Author > maxCaptureIndex:
		return "author"
	case m.Intro < 0 || m.Intro > maxCaptureIndex:
		return "intro"
	}
	return ""
}

// validateSplit 校验切章层：强制策略名与行级规则。
func (p *Profile) validateSplit() *Error {
	if s := p.Split.ForceSplitter; s != "" {
		if utf8.RuneCountInString(s) > MaxNameRunes {
			return paramErr("split.forceSplitter 不能超过 %d 字符", MaxNameRunes)
		}
		if !idPattern.MatchString(s) {
			return paramErr("split.forceSplitter %q 只能包含小写字母、数字和连字符", s)
		}
	}
	for i := range p.Split.Rules {
		r := &p.Split.Rules[i]
		label := fmt.Sprintf("第 %d 条切章规则", i+1)
		if err := checkRuleName(r.Name, label); err != nil {
			return err
		}
		if err := validatePattern(r.Pattern, label+"的正则"); err != nil {
			return err
		}
	}
	return nil
}

// validateTags 校验标签词表：条数、标签名、关键词数量与长度、同名标签。
func (p *Profile) validateTags() *Error {
	if len(p.Tags) > MaxRulesPerProfile {
		return paramErr("每份规则包的标签词表最多 %d 条（收到 %d 条）", MaxRulesPerProfile, len(p.Tags))
	}
	seen := make(map[string]bool, len(p.Tags))
	for i := range p.Tags {
		if err := validateTagRule(&p.Tags[i], i, seen); err != nil {
			return err
		}
	}
	return nil
}

// validateTagRule 校验一条标签规则；seen 用于拒绝同名标签（与词表规则同口径）。
func validateTagRule(t *TagRule, pos int, seen map[string]bool) *Error {
	if t.Tag == "" {
		return paramErr("第 %d 条标签规则的名字不能为空", pos+1)
	}
	if utf8.RuneCountInString(t.Tag) > MaxTagRunes {
		return paramErr("第 %d 条标签规则的名字不能超过 %d 字", pos+1, MaxTagRunes)
	}
	if seen[t.Tag] {
		return paramErr("标签 %q 出现了两次", t.Tag)
	}
	seen[t.Tag] = true
	if len(t.Keywords) == 0 {
		return paramErr("第 %d 条标签规则（%s）至少要有一个关键词", pos+1, t.Tag)
	}
	if len(t.Keywords) > MaxKeywords {
		return paramErr("第 %d 条标签规则（%s）的关键词不能超过 %d 个", pos+1, t.Tag, MaxKeywords)
	}
	for _, kw := range t.Keywords {
		if kw == "" {
			return paramErr("第 %d 条标签规则（%s）的关键词不能为空", pos+1, t.Tag)
		}
		if utf8.RuneCountInString(kw) > MaxKeywordRunes {
			return paramErr("第 %d 条标签规则（%s）的关键词不能超过 %d 字", pos+1, t.Tag, MaxKeywordRunes)
		}
	}
	return nil
}

// validateLimits 校验两个业务上限（负数与超大值都拒绝，不静默夹紧）。
func (p *Profile) validateLimits() *Error {
	if v := p.Limits.MaxChapterChars; v < 0 || v > maxChapterChars {
		return paramErr("limits.maxChapterChars 必须在 0~%d 之间（0 表示不限制）", maxChapterChars)
	}
	if v := p.Limits.MinChapters; v < 0 || v > maxMinChapters {
		return paramErr("limits.minChapters 必须在 0~%d 之间", maxMinChapters)
	}
	return nil
}

// validatePattern 校验一条正则：空/超长 → invalid_param，编译失败 → invalid_pattern。
func validatePattern(pattern, label string) *Error {
	if strings.TrimSpace(pattern) == "" {
		return paramErr("%s不能为空", label)
	}
	if utf8.RuneCountInString(pattern) > MaxPatternRunes {
		return paramErr("%s不能超过 %d 字", label, MaxPatternRunes)
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return &Error{Code: CodePattern, Message: fmt.Sprintf("%s非法：%v", label, err)}
	}
	return nil
}

// validateOptionalPattern 校验可选正则：留空表示「不启用这条匹配」，不报错。
func validateOptionalPattern(pattern, label string) *Error {
	if strings.TrimSpace(pattern) == "" {
		return nil
	}
	return validatePattern(pattern, label)
}

// checkRuleName 校验规则名：不能为空、不超过 MaxNameRunes。
func checkRuleName(name, label string) *Error {
	if name == "" {
		return paramErr("%s的名称不能为空", label)
	}
	if utf8.RuneCountInString(name) > MaxNameRunes {
		return paramErr("%s的名称不能超过 %d 字", label, MaxNameRunes)
	}
	return nil
}

// displayID 用于错误消息里的身份展示（id 为空时也能读）。
func displayID(id string) string {
	if id == "" {
		return "未命名"
	}
	return id
}

// paramErr 构造一条 invalid_param 校验错误。
func paramErr(format string, args ...any) *Error {
	return &Error{Code: CodeParam, Message: fmt.Sprintf(format, args...)}
}
