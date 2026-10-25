package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"norvels/internal/parser"
	"norvels/internal/store"
)

// 自定义章节识别规则的跨边界上限（spec §2.2）：条数与单条正则长度。
const (
	maxChapterRules         = 20
	maxChapterRuleRunes     = 200
	maxChapterRuleNameRunes = 60
)

// loadChapterRules 读取全局章节规则并编译成 parser.Rule，供导入链路注入。
//
// 这是规则唯一的作用点：目录导入与上传导入的 Options.ChapterRules 都由它填。
// 单条编译失败只跳过并记日志、不中断导入：settings 里的规则是 PUT 校验过的，
// 但库可能被手改过，一条坏规则不该让整次导入失败（退化成「这条不生效」）。
//
// 没有任何规则时返回 nil——nil 与「不启用自定义规则」等价，避免空切片改变既有行为。
func (s *Server) loadChapterRules(ctx context.Context) []parser.Rule {
	stored, err := s.st.ListChapterRules(ctx)
	if err != nil {
		s.logf("api: 读取章节规则失败，本次导入只用内置策略: %v", err)
		return nil
	}
	rules := make([]parser.Rule, 0, len(stored))
	for _, rule := range stored {
		pattern, cerr := regexp.Compile(rule.Pattern)
		if cerr != nil {
			s.logf("api: 跳过非法章节规则 name=%q pattern=%q: %v", rule.Name, rule.Pattern, cerr)
			continue
		}
		rules = append(rules, parser.Rule{Name: rule.Name, Pattern: pattern})
	}
	if len(rules) == 0 {
		return nil
	}
	return rules
}

// chapterRulesBody 是 GET 响应与 PUT 请求体（结构相同，全量替换）。
type chapterRulesBody struct {
	Rules []store.ChapterRule `json:"rules"`
}

// handleGetChapterRules 处理 GET /api/settings/chapter-rules。
//
// 未配置时返回 {"rules":[]} 而不是 null：前端据此直接渲染空列表。
func (s *Server) handleGetChapterRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.st.ListChapterRules(r.Context())
	if err != nil {
		s.fail(w, r, err, "章节规则不可用")
		return
	}
	if rules == nil {
		rules = []store.ChapterRule{}
	}
	writeJSON(w, http.StatusOK, chapterRulesBody{Rules: rules})
}

// handlePutChapterRules 处理 PUT /api/settings/chapter-rules：全量替换。
//
// 成功返回 204 空 body（与 PUT /api/reading/progress 一致）：这次写入没有用户需要的
// 回执，前端按「无内容」处理即可；回 200 + JSON 反而会让空 body 被判成无法识别。
//
// 三道校验都在这里做完再落库（正则能编译、条数 ≤20、单条正则 ≤200 字）：
// 跨信任边界的校验只做一次，store 只负责序列化。
func (s *Server) handlePutChapterRules(w http.ResponseWriter, r *http.Request) {
	var body chapterRulesBody
	if !decodeJSONBody(w, r, &body, "请求体必须是 {\"rules\":[{\"name\":\"…\",\"pattern\":\"…\"}]}") {
		return
	}
	rules, code, err := normalizeChapterRules(body.Rules)
	if err != nil {
		s.badRequest(w, err, code)
		return
	}
	if err := s.st.SaveChapterRules(r.Context(), rules); err != nil {
		s.fail(w, r, err, "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// normalizeChapterRules 校验并归一化整批规则。
//
// 返回的 code 是失败时应写给前端的错误码：正则编译失败 → invalid_pattern，
// 条数/长度/空正则 → invalid_param。校验顺序是「先粗后细」：先判条数与长度
// （便宜），再编译正则（贵）。
func normalizeChapterRules(rules []store.ChapterRule) ([]store.ChapterRule, string, error) {
	if len(rules) > maxChapterRules {
		return nil, codeInvalidParam, badParam("章节规则最多 %d 条", maxChapterRules)
	}
	out := make([]store.ChapterRule, 0, len(rules))
	for i, rule := range rules {
		pattern := strings.TrimSpace(rule.Pattern)
		if pattern == "" {
			return nil, codeInvalidParam, badParam("第 %d 条规则的正则不能为空", i+1)
		}
		if utf8.RuneCountInString(pattern) > maxChapterRuleRunes {
			return nil, codeInvalidParam,
				badParam("第 %d 条规则的正则不能超过 %d 字", i+1, maxChapterRuleRunes)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return nil, codeInvalidPattern, badParam("第 %d 条规则的正则非法：%v", i+1, err)
		}
		out = append(out, store.ChapterRule{Name: chapterRuleName(rule.Name, i), Pattern: pattern})
	}
	return out, "", nil
}

// chapterRuleName 归一化规则名：去空白，留空时补「规则N」，超长截断。
//
// 名字只用于前端展示，不参与匹配，所以不拒绝而是补全——用户点「保存」时
// 最不想要的就是因为没起名而被退回。
func chapterRuleName(raw string, pos int) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = fmt.Sprintf("规则%d", pos+1)
	}
	if r := []rune(name); len(r) > maxChapterRuleNameRunes {
		return string(r[:maxChapterRuleNameRunes])
	}
	return name
}
