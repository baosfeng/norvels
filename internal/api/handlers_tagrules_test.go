package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTagRulesEndpoints GET 空 → PUT 合法 → GET 回显。
func TestTagRulesEndpoints(t *testing.T) {
	env := newEnv(t)
	if rr := env.get("/api/settings/tag-rules"); rr.Code != http.StatusOK {
		t.Fatalf("GET 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	} else if body := decodeBody[tagRulesBody](t, rr); len(body.Rules) != 0 {
		t.Fatalf("未配置时 rules = %+v，期望空", body.Rules)
	}
	payload := `{"rules":[{"tag":"奇幻","keywords":["魔法","精灵"]},{"tag":"科幻","keywords":["飞船"]}]}`
	if rr := env.putTagRules(payload); rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	body := decodeBody[tagRulesBody](t, env.get("/api/settings/tag-rules"))
	if len(body.Rules) != 2 || body.Rules[0].Tag != "奇幻" || len(body.Rules[1].Keywords) != 1 {
		t.Fatalf("GET 回显 = %+v", body.Rules)
	}
	if rr := env.putTagRules(`{"rules":[]}`); rr.Code != http.StatusNoContent {
		t.Fatalf("清空 PUT 状态码 = %d", rr.Code)
	}
}

// TestTagRulesValidation 覆盖条数 / 标签名 / 关键词 / 重名四类非法入参。
func TestTagRulesValidation(t *testing.T) {
	env := newEnv(t)
	tooMany := make([]string, 0, maxTagRules+1)
	for i := 0; i <= maxTagRules; i++ {
		tooMany = append(tooMany, fmt.Sprintf(`{"tag":"标签%d","keywords":["词"]}`, i))
	}
	bad := []string{
		`{"rules":[` + strings.Join(tooMany, ",") + `]}`,
		`{"rules":[{"tag":"  ","keywords":["词"]}]}`,
		fmt.Sprintf(`{"rules":[{"tag":%q,"keywords":["词"]}]}`, strings.Repeat("长", maxTagRuleRunes+1)),
		`{"rules":[{"tag":"奇幻","keywords":[]}]}`,
		`{"rules":[{"tag":"奇幻","keywords":["  "]}]}`,
		`{"rules":[{"tag":"奇幻","keywords":["词"]},{"tag":"奇幻","keywords":["另一个词"]}]}`,
	}
	for _, payload := range bad {
		t.Run("payload", func(t *testing.T) {
			requireError(t, env.putTagRules(payload), http.StatusBadRequest, codeInvalidParam)
		})
	}
}

// putTagRules 发一次 PUT /api/settings/tag-rules。
func (e *testEnv) putTagRules(payload string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodPut, "/api/settings/tag-rules", strings.NewReader(payload), nil)
}
