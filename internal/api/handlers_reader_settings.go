package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type readerSettingsResponse struct {
	Settings json.RawMessage `json:"settings"`
}

func (s *Server) handleGetReaderSettings(w http.ResponseWriter, r *http.Request) {
	raw, ok, err := s.st.GetReaderSettings(r.Context())
	if err != nil {
		s.fail(w, r, err, "阅读器设置不可用")
		return
	}
	var payload json.RawMessage
	if ok && strings.TrimSpace(raw) != "" {
		payload = json.RawMessage(raw)
	} else {
		payload = json.RawMessage("null")
	}
	writeJSON(w, http.StatusOK, readerSettingsResponse{Settings: payload})
}

func (s *Server) handlePutReaderSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if !decodeJSONBody(w, r, &body, "请求体必须是 JSON 对象") {
		return
	}
	var payload json.RawMessage
	if v, ok := body["settings"]; ok {
		payload = v
	} else {
		b, err := json.Marshal(body)
		if err != nil {
			s.badRequest(w, err, codeInvalidParam)
			return
		}
		payload = b
	}
	str := strings.TrimSpace(string(payload))
	if str == "" {
		s.badRequest(w, fmt.Errorf("empty"), codeInvalidParam)
		return
	}
	var probe any
	if err := json.Unmarshal(payload, &probe); err != nil {
		s.badRequest(w, badParam("设置必须是合法 JSON"), codeInvalidParam)
		return
	}
	if err := s.st.PutReaderSettings(r.Context(), string(payload)); err != nil {
		s.fail(w, r, err, "阅读器设置保存失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
