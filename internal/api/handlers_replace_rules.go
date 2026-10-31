package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"norvels/internal/parser"
	"norvels/internal/source"
	"norvels/internal/store"
)

// 净化规则的跨边界上限。
const (
	maxCleanRules        = 100
	maxCleanPatternRunes = 200
	maxCleanNameRunes    = 60
	maxPreviewTextBytes  = 1 << 20
)

// cleanRulesBody 是 GET 响应与 PUT 请求体（结构相同，全量替换）。
type cleanRulesBody struct {
	Rules []store.CleanRule `json:"rules"`
}

// handleGetReplaceRules 处理 GET /api/settings/replace-rules。
func (s *Server) handleGetReplaceRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.st.ListCleanRules(r.Context())
	if err != nil {
		s.fail(w, r, err, "净化规则不可用")
		return
	}
	if rules == nil {
		rules = []store.CleanRule{}
	}
	writeJSON(w, http.StatusOK, cleanRulesBody{Rules: rules})
}

// handlePutReplaceRules 处理 PUT /api/settings/replace-rules：全量替换，成功 204。
func (s *Server) handlePutReplaceRules(w http.ResponseWriter, r *http.Request) {
	var body cleanRulesBody
	if !decodeJSONBody(w, r, &body, "请求体必须是 {\"rules\":[{\"name\":\"…\",\"pattern\":\"…\"}]}") {
		return
	}
	rules, code, err := normalizeCleanRules(body.Rules)
	if err != nil {
		s.badRequest(w, err, code)
		return
	}
	if err := s.st.SaveCleanRules(r.Context(), rules); err != nil {
		s.fail(w, r, err, "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// normalizeCleanRules 校验并归一化整批规则：条数 → 长度 → 正则可编译 → 分配 ID。
func normalizeCleanRules(rules []store.CleanRule) ([]store.CleanRule, string, error) {
	if len(rules) > maxCleanRules {
		return nil, codeInvalidParam, badParam("净化规则最多 %d 条", maxCleanRules)
	}
	out := make([]store.CleanRule, 0, len(rules))
	for i, rule := range rules {
		pattern := strings.TrimSpace(rule.Pattern)
		if pattern == "" {
			return nil, codeInvalidParam, badParam("第 %d 条规则的模式不能为空", i+1)
		}
		if utf8.RuneCountInString(pattern) > maxCleanPatternRunes {
			return nil, codeInvalidParam, badParam("第 %d 条规则的模式不能超过 %d 字", i+1, maxCleanPatternRunes)
		}
		if _, _, err := cleanPattern(rule); err != nil {
			return nil, codeInvalidPattern, badParam("第 %d 条规则的模式非法：%v", i+1, err)
		}
		rule.Pattern = pattern
		rule.Name = cleanRuleName(rule.Name, i)
		if rule.ID <= 0 {
			rule.ID = i + 1
		}
		out = append(out, rule)
	}
	return out, "", nil
}

// cleanRuleName 归一化规则名：留空补「规则N」，超长截断（名字只用于展示）。
func cleanRuleName(raw string, pos int) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = fmt.Sprintf("规则%d", pos+1)
	}
	if r := []rune(name); len(r) > maxCleanNameRunes {
		return string(r[:maxCleanNameRunes])
	}
	return name
}

// cleanPattern 返回规则生效用的正则与替换串：IsRegex=false 时把字面量转义。
func cleanPattern(rule store.CleanRule) (*regexp.Regexp, string, error) {
	pattern := rule.Pattern
	if !rule.IsRegex {
		pattern = regexp.QuoteMeta(pattern)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, "", err
	}
	return re, rule.Replacement, nil
}

// loadCleanRules 读取全局净化规则并编译成 parser.Filter（按 order 升序）。
//
// 单条编译失败只跳过并记日志：库可能被手改过，一条坏规则不该让整次导入失败。
func (s *Server) loadCleanRules(ctx context.Context) []parser.Filter {
	stored, err := s.st.ListCleanRules(ctx)
	if err != nil {
		s.logf("api: 读取净化规则失败，本次导入只用内置过滤器: %v", err)
		return nil
	}
	return cleanFilters(stored)
}

// cleanFilters 把启用的规则编译成 Filter，按 order 升序（内置过滤器仍排在最前）。
func cleanFilters(rules []store.CleanRule) []parser.Filter {
	enabled := make([]store.CleanRule, 0, len(rules))
	for _, r := range rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	sortCleanRules(enabled)
	out := make([]parser.Filter, 0, len(enabled))
	for _, r := range enabled {
		re, repl, err := cleanPattern(r)
		if err != nil {
			continue
		}
		f, err := parser.NewDeclFilter(r.Name, r.Order, re.String(), repl)
		if err != nil {
			continue
		}
		out = append(out, f)
	}
	return out
}

// withCleanRules 把全局净化规则前置进每份规则包的 filters.extra（保持全局在前）。
func withCleanRules(profiles []source.Profile, rules []store.CleanRule) []source.Profile {
	enabled := make([]store.CleanRule, 0, len(rules))
	for _, r := range rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	if len(enabled) == 0 {
		return profiles
	}
	sortCleanRules(enabled)
	out := make([]source.Profile, 0, len(profiles))
	for _, p := range profiles {
		extra := make([]source.FilterRule, 0, len(enabled)+len(p.Filters.Extra))
		for _, r := range enabled {
			re, _, err := cleanPattern(r)
			if err != nil {
				continue
			}
			extra = append(extra, source.FilterRule{Name: r.Name, Priority: r.Order, Pattern: re.String(), Replace: r.Replacement})
		}
		p.Filters.Extra = append(extra, p.Filters.Extra...)
		out = append(out, p)
	}
	return out
}

// sortCleanRules 按 order 升序、同 order 保持原顺序。
func sortCleanRules(rules []store.CleanRule) {
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Order < rules[j].Order })
}

// handlePreviewReplaceRules 处理 POST /api/settings/replace-rules/preview：试跑不落库。
func (s *Server) handlePreviewReplaceRules(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if !decodeJSONBody(w, r, &body, "请求体必须是 {\"text\":\"…\"}") {
		return
	}
	if len(body.Text) > maxPreviewTextBytes {
		s.badRequest(w, badParam("试跑文本不能超过 %d 字节", maxPreviewTextBytes), codeInvalidParam)
		return
	}
	rules, err := s.st.ListCleanRules(r.Context())
	if err != nil {
		s.fail(w, r, err, "净化规则不可用")
		return
	}
	after, hits := applyCleanRules(body.Text, rules)
	writeJSON(w, http.StatusOK, map[string]any{
		"before": body.Text,
		"after":  after,
		"hits":   hits,
	})
}

// cleanHit 是一条规则在试跑文本上的命中情况。
type cleanHit struct {
	RuleID int    `json:"ruleId"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
}

// applyCleanRules 依次应用启用的规则，返回替换后文本与每条规则的命中数。
func applyCleanRules(text string, rules []store.CleanRule) (string, []cleanHit) {
	enabled := make([]store.CleanRule, 0, len(rules))
	for _, r := range rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	sortCleanRules(enabled)
	out := text
	hits := make([]cleanHit, 0, len(enabled))
	for _, r := range enabled {
		re, repl, err := cleanPattern(r)
		if err != nil {
			continue
		}
		count := len(re.FindAllString(out, -1))
		out = re.ReplaceAllString(out, repl)
		hits = append(hits, cleanHit{RuleID: r.ID, Name: r.Name, Count: count})
	}
	return out, hits
}
