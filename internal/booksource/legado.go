// Package booksource 解析 Legado（开源阅读）的规则 JSON：书源、替换净化规则
// 与 TXT 章节规则。
//
// 本包是通用导入解析器：按字段自动识别条目类型（见 Type* 常量）。书源会被适配
// 成 source.Profile（kind=online），净化规则与 TXT 规则本期只识别、不落库。
// 不发起任何网络请求（取回 JSON 由 internal/ruleimport 负责），也不执行规则串；
// @js 规则由 internal/ruleexec 在编译期标记降级。
package booksource

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"norvels/internal/ruleexec"
	"norvels/internal/source"
)

// 条目的处理结果。ok / partial / unusable 可入库（后两者带可见的降级标记），
// invalid / unsupported_type 明确拒绝并说明原因；recognized 表示已识别但本期
// 不落库；unknown_type 表示无法判定类型。都不静默吞掉。
const (
	StatusOK              = "ok"
	StatusPartial         = "partial"
	StatusUnusable        = "unusable"
	StatusInvalid         = "invalid"
	StatusUnsupportedType = "unsupported_type"
	StatusRecognized      = "recognized"
	StatusUnknownType     = "unknown_type"
)

// 条目类型（前端预览表按它显示「书源 / 净化 / TXT 规则 / 未知」）。
const (
	TypeBookSource  = "book_source"
	TypeReplaceRule = "replace_rule"
	TypeTXTTocRule  = "txt_toc_rule"
	TypeUnknown     = "unknown"
)

// ImportIssue 是一条输入的导入结果（与输入条目一一对应）。
type ImportIssue struct {
	Index             int      `json:"index"`
	Type              string   `json:"type"`
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Status            string   `json:"status"`
	DegradedFields    []string `json:"degradedFields"`
	UnsupportedFields []string `json:"unsupportedFields"`
	Reason            string   `json:"reason"`
}

// legadoSource 是 Legado 书源对象里本适配器关心的字段（其余字段忽略）。
type legadoSource struct {
	BookSourceURL   string            `json:"bookSourceUrl"`
	BookSourceName  string            `json:"bookSourceName"`
	BookSourceGroup string            `json:"bookSourceGroup"`
	BookSourceType  int               `json:"bookSourceType"`
	Enabled         *bool             `json:"enabled"`
	CustomOrder     int               `json:"customOrder"`
	Weight          int               `json:"weight"`
	SearchURL       string            `json:"searchUrl"`
	ExploreURL      string            `json:"exploreUrl"`
	BookURLPattern  string            `json:"bookUrlPattern"`
	RuleSearch      map[string]string `json:"ruleSearch"`
	RuleExplore     map[string]string `json:"ruleExplore"`
	RuleBookInfo    map[string]string `json:"ruleBookInfo"`
	RuleToc         map[string]string `json:"ruleToc"`
	RuleContent     map[string]string `json:"ruleContent"`
	Header          map[string]string `json:"header"`
	LoginURL        string            `json:"loginUrl"`
	Cookie          string            `json:"cookie"`
	WebJS           string            `json:"webJs"`
}

// ParseLegado 解析 Legado 书源 JSON，返回可入库的规则包与逐条结果。
//
// 接受三种打包形态：单个书源对象、JSON 数组、{"bookSources":[...]}（兼容
// {"sources":[...]}）。被拒绝的条目只出现在 issues 里，不出现在 profiles 里。
func ParseLegado(raw []byte) ([]source.Profile, []ImportIssue, error) {
	entries, err := splitEnvelope(raw)
	if err != nil {
		return nil, nil, err
	}
	if len(entries) == 0 {
		return nil, nil, fmt.Errorf("booksource: 没有可导入的书源")
	}
	profiles := make([]source.Profile, 0, len(entries))
	issues := make([]ImportIssue, 0, len(entries))
	for i, entry := range entries {
		p, issue := parseEntry(entry, i)
		issues = append(issues, issue)
		if p != nil {
			profiles = append(profiles, *p)
		}
	}
	return profiles, issues, nil
}

// parseEntry 识别条目类型并分派：书源走完整适配，净化 / TXT 规则只识别，
// 无法判定的条目返回 unknown_type（不静默丢）。
func parseEntry(raw json.RawMessage, index int) (*source.Profile, ImportIssue) {
	typ, err := detectType(raw)
	if err != nil {
		return nil, newIssue(index, TypeUnknown, StatusUnknownType, "不是合法的 JSON 对象："+err.Error())
	}
	switch typ {
	case TypeBookSource:
		return convertBookSource(raw, index)
	case TypeReplaceRule:
		return nil, newIssue(index, typ, StatusRecognized, "替换净化规则已识别，本期不落库")
	case TypeTXTTocRule:
		return nil, newIssue(index, typ, StatusRecognized, "TXT 章节规则已识别，本期不落库")
	default:
		return nil, newIssue(index, TypeUnknown, StatusUnknownType, "无法识别的规则类型")
	}
}

