package api

// 补 internal/api 里「store 出错时怎么响应」这一整类分支。
//
// 做法统一：先用真实 store 把数据准备好，再把 store 关掉让后续调用返回错误，
// 断言端点给出 500（或既定的降级行为），而不是 404/200 或 panic。
// 这类分支正是「数据库抖动时前端看到什么」的真实口径。

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"norvels/internal/importer"
	"norvels/internal/store"
)

// closedEnv 返回一个 store 已关闭的测试环境（请求链路本身保持完整）。
func closedEnv(t *testing.T) *testEnv {
	t.Helper()
	e := newEnv(t)
	requireNoError(t, e.st.Close(), "关闭 store")
	return e
}

// requireNoError 断言 err 为 nil。
func requireNoError(t *testing.T, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// requireInternal 断言状态码 500 + code=internal。
func requireInternal(t *testing.T, rr *httptest.ResponseRecorder, what string) {
	t.Helper()
	requireError(t, rr, http.StatusInternalServerError, codeInternal)
}

// TestHealthStoreDown 数据库不可用时健康检查必须报 503 + status=error（不能被判成 ok）。
func TestHealthStoreDown(t *testing.T) {
	e := closedEnv(t)
	rr := e.get("/api/health")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /api/health = %d，期望 503，body=%s", rr.Code, rr.Body.String())
	}
	body := decodeBody[healthResponse](t, rr)
	if body.Status != "error" || body.DB != "error" {
		t.Errorf("健康检查 = %+v，期望 status/db 都是 error", body)
	}
}

// TestReads500WhenStoreDown 只读端点：store 挂了就是 500 internal（不是 404、不是空列表）。
func TestReads500WhenStoreDown(t *testing.T) {
	e := closedEnv(t)
	targets := []string{"/api/stats", "/api/shelf", "/api/shelf/ids",
		"/api/auth/me", "/api/auth/users", "/api/settings/chapter-rules",
		"/api/import/jobs/1/files"}
	for _, target := range targets {
		rr := e.get(target)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("GET %s 状态码 = %d，期望 500，body=%s", target, rr.Code, rr.Body.String())
		}
	}
}

// TestShelfWritesWhenStoreDown 书架写入：store 不可用统一 500。
func TestShelfWritesWhenStoreDown(t *testing.T) {
	e := closedEnv(t)
	for _, c := range []struct{ method, target string }{
		{http.MethodPut, "/api/shelf/1"},
		{http.MethodDelete, "/api/shelf/1"},
	} {
		rr := e.do(c.method, c.target, nil, nil)
		requireInternal(t, rr, c.method+" "+c.target)
	}
}

// TestShelfRejectsBadID 书架路径参数必须正整数：0 / 负数 / 非数字都是 400 invalid_id。
func TestShelfRejectsBadID(t *testing.T) {
	e := closedEnv(t)
	for _, id := range []string{"0", "-3", "abc"} {
		rr := e.do(http.MethodPut, "/api/shelf/"+id, nil, nil)
		requireError(t, rr, http.StatusBadRequest, codeInvalidID)
	}
}

// TestShelfPaginationBadParam 分页参数非法 → 400 invalid_param（在访问 store 之前就挡掉）。
func TestShelfPaginationBadParam(t *testing.T) {
	e := closedEnv(t)
	for _, q := range []string{"?page=0", "?size=0", "?page=x", "?size=999"} {
		rr := e.get("/api/shelf" + q)
		requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	}
}

// TestSettingsWriteWhenStoreDown 写设置类端点：store 挂了是 500。
func TestSettingsWriteWhenStoreDown(t *testing.T) {
	e := closedEnv(t)
	rr := e.do(http.MethodPut, "/api/settings/chapter-rules", strings.NewReader(`{"rules":[]}`),
		map[string]string{"Content-Type": "application/json"})
	requireInternal(t, rr, "PUT /api/settings/chapter-rules")
}

// TestPutChapterRulesValidation PUT 章节规则的三类非法载荷：条数超限、空正则、正则编译失败。
// 用原始字符串走 do（putJSON 会再序列化一层，测不出「载荷本身长这样」的语义）。
func TestPutChapterRulesValidation(t *testing.T) {
	e := newEnv(t)
	tooMany := make([]string, 0, maxChapterRules+1)
	for i := 0; i <= maxChapterRules; i++ {
		tooMany = append(tooMany, `{"name":"r","pattern":"^第.+章"}`)
	}
	cases := []struct{ name, body, wantCode string }{
		{"条数超限", `{"rules":[` + strings.Join(tooMany, ",") + `]}`, codeInvalidParam},
		{"空正则", `{"rules":[{"name":"r","pattern":"   "}]}`, codeInvalidParam},
		{"正则非法", `{"rules":[{"name":"r","pattern":"("}]}`, codeInvalidPattern},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := e.do(http.MethodPut, "/api/settings/chapter-rules", strings.NewReader(c.body),
				map[string]string{"Content-Type": "application/json"})
			requireError(t, rr, http.StatusBadRequest, c.wantCode)
		})
	}
}

