package api

import (
	"net/http"

	"norvels/internal/store"
)

// AI 审核开关（设计 §6）：settings 里存 ai.enabled，API key 只从环境变量进内存。
//
// 为什么开关落库而 key 不落库：开关要能在不重启、不改部署的情况下关掉；key 是
// 机密，只经 NORVELS_AI_API_KEY 进进程（见 docs/deploy.md）。所以 GET 只回一个
// 派生的 apiKeyConfigured 布尔值，永不回显 key 本身。

// aiSettingsBody 是 GET 响应与 PUT 请求体（PUT 只看 enabled）。
type aiSettingsBody struct {
	Enabled bool `json:"enabled"`
	// APIKeyConfigured 是派生值（配置里有没有 key），只在 GET 响应里有意义。
	APIKeyConfigured bool `json:"apiKeyConfigured"`
}

// handleGetAISettings 处理 GET /api/settings/ai。
//
// 读库失败不 500：与去重阈值同理，开关读不懂按「关」返回（与 store.AIEnabled 口径
// 一致——AI 是可选增强，读不懂的配置不该把正文外发出去），只记一条日志。
func (s *Server) handleGetAISettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, aiSettingsBody{
		Enabled:          s.st.AIEnabled(r.Context()),
		APIKeyConfigured: s.cfg.AIAPIKey != "",
	})
}

// handlePutAISettings 处理 PUT /api/settings/ai（写库即生效，下一次审核任务就按新开关跑）。
//
// 开 enabled 但没配 key 时**仍返回 204**：前端拿 GET 的 apiKeyConfigured=false 提示
// 「没有 API key，开启也不会调用」。没 key 是部署缺项，不是请求非法——在这里 400
// 会让「先开关后配 key」的部署顺序直接卡住。
func (s *Server) handlePutAISettings(w http.ResponseWriter, r *http.Request) {
	var body aiSettingsBody
	if !decodeJSONBody(w, r, &body, "请求体必须是 {\"enabled\":true}") {
		return
	}
	value := "false"
	if body.Enabled {
		value = "true"
	}
	if err := s.st.PutSetting(r.Context(), store.SettingKeyAIEnabled, value); err != nil {
		s.fail(w, r, err, "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
