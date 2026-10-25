package api

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"norvels/internal/store"
)

// 词表规则的跨边界上限（P1-B §2.2）。
const (
	maxTagRules        = 50
	maxTagRuleRunes    = 20
	maxTagRuleKeywords = 100
	maxTagKeywordRunes = 20
)

// tagRulesBody 是 GET 响应与 PUT 请求体（结构相同，全量替换）。
type tagRulesBody struct {
	Rules []store.TagRule `json:"rules"`
}

// loadTagRules 读取词表规则，供导入链路注入（读失败只记日志、不打自动标签）。
func (s *Server) loadTagRules(ctx context.Context) []store.TagRule {
	rules, err := s.st.ListTagRules(ctx)
	if err != nil {
		s.logf("api: 读取词表规则失败，本次导入不打自动标签: %v", err)
		return nil
	}
	return rules
}

// handleGetTagRules 处理 GET /api/settings/tag-rules；未配置时返回 {"rules":[]}。
func (s *Server) handleGetTagRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.st.ListTagRules(r.Context())
	if err != nil {
		s.fail(w, r, err, "词表规则不可用")
		return
	}
	if rules == nil {
		rules = []store.TagRule{}
	}
	writeJSON(w, http.StatusOK, tagRulesBody{Rules: rules})
}

// handlePutTagRules 处理 PUT /api/settings/tag-rules：全量替换，成功 204。
func (s *Server) handlePutTagRules(w http.ResponseWriter, r *http.Request) {
	var body tagRulesBody
	if !decodeJSONBody(w, r, &body, "请求体必须是包含 rules 数组的 JSON 对象") {
		return
	}
	rules, err := normalizeTagRules(body.Rules)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	if err := s.st.SaveTagRules(r.Context(), rules); err != nil {
		s.fail(w, r, err, "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// normalizeTagRules 校验并归一化整批词表规则（条数 / 标签名 / 关键词条数与长度 / 重名）。
func normalizeTagRules(rules []store.TagRule) ([]store.TagRule, error) {
	if len(rules) > maxTagRules {
		return nil, badParam("词表规则最多 %d 条", maxTagRules)
	}
	out := make([]store.TagRule, 0, len(rules))
	seen := make(map[string]bool, len(rules))
	for i, rule := range rules {
		tag := strings.TrimSpace(rule.Tag)
		if tag == "" {
			return nil, badParam("第 %d 条规则的标签名不能为空", i+1)
		}
		if utf8.RuneCountInString(tag) > maxTagRuleRunes {
			return nil, badParam("第 %d 条规则的标签名不能超过 %d 字", i+1, maxTagRuleRunes)
		}
		if seen[tag] {
			return nil, badParam("标签 %q 出现了两次", tag)
		}
		seen[tag] = true
		keywords, err := normalizeKeywords(rule.Keywords, i, tag)
		if err != nil {
			return nil, err
		}
		out = append(out, store.TagRule{Tag: tag, Keywords: keywords})
	}
	return out, nil
}

// normalizeKeywords 校验并归一化一条规则的关键词：去空、去重、条数与长度受限。
func normalizeKeywords(raw []string, ruleIndex int, tag string) ([]string, error) {
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, kw := range raw {
		k := strings.TrimSpace(kw)
		if k == "" || seen[k] {
			continue
		}
		if utf8.RuneCountInString(k) > maxTagKeywordRunes {
			return nil, badParam("规则 %q 的关键词不能超过 %d 字", tag, maxTagKeywordRunes)
		}
		seen[k] = true
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil, badParam("第 %d 条规则（标签 %q）至少要有一个关键词", ruleIndex+1, tag)
	}
	if len(out) > maxTagRuleKeywords {
		return nil, badParam("规则 %q 的关键词不能超过 %d 个", tag, maxTagRuleKeywords)
	}
	return out, nil
}
