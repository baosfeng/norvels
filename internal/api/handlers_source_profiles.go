package api

import (
	"net/http"

	"norvels/internal/source"
)

// sourceProfilesPath 是书源规则包接口路径（GET/PUT 同路径，validate 是子路径）。
const sourceProfilesPath = "/api/import/source-profiles"

// sourceProfilesBodyLimit 是规则包请求体上限：20 份 × 30 条规则 × 200 字正则，
// 最坏约 120 KiB，留一倍余量；不能沿用 64 KiB 的小 JSON 上限。
const sourceProfilesBodyLimit = 1 << 20

// sourceProfilesInvalidMsg 是请求体形状不对时的提示。
const sourceProfilesInvalidMsg = `请求体必须是 {"profiles":[…]}`

// sourceProfilesBody 是 GET 响应与 PUT 请求体（结构相同，全量替换）。
type sourceProfilesBody struct {
	Profiles []source.Profile `json:"profiles"`
}

// sourceValidateResponse 是 POST .../validate 的逐条结果。
type sourceValidateResponse struct {
	Results []source.Issue `json:"results"`
}

// handleGetSourceProfiles 处理 GET /api/import/source-profiles。
//
// 未配置时返回 {"profiles":[]} 而不是 null：前端据此直接渲染空列表。
// 这里不重新校验库里已有的规则包：写入边界已经校验过，读路径只报「库坏了」。
func (s *Server) handleGetSourceProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.st.ListSourceProfiles(r.Context())
	if err != nil {
		s.fail(w, r, err, "来源规则包不可用")
		return
	}
	if profiles == nil {
		profiles = []source.Profile{}
	}
	writeJSON(w, http.StatusOK, sourceProfilesBody{Profiles: profiles})
}

// handlePutSourceProfiles 处理 PUT /api/import/source-profiles：全量替换。
//
// 成功返回 204 空 body（与 PUT /api/settings/chapter-rules 一致）：本次写入没有
// 用户需要的回执。校验在落库前一次做完：份数/id 冲突/条数/正则编译，任一失败
// 整批拒绝、不改库——规则包只允许「全生效」或「全不生效」，没有半套。
func (s *Server) handlePutSourceProfiles(w http.ResponseWriter, r *http.Request) {
	var body sourceProfilesBody
	if !decodeJSONBodyLimit(w, r, &body, sourceProfilesInvalidMsg, sourceProfilesBodyLimit) {
		return
	}
	profiles, verr := validateSourceProfiles(body.Profiles)
	if verr != nil {
		s.badRequest(w, verr, string(verr.Code))
		return
	}
	if err := s.st.SaveSourceProfiles(r.Context(), profiles); err != nil {
		s.fail(w, r, err, "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleValidateSourceProfiles 处理 POST /api/import/source-profiles/validate：
// 只校验不落库，逐份返回 {index,id,ok,reason}，供前端在保存前标红具体那一份。
//
// 全局问题（超过 20 份、id 重复）没有「第几份」可定位，直接 400：
// 返回 400 时前端按整批错误展示，与 PUT 的失败口径完全一致。
func (s *Server) handleValidateSourceProfiles(w http.ResponseWriter, r *http.Request) {
	var body sourceProfilesBody
	if !decodeJSONBodyLimit(w, r, &body, sourceProfilesInvalidMsg, sourceProfilesBodyLimit) {
		return
	}
	profiles := source.Normalize(body.Profiles)
	issues, verr := source.CheckAll(profiles)
	if issues == nil {
		// 整批被拒（超过 20 份）：没有「第几份」可定位，与 PUT 一样回 400。
		s.badRequest(w, verr, string(verr.Code))
		return
	}
	writeJSON(w, http.StatusOK, sourceValidateResponse{Results: issues})
}

// validateSourceProfiles 归一化并校验整批规则包；通过时返回可落库的形态。
//
// 归一化在前、校验在后：用户手写的「.TXT」「txt」或省略 schemaVersion 不该被拒绝，
// 先等价改写再判合法性，错误消息才指向用户真正写错的那一处。
func validateSourceProfiles(profiles []source.Profile) ([]source.Profile, *source.Error) {
	normalized := source.Normalize(profiles)
	if _, err := source.CheckAll(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}