// TestPutChapterRulesNamesNormalized 规则名留空补「规则N」，超长截断到上限。
func TestPutChapterRulesNamesNormalized(t *testing.T) {
	e := newEnv(t)
	longName := strings.Repeat("长", maxChapterRuleNameRunes+5)
	body := `{"rules":[{"name":"  ","pattern":"^第.+章"},{"name":"` + longName + `","pattern":"^卷.+$"}]}`
	rrPut := e.do(http.MethodPut, "/api/settings/chapter-rules", strings.NewReader(body),
		map[string]string{"Content-Type": "application/json"})
	if rrPut.Code != http.StatusNoContent {
		t.Fatalf("PUT 状态码 = %d，body=%s", rrPut.Code, rrPut.Body.String())
	}
	got := decodeBody[chapterRulesBody](t, e.get("/api/settings/chapter-rules"))
	if len(got.Rules) != 2 {
		t.Fatalf("规则条数 = %d，期望 2", len(got.Rules))
	}
	if got.Rules[0].Name != "规则1" {
		t.Errorf("空名补全 = %q，期望 规则1", got.Rules[0].Name)
	}
	if n := len([]rune(got.Rules[1].Name)); n != maxChapterRuleNameRunes {
		t.Errorf("超长名截断后长度 = %d，期望 %d", n, maxChapterRuleNameRunes)
	}
}

// TestLoadChapterRulesSkipsBroken 库里存了一条编译不过的规则：跳过它、保留其余，不整批失败。
func TestLoadChapterRulesSkipsBroken(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	requireNoError(t, e.st.SaveChapterRules(ctx, []store.ChapterRule{
		{Name: "坏的", Pattern: "("},
		{Name: "好的", Pattern: "^第.+章"},
	}), "写入章节规则")

	rules := e.srv.loadChapterRules(ctx)
	if len(rules) != 1 || rules[0].Name != "好的" {
		t.Fatalf("编译后规则 = %+v，期望只剩「好的」", rules)
	}
	// 全坏时返回 nil（= 不启用自定义规则），不是空切片。
	requireNoError(t, e.st.SaveChapterRules(ctx, []store.ChapterRule{{Name: "坏", Pattern: "("}}), "写入坏规则")
	if got := e.srv.loadChapterRules(ctx); got != nil {
		t.Errorf("规则全坏时 = %+v，期望 nil", got)
	}
}

// TestLoadersDegradeWhenStoreDown 读设置失败只降级（nil / 缺省阈值），不 panic、不中断导入。
func TestLoadersDegradeWhenStoreDown(t *testing.T) {
	e := closedEnv(t)
	ctx := context.Background()
	if rules := e.srv.loadChapterRules(ctx); rules != nil {
		t.Errorf("读失败时章节规则 = %+v，期望 nil", rules)
	}
	if v := e.srv.loadDedupeThreshold(ctx); v != store.DefaultDedupeThreshold {
		t.Errorf("读失败时阈值 = %v，期望缺省 %v", v, store.DefaultDedupeThreshold)
	}
	if rules := e.srv.loadCleanRuleList(ctx); rules != nil {
		t.Errorf("读失败时净化规则 = %+v，期望 nil", rules)
	}
}

// TestUploadRootFallback UploadDir 为空时回落系统临时目录，配置了就用配置值。
func TestUploadRootFallback(t *testing.T) {
	if got := (Config{}).uploadRoot(); got == "" {
		t.Error("未配置 UploadDir 时应回落到临时目录，实际空串")
	}
	if got := (Config{UploadDir: "  /tmp/x  "}).uploadRoot(); got != "/tmp/x" {
		t.Errorf("uploadRoot = %q，期望去掉首尾空白的 /tmp/x", got)
	}
}

// TestUploadFailureMapping 上传失败映射成任务终态与提示。
func TestUploadFailureMapping(t *testing.T) {
	if status, msg := uploadFailure(importer.ErrNoFiles); status != "failed" || !strings.Contains(msg, "可导入") {
		t.Errorf("无文件 → %q/%q", status, msg)
	}
	if status, _ := uploadFailure(context.Canceled); status != "canceled" {
		t.Errorf("取消 → %q，期望 canceled", status)
	}
	if status, msg := uploadFailure(errors.New("boom")); status != "failed" || !strings.Contains(msg, "boom") {
		t.Errorf("其它错误 → %q/%q", status, msg)
	}
}

// TestUploadSessionFinish 批次校验：空批次 400、全不支持 400 unsupported_format。
func TestUploadSessionFinish(t *testing.T) {
	empty := &uploadSession{}
	rr := httptest.NewRecorder()
	if empty.finish(rr) {
		t.Fatal("空批次应被拒")
	}
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)

	rejectedOnly := &uploadSession{rejected: []uploadRejected{{Name: "a.exe", Code: codeUnsupportedFormat, Message: "不支持"}}}
	rr = httptest.NewRecorder()
	if rejectedOnly.finish(rr) {
		t.Fatal("全不支持的批次应被拒")
	}
	requireError(t, rr, http.StatusBadRequest, codeUnsupportedFormat)
}

// TestDiscardUploadEmptyDir 空目录名是空操作（绝不能把 uploadRoot 整个删掉）。
func TestDiscardUploadEmptyDir(t *testing.T) {
	e := newEnv(t)
	e.srv.discardUpload("")
}
