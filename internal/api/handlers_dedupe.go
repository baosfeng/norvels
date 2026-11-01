package api

import (
	"math"
	"net/http"
	"strconv"

	"norvels/internal/store"
)

// 内容去重阈值（设计 §4 / settings 的 dedupe.threshold）：章节命中率 ≥ 该值即视为同一本书。
//
// 为什么单独开一个端点而不是并进某个「大设置」：它是导入行为的唯一旋钮——调高更保守
// （只拦纯重复），调低更激进（相近的版本也拦）。写成 (0,1] 的小数而不是百分比，
// 与库里存的值、与文档口径保持同一个单位。

// dedupeSettingsBody 是 GET 响应与 PUT 请求体（结构相同）。
type dedupeSettingsBody struct {
	Threshold float64 `json:"threshold"`
	// Warning 非空表示库里的值非法、已按缺省值返回（前端可提示重设）。
	// 只在 GET 响应里出现，PUT 不读它。
	Warning string `json:"warning,omitempty"`
}

// handleGetDedupeSettings 处理 GET /api/settings/dedupe。
//
// 库里的值非法时返回**生效值**（缺省 0.95）+ warning，而不是 500：读一个设置项失败
// 就让驾驶舱 500 更糟，用户真正需要知道的是「现在按 0.95 在跑，去把配置改对」。
func (s *Server) handleGetDedupeSettings(w http.ResponseWriter, r *http.Request) {
	threshold, err := s.st.DedupeThreshold(r.Context())
	body := dedupeSettingsBody{Threshold: threshold}
	if err != nil {
		s.logf("api: 读取去重阈值失败，按缺省 %.2f 返回: %v", store.DefaultDedupeThreshold, err)
		body.Warning = err.Error()
	}
	writeJSON(w, http.StatusOK, body)
}

// handlePutDedupeSettings 处理 PUT /api/settings/dedupe（写库即生效，下一次导入就按新阈值判）。
//
// 校验口径：取值 (0, 1]，其它一律 400 invalid_param（不静默夹紧——把 95 悄悄改成 1
// 会让用户以为自己配了 95%，实际从此只拦纯重复）。
func (s *Server) handlePutDedupeSettings(w http.ResponseWriter, r *http.Request) {
	var body dedupeSettingsBody
	if !decodeJSONBody(w, r, &body, "请求体必须是 {\"threshold\":0.95}") {
		return
	}
	if body.Threshold <= 0 || body.Threshold > 1 || math.IsNaN(body.Threshold) {
		s.badRequest(w, badParam("threshold 必须在 (0, 1] 之间（0.95 表示 95%%）"), codeInvalidParam)
		return
	}
	if err := s.st.PutSetting(r.Context(), store.SettingKeyDedupeThreshold,
		strconv.FormatFloat(body.Threshold, 'f', -1, 64)); err != nil {
		s.fail(w, r, err, "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