// detectType 按字段判定条目类型：bookSourceUrl 优先；pattern+replacement 是
// 替换净化规则；rule / pattern 是 TXT 章节规则；其余无法判定。
func detectType(raw json.RawMessage) (string, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", err
	}
	if hasBookSourceField(probe) {
		return TypeBookSource, nil
	}
	_, hasPattern := probe["pattern"]
	_, hasReplacement := probe["replacement"]
	if hasPattern && hasReplacement {
		return TypeReplaceRule, nil
	}
	_, hasRule := probe["rule"]
	if hasRule || hasPattern {
		return TypeTXTTocRule, nil
	}
	return TypeUnknown, nil
}

// hasBookSourceField 判断条目是否带书源特征字段（含缺 bookSourceUrl 的书源：
// 这类条目要报「缺少 bookSourceUrl」而不是「无法识别」）。
func hasBookSourceField(probe map[string]json.RawMessage) bool {
	for _, key := range []string{"bookSourceUrl", "bookSourceName", "bookSourceGroup",
		"ruleSearch", "ruleExplore", "ruleBookInfo", "ruleToc", "ruleContent"} {
		if _, ok := probe[key]; ok {
			return true
		}
	}
	return false
}

// Applicable 报告该状态的书源是否可落库：只有 ok / partial 可导入；
// unusable 的关键字段含 @js，抓取必失败，apply 时跳过并在 results 里回显原因。
func Applicable(status string) bool {
	return status == StatusOK || status == StatusPartial
}

// newIssue 构造一条带空切片的结果（避免 JSON 出现 null）。
func newIssue(index int, typ, status, reason string) ImportIssue {
	return ImportIssue{Index: index, Type: typ, Status: status, Reason: reason,
		DegradedFields: []string{}, UnsupportedFields: []string{}}
}

// splitEnvelope 把三种打包形态拆成条目列表。
func splitEnvelope(raw []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("booksource: 书源 JSON 不能为空")
	}
	if trimmed[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, fmt.Errorf("booksource: 解析书源数组失败: %w", err)
		}
		return arr, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, fmt.Errorf("booksource: 解析书源 JSON 失败: %w", err)
	}
	for _, key := range []string{"bookSources", "sources"} {
		list, ok := obj[key]
		if !ok {
			continue
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(list, &arr); err != nil {
			return nil, fmt.Errorf("booksource: %s 必须是数组: %w", key, err)
		}
		return arr, nil
	}
	return []json.RawMessage{trimmed}, nil
}

// convertBookSource 把一条书源转成规则包；被拒绝时返回 nil 与带原因的结果。
func convertBookSource(raw json.RawMessage, index int) (*source.Profile, ImportIssue) {
	issue := newIssue(index, TypeBookSource, StatusInvalid, "")
	var ls legadoSource
	if err := json.Unmarshal(raw, &ls); err != nil {
		issue.Reason = "不是合法的书源对象：" + err.Error()
		return nil, issue
	}
	ls.BookSourceURL = strings.TrimSpace(ls.BookSourceURL)
	issue.Name = strings.TrimSpace(ls.BookSourceName)
	if ls.BookSourceURL == "" {
		issue.Reason = "缺少 bookSourceUrl"
		return nil, issue
	}
	issue.ID = SourceID(ls.BookSourceURL)
	if !isHTTPURL(ls.BookSourceURL) {
		issue.Reason = "bookSourceUrl 必须以 http:// 或 https:// 开头"
		return nil, issue
	}
	if ls.BookSourceType != 0 {
		issue.Status = StatusUnsupportedType
		issue.Reason = fmt.Sprintf("bookSourceType=%d：只支持文本书源（音频/图片/文件源暂不支持）",
			ls.BookSourceType)
		return nil, issue
	}
	// 请求头来自外部配置：先清洗（CR/LF 值、Host/Content-Length 等键会被丢弃），
	// 再进入编译与落库；被丢弃的键在结果里可见。
	cleanHeader, rejectedHeaders := source.SanitizeHeaders(ls.Header)
	ls.Header = cleanHeader
	issue.UnsupportedFields = unsupportedFields(ls)
	// 先把 Legado 简写翻译成标准 CSS，再编译：status 反映的是真正会落库、会执行的规则。
	convFields, convReasons := convertLegadoRules(&ls)
	status, degraded, reason := compileRules(ls)
	issue.Status = status
	issue.DegradedFields = mergeNames(degraded, convFields)
	issue.Reason = reason
	if len(convFields) > 0 {
		// 转换不了的字段保留原文：该字段大概率抽不出数据，按 partial 如实告知，但不拦入库。
		if issue.Status == StatusOK {
			issue.Status = StatusPartial
		}
		issue.Reason = joinReason(reason, "部分规则含无法转换的 Legado 简写，已保留原文："+
			strings.Join(convReasons, "、"))
	}
	if len(rejectedHeaders) > 0 {
		if issue.Status == StatusOK {
			issue.Status = StatusPartial
		}
		issue.Reason = joinReason(issue.Reason, "部分请求头不安全已丢弃："+
			strings.Join(rejectedHeaders, "、"))
	}
	if !Applicable(issue.Status) {
		// unusable（@js 硬字段）不产出可落库的规则包：抓取必失败，apply 时跳过。
		return nil, issue
	}
	p := buildProfile(ls, issue.ID, issue.UnsupportedFields)
	return &p, issue
}

