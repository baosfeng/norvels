package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"norvels/internal/booksource"
	"norvels/internal/fetch"
	"norvels/internal/ruleimport"
	"norvels/internal/source"
)

// sourceImportBodyLimit 是 Legado 书源 JSON 的请求体上限：一次导入几十个源
// 约 100–300 KiB，2 MiB 留足余量。
const sourceImportBodyLimit = 2 << 20

// sourceImportRequest 是 POST /api/import/source-profiles/import 的请求体。
// apply 缺省 false：只预览，不落库。
type sourceImportRequest struct {
	Raw   string `json:"raw"`
	Apply bool   `json:"apply"`
}

// sourceImportResponse 是预览/应用的响应。
type sourceImportResponse struct {
	Applied     int `json:"applied"`
	Overwritten int `json:"overwritten"`
	// Via 是输入形态：text（直接粘贴）/ url / legado / yuedu（从链接取回）。
	Via     string                   `json:"via"`
	Skipped []skippedIssue           `json:"skipped"`
	Results []booksource.ImportIssue `json:"results"`
}

// skippedIssue 是 apply 时被跳过的书源（音频/图片/文件源、缺 url、@js 硬字段）。
// 它同时出现在 results 里，这里单独列一份方便前端直接展示「跳过了哪些」。
type skippedIssue struct {
	Index  int    `json:"index"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// handleImportLegadoSources 处理 POST /api/import/source-profiles/import。
//
// 先预览后应用：apply=false 只返回逐条结果；apply=true 时**跳过不可用条目**
// （音频/图片/文件源、缺 url、@js 硬字段），只把可用书源（ok/partial）与库里
// 已有规则包合并（同 id 覆盖、local 保留），走与 PUT 相同的校验后落库。
//
// 整包导入的真实场景必然混着漫画/音频源，所以「一条坏的不该拖垮整批」；
// 只有一条可用的都没有时才 400（消息说明原因见 results）。
func (s *Server) handleImportLegadoSources(w http.ResponseWriter, r *http.Request) {
	var body sourceImportRequest
	if !decodeJSONBodyLimit(w, r, &body,
		`请求体必须是 {"raw":"…","apply":false}`, sourceImportBodyLimit) {
		return
	}
	if strings.TrimSpace(body.Raw) == "" {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "raw 不能为空（粘贴 Legado 书源 JSON）")
		return
	}
	// 先取回：raw 可以是 JSON 文本，也可以是 http(s) / legado:// / yuedu:// 链接。
	fetched, ferr := ruleimport.Fetch(r.Context(), body.Raw, s.cfg.RuleImportOptions)
	if ferr != nil {
		writeError(w, http.StatusBadRequest, codeInvalidParam, ruleImportErrorMessage(ferr))
		return
	}
	profiles, issues, err := booksource.ParseLegado(fetched.JSON)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidParam, err.Error())
		return
	}
	resp := sourceImportResponse{Via: fetched.Via, Results: issues}
	if !body.Apply {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(profiles) == 0 {
		writeError(w, http.StatusBadRequest, codeInvalidParam,
			fmt.Sprintf("%d 条输入里没有可导入的书源（ok/partial），原因见 results", len(issues)))
		return
	}
	applied, overwritten, verr, err := s.mergeLegadoSources(r.Context(), profiles)
	if err != nil {
		s.fail(w, r, err, "")
		return
	}
	if verr != nil {
		s.badRequest(w, verr, string(verr.Code))
		return
	}
	resp.Applied, resp.Overwritten = applied, overwritten
	resp.Skipped = collectSkipped(issues)
	writeJSON(w, http.StatusOK, resp)
}

// ruleImportErrorMessage 把取回错误翻成用户可读的 400 消息。
//
// ErrUnsupportedInput 的消息本身已列出支持的形态（JSON 文本 / http(s) /
// legado:// / yuedu://），直接透出；其余按安全与容量语义给固定文案。
func ruleImportErrorMessage(err error) string {
	switch {
	case errors.Is(err, ruleimport.ErrUnsupportedInput):
		return err.Error()
	case errors.Is(err, fetch.ErrBlocked):
		return "目标地址不允许访问（只允许公网 http/https 的普通端口）"
	case errors.Is(err, ruleimport.ErrTooLarge):
		return "取回内容超过上限（1 MiB），请先精简书源"
	case errors.Is(err, ruleimport.ErrNotJSON):
		return "取回内容里没有可用的 JSON（页面里也没找到内嵌的 JSON）"
	default:
		return "取回失败：" + err.Error()
	}
}

// collectSkipped 列出 apply 时被跳过的书源（书源类且状态不可落库）。
// 净化 / TXT 规则与未知条目不是书源，不进 skipped——它们在 results 里可见。
func collectSkipped(issues []booksource.ImportIssue) []skippedIssue {
	out := make([]skippedIssue, 0)
	for _, issue := range issues {
		if issue.Type != booksource.TypeBookSource || booksource.Applicable(issue.Status) {
			continue
		}
		out = append(out, skippedIssue{Index: issue.Index, ID: issue.ID, Name: issue.Name,
			Type: issue.Type, Status: issue.Status, Reason: issue.Reason})
	}
	return out
}

// mergeLegadoSources 把导入的规则包与库里已有规则包合并后落库。
//
// 同 id 覆盖、其余（含 local 规则包）保留；合并结果走与 PUT 相同的归一化与
// 校验（≤20 份、id 唯一、正则合法），校验失败不落库。
func (s *Server) mergeLegadoSources(ctx context.Context, incoming []source.Profile) (
	applied, overwritten int, verr *source.Error, err error) {
	existing, err := s.st.ListSourceProfiles(ctx)
	if err != nil {
		return 0, 0, nil, err
	}
	incomingIDs := make(map[string]bool, len(incoming))
	for _, p := range incoming {
		incomingIDs[p.ID] = true
	}
	merged := make([]source.Profile, 0, len(existing)+len(incoming))
	for _, p := range existing {
		if incomingIDs[p.ID] {
			overwritten++
			continue
		}
		merged = append(merged, p)
	}
	merged = append(merged, incoming...)
	normalized := source.Normalize(merged)
	if _, cerr := source.CheckAll(normalized); cerr != nil {
		return 0, 0, cerr, nil
	}
	if err := s.st.SaveSourceProfiles(ctx, normalized); err != nil {
		return 0, 0, nil, err
	}
	return len(incoming), overwritten, nil, nil
}
