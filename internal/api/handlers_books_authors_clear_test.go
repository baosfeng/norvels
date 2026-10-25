package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestPatchBookClearAuthors authors 传空数组 = 清空作者（P1-B §2.5 口径覆盖）。
func TestPatchBookClearAuthors(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"空数组", `{"authors":[]}`},
		{"只有空白名", `{"authors":["  ",""]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)
			id := env.seedBook("清作者之书", "原作者")
			rr := env.do(http.MethodPatch, fmt.Sprintf("/api/books/%d", id),
				strings.NewReader(tc.body), nil)
			if rr.Code != http.StatusOK {
				t.Fatalf("PATCH %s 状态码 = %d，期望 200，body=%s", tc.body, rr.Code, rr.Body.String())
			}
			body := decodeBody[bookDetailResp](t, rr)
			if len(body.Authors) != 0 || body.AuthorName != "" {
				t.Fatalf("清空后 authors=%v authorName=%q，期望 [] 与空串", body.Authors, body.AuthorName)
			}
			// 再 GET 一次确认已持久化（不是只在响应里清空）。
			again := decodeBody[bookDetailResp](t, env.get(bookPath(id)))
			if len(again.Authors) != 0 || again.AuthorName != "" {
				t.Errorf("重新读详情 authors=%v authorName=%q，期望 [] 与空串", again.Authors, again.AuthorName)
			}
		})
	}
}