// buildProfile 把 Legado 字段映射成 norvels 的规则包。
func buildProfile(ls legadoSource, id string, unsupported []string) source.Profile {
	enabled := true
	if ls.Enabled != nil {
		enabled = *ls.Enabled
	}
	name := strings.TrimSpace(ls.BookSourceName)
	if name == "" {
		name = ls.BookSourceURL
	}
	return source.Profile{
		ID: id, Name: name, SchemaVersion: source.SchemaVersion,
		Enabled: enabled, Priority: ls.CustomOrder, Kind: source.KindOnline,
		Online: &source.Online{
			BookSourceURL:     ls.BookSourceURL,
			BookSourceType:    ls.BookSourceType,
			Group:             strings.TrimSpace(ls.BookSourceGroup),
			Weight:            ls.Weight,
			SearchURL:         strings.TrimSpace(ls.SearchURL),
			ExploreURL:        strings.TrimSpace(ls.ExploreURL),
			BookURLPattern:    strings.TrimSpace(ls.BookURLPattern),
			RuleSearch:        ls.RuleSearch,
			RuleExplore:       ls.RuleExplore,
			RuleBookInfo:      ls.RuleBookInfo,
			RuleToc:           ls.RuleToc,
			RuleContent:       ls.RuleContent,
			Header:            ls.Header,
			UnsupportedFields: unsupported,
		},
	}
}

// compileRules 编译全部规则块，返回状态、降级字段与给用户看的原因。
//
// 五个规则块交给 ruleexec.CompileFields（它按字段名区分 hard / soft）；
// searchUrl / exploreUrl 是 URL 模板而不是选择器，单独判 @js。
func compileRules(ls legadoSource) (string, []string, string) {
	fs, err := ruleexec.CompileFields(collectRuleFields(ls))
	if err != nil {
		return StatusPartial, nil, "部分规则无法编译，已保留原规则但暂不可执行：" + err.Error()
	}
	status := fs.Status()
	degraded := degradedNames(fs.Degraded)
	if hasJS(ls.SearchURL) {
		degraded, status = appendDegraded(degraded, "searchUrl"), StatusUnusable
	}
	if hasJS(ls.ExploreURL) {
		degraded, status = appendDegraded(degraded, "exploreUrl"), StatusUnusable
	}
	sort.Strings(degraded)
	return status, degraded, statusReason(status)
}

// collectRuleFields 把五个规则块摊平成 "块.字段" → 规则串；空规则不参与编译
// （ruleexec 对空串返回错误）。
func collectRuleFields(ls legadoSource) map[string]string {
	fields := make(map[string]string)
	blocks := []struct {
		prefix string
		rules  map[string]string
	}{
		{"ruleSearch", ls.RuleSearch},
		{"ruleExplore", ls.RuleExplore},
		{"ruleBookInfo", ls.RuleBookInfo},
		{"ruleToc", ls.RuleToc},
		{"ruleContent", ls.RuleContent},
	}
	for _, b := range blocks {
		for key, val := range b.rules {
			if strings.TrimSpace(val) == "" {
				continue
			}
			fields[b.prefix+"."+key] = val
		}
	}
	return fields
}

// degradedNames 把 ruleexec 的降级字段转成 "块.字段" 形式。
func degradedNames(list []ruleexec.DegradedField) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		out = append(out, d.Block+"."+d.Field)
	}
	return out
}

// appendDegraded 去重追加一个降级字段。
func appendDegraded(list []string, name string) []string {
	for _, s := range list {
		if s == name {
			return list
		}
	}
	return append(list, name)
}

// hasJS 判断规则串是否含 @js。
func hasJS(rule string) bool { return strings.Contains(rule, "@js") }

// statusReason 给出面向用户的原因（ok 时为空）。
func statusReason(status string) string {
	switch status {
	case StatusUnusable:
		return "关键字段含 @js 规则（如 bookList/chapterList/content），该源暂不可用"
	case StatusPartial:
		return "部分字段含 @js 规则，已降级为不生效"
	}
	return ""
}

// unsupportedFields 收集「保存但不会生效」的字段名（header 已生效，不再列入）。
func unsupportedFields(ls legadoSource) []string {
	out := make([]string, 0, 3)
	if strings.TrimSpace(ls.LoginURL) != "" {
		out = append(out, "loginUrl")
	}
	if strings.TrimSpace(ls.Cookie) != "" {
		out = append(out, "cookie")
	}
	if strings.TrimSpace(ls.WebJS) != "" {
		out = append(out, "webJs")
	}
	return out
}

// isHTTPURL 只接受 http(s) 前缀（与 source 校验同口径）。
func isHTTPURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

// SourceID 从 bookSourceUrl 派生稳定 id：src- + sha256 前 8 位（[a-z0-9-]）。
// 同一 url 永远同一 id，重复导入即覆盖（与 Legado 的唯一键语义一致）。
func SourceID(url string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(url)))
	return "src-" + hex.EncodeToString(sum[:])[:8]
}
