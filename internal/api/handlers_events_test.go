package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"net/http/httptest"
)

// jsonHeader 是埋点端点的请求头（Content-Type 决定 decodeJSONBody 的解析路径）。
var jsonHeader = map[string]string{"Content-Type": "application/json"}

// postEvents 向 /api/events 发一批事件。
func postEvents(t *testing.T, e *testEnv, body string) *httptest.ResponseRecorder {
	t.Helper()
	return e.do(http.MethodPost, "/api/events", strings.NewReader(body), jsonHeader)
}

func TestEventsRequiresRecStore(t *testing.T) {
	e := newEnv(t)
	rr := postEvents(t, e, `{"events":[{"kind":"click","bookId":1}]}`)
	requireError(t, rr, http.StatusServiceUnavailable, codeUnavailable)
}

func TestEventsAcceptsBatch(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("埋点书", "埋点作者")
	body := fmt.Sprintf(`{"events":[`+
		`{"kind":"click","bookId":%d,"categoryL1":"武侠"},`+
		`{"kind":"impression","bookId":%d}]}`, bookID, bookID)
	rr := postEvents(t, e, body)
	requireStatus(t, rr, http.StatusOK)
	if got := decodeBody[map[string]int](t, rr)["accepted"]; got != 2 {
		t.Fatalf("accepted = %d，期望 2", got)
	}
	// 兴趣落库后推荐端点的理由应能解释「为什么推这本书」。
	items := recommendItems(t, e, 5)
	if len(items) == 0 || items[0].Reason == "" {
		t.Fatalf("推荐结果为空或缺理由: %+v", items)
	}
}

func TestEventsValidation(t *testing.T) {
	e := newEnvWithRec(t)
	tooMany := strings.Repeat(`{"kind":"click","bookId":1},`, 101)
	cases := map[string]string{
		"空批次":      `{"events":[]}`,
		"缺 events": `{}`,
		"超批量":      `{"events":[` + strings.TrimSuffix(tooMany, ",") + `]}`,
		"未知 kind":  `{"events":[{"kind":"bogus","bookId":1}]}`,
		"缺 bookId": `{"events":[{"kind":"click"}]}`,
		"分类超长":     `{"events":[{"kind":"click","bookId":1,"categoryL1":"` + strings.Repeat("武", maxEventFieldLen+1) + `"}]}`,
		"extra 超长": `{"events":[{"kind":"click","bookId":1,"extra":"` + strings.Repeat("x", maxEventExtraLen+1) + `"}]}`,
		"非法 JSON":  `{"events":`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			requireError(t, postEvents(t, e, body), http.StatusBadRequest, codeInvalidParam)
		})
	}
}
