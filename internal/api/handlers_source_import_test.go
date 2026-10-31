package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"norvels/internal/booksource"
	"norvels/internal/fetch"
	"norvels/internal/ruleimport"
	"norvels/internal/source"
)

// legadoSourceJSON 造一条虚构 Legado 书源（站点 example.invalid）。
func legadoSourceJSON(url, name string) map[string]any {
	return map[string]any{
		"bookSourceUrl":  url,
		"bookSourceName": name,
		"bookSourceType": 0,
		"searchUrl":      "https://example.invalid/search?q={{key}}",
		"ruleSearch":     map[string]string{"bookList": ".item", "name": ".title"},
		"header":         map[string]string{"User-Agent": "示例"},
	}
}

// rawLegado 把书源列表序列化成导入用的 raw 字符串。
func rawLegado(t *testing.T, sources ...map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(sources)
	if err != nil {
		t.Fatalf("序列化书源: %v", err)
	}
	return string(raw)
}

// postLegadoImport 发一次 Legado 导入请求。
func (e *testEnv) postLegadoImport(t *testing.T, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("序列化请求体: %v", err)
	}
	return e.postJSON(sourceProfilesPath+"/import", string(payload))
}

// TestLegadoImportPreviewOnly 预览不落库：逐条返回状态（含明确拒绝的非文本书源）。
func TestLegadoImportPreviewOnly(t *testing.T) {
	e := newEnv(t)
	audio := legadoSourceJSON("https://example.invalid/audio", "音频源")
	audio["bookSourceType"] = 1
	raw := rawLegado(t, legadoSourceJSON("https://example.invalid/a", "示例源"), audio)

	rr := e.postLegadoImport(t, map[string]any{"raw": raw})
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[sourceImportResponse](t, rr)
	if len(got.Results) != 2 {
		t.Fatalf("结果数 = %d，期望 2", len(got.Results))
	}
	if got.Results[0].Status != booksource.StatusOK {
		t.Errorf("第 1 条 status = %q，期望 ok", got.Results[0].Status)
	}
	if got.Results[1].Status != booksource.StatusUnsupportedType {
		t.Errorf("第 2 条 status = %q，期望 unsupported_type", got.Results[1].Status)
	}
	if !strings.Contains(got.Results[1].Reason, "只支持文本书源") {
		t.Errorf("第 2 条 reason = %q", got.Results[1].Reason)
	}
	if got.Results[1].DegradedFields == nil || got.Results[1].UnsupportedFields == nil {
		t.Error("被拒绝条目的 degradedFields/unsupportedFields 应为空数组而不是 null")
	}
	if stored := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)); len(stored.Profiles) != 0 {
		t.Errorf("预览不该落库: %+v", stored.Profiles)
	}
}

// TestLegadoImportApplyMergesLocal 应用后与已有 local 规则包合并，local 不丢。
func TestLegadoImportApplyMergesLocal(t *testing.T) {
	e := newEnv(t)
	putLocalProfile(t, e, "local-keep")
	raw := rawLegado(t, legadoSourceJSON("https://example.invalid/a", "示例源"))
	rr := e.postLegadoImport(t, map[string]any{"raw": raw, "apply": true})
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[sourceImportResponse](t, rr)
	if got.Applied != 1 || got.Overwritten != 0 {
		t.Errorf("applied/overwritten = %d/%d，期望 1/0", got.Applied, got.Overwritten)
	}
	requireMergedStored(t, e)
}

// putLocalProfile 通过 PUT 落一份 local 规则包（模拟用户手写的规则）。
func putLocalProfile(t *testing.T, e *testEnv, id string) {
	t.Helper()
	rr := e.putJSON(sourceProfilesPath, map[string]any{"profiles": []source.Profile{sampleProfile(id)}})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 本地规则包: %d", rr.Code)
	}
}

// requireMergedStored 断言落库结果是「local 保留 + 导入源追加」。
func requireMergedStored(t *testing.T, e *testEnv) {
	t.Helper()
	stored := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)).Profiles
	if len(stored) != 2 {
		t.Fatalf("规则包数 = %d，期望 2（local + 导入）: %+v", len(stored), stored)
	}
	if stored[0].ID != "local-keep" {
		t.Errorf("local 规则包丢失或顺序变化: %+v", stored)
	}
	if stored[1].ID != booksource.SourceID("https://example.invalid/a") || stored[1].Online == nil {
		t.Errorf("导入结果不对: %+v", stored[1])
	}
	if len(stored[1].Online.UnsupportedFields) == 0 || stored[1].Online.UnsupportedFields[0] != "header" {
		t.Errorf("unsupportedFields 未写入: %+v", stored[1].Online.UnsupportedFields)
	}
}

// TestLegadoImportOverwriteIdempotent 同一 url 重复导入：同 id 覆盖，不新增。
func TestLegadoImportOverwriteIdempotent(t *testing.T) {
	e := newEnv(t)
	raw := rawLegado(t, legadoSourceJSON("https://example.invalid/a", "示例源"))
	for i := 0; i < 2; i++ {
		rr := e.postLegadoImport(t, map[string]any{"raw": raw, "apply": true})
		if rr.Code != http.StatusOK {
			t.Fatalf("第 %d 次 apply: %d %s", i+1, rr.Code, rr.Body.String())
		}
		got := decodeBody[sourceImportResponse](t, rr)
		if got.Applied != 1 {
			t.Errorf("第 %d 次 applied = %d，期望 1", i+1, got.Applied)
		}
		if i == 1 && got.Overwritten != 1 {
			t.Errorf("第 2 次 overwritten = %d，期望 1", got.Overwritten)
		}
	}
	if stored := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)).Profiles; len(stored) != 1 {
		t.Errorf("重复导入不该新增: %+v", stored)
	}
}

// TestLegadoImportSkipsAudio 分享站整包里的音频源被跳过，不拖垮整批。
func TestLegadoImportSkipsAudio(t *testing.T) {
	e := newEnv(t)
	audio := legadoSourceJSON("https://example.invalid/audio", "示例音频源")
	audio["bookSourceType"] = 1
	raw := rawLegado(t, legadoSourceJSON("https://example.invalid/a", "示例源"), audio)
	rr := e.postLegadoImport(t, map[string]any{"raw": raw, "apply": true})
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[sourceImportResponse](t, rr)
	if got.Applied != 1 {
		t.Errorf("applied = %d，期望 1", got.Applied)
	}
	if len(got.Skipped) != 1 || got.Skipped[0].Status != booksource.StatusUnsupportedType {
		t.Fatalf("skipped = %+v，期望 1 条 unsupported_type", got.Skipped)
	}
	if !strings.Contains(got.Skipped[0].Reason, "只支持文本书源") {
		t.Errorf("skipped 原因 = %q", got.Skipped[0].Reason)
	}
	if stored := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)).Profiles; len(stored) != 1 {
		t.Errorf("库里应只有 1 份: %+v", stored)
	}
}

// TestLegadoImportSkipsInvalid 混合集合里的 invalid 条目被跳过，其余照常入库。
func TestLegadoImportSkipsInvalid(t *testing.T) {
	e := newEnv(t)
	raw := rawLegado(t,
		legadoSourceJSON("https://example.invalid/a", "示例源"),
		map[string]any{"bookSourceName": "缺 url"},
	)
	rr := e.postLegadoImport(t, map[string]any{"raw": raw, "apply": true})
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[sourceImportResponse](t, rr)
	if got.Applied != 1 || len(got.Skipped) != 1 || got.Skipped[0].Status != booksource.StatusInvalid {
		t.Fatalf("applied/skipped = %d/%+v，期望 1/1(invalid)", got.Applied, got.Skipped)
	}
	if stored := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)).Profiles; len(stored) != 1 {
		t.Errorf("库里应只有 1 份: %+v", stored)
	}
}

// TestLegadoImportNoUsable 一条可用的都没有 → 400，消息指向 results。
func TestLegadoImportNoUsable(t *testing.T) {
	e := newEnv(t)
	audio := legadoSourceJSON("https://example.invalid/audio", "示例音频源")
	audio["bookSourceType"] = 1
	rr := e.postLegadoImport(t, map[string]any{"raw": rawLegado(t, audio), "apply": true})
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	msg := decodeBody[errResp](t, rr).Error.Message
	if !strings.Contains(msg, "没有可导入的书源") || !strings.Contains(msg, "results") {
		t.Errorf("消息 %q 未说明原因见 results", msg)
	}
	if stored := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)).Profiles; len(stored) != 0 {
		t.Errorf("没有可用书源时不该落库: %+v", stored)
	}
}

// TestLegadoImportTooMany 合并后超过 20 份 → 400。
func TestLegadoImportTooMany(t *testing.T) {
	e := newEnv(t)
	many := make([]source.Profile, 0, source.MaxProfiles)
	for i := 0; i < source.MaxProfiles; i++ {
		many = append(many, sampleProfile(fmt.Sprintf("p-%d", i)))
	}
	if rr := e.putJSON(sourceProfilesPath, map[string]any{"profiles": many}); rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 20 份: %d", rr.Code)
	}
	raw := rawLegado(t, legadoSourceJSON("https://example.invalid/a", "示例源"))
	requireError(t, e.postLegadoImport(t, map[string]any{"raw": raw, "apply": true}),
		http.StatusBadRequest, codeInvalidParam)
}

// TestLegadoImportMixedTypes 混合输入：只落书源，净化 / TXT / 未知只回显不阻断。
func TestLegadoImportMixedTypes(t *testing.T) {
	e := newEnv(t)
	raw := `[{"bookSourceUrl":"https://example.invalid/a","bookSourceName":"示例书源"},
		{"name":"示例净化","pattern":"广告.*","replacement":""},
		{"name":"示例TXT规则","rule":"^第\\d+章.*$"},
		{"foo":"bar"}]`
	rr := e.postLegadoImport(t, map[string]any{"raw": raw, "apply": true})
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[sourceImportResponse](t, rr)
	if got.Applied != 1 || len(got.Results) != 4 {
		t.Fatalf("applied/results = %d/%d，期望 1/4: %+v", got.Applied, len(got.Results), got.Results)
	}
	requireMixedTypeResults(t, got)
	if stored := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)).Profiles; len(stored) != 1 {
		t.Errorf("只该落书源一份，实际 %d: %+v", len(stored), stored)
	}
}

// requireMixedTypeResults 断言四条结果的 type/status 与 skipped 为空。
func requireMixedTypeResults(t *testing.T, got sourceImportResponse) {
	t.Helper()
	wantTypes := []string{booksource.TypeBookSource, booksource.TypeReplaceRule,
		booksource.TypeTXTTocRule, booksource.TypeUnknown}
	for i, want := range wantTypes {
		if got.Results[i].Type != want {
			t.Errorf("第 %d 条 type = %q，期望 %q", i+1, got.Results[i].Type, want)
		}
	}
	if got.Results[1].Status != booksource.StatusRecognized || got.Results[2].Status != booksource.StatusRecognized {
		t.Errorf("净化/TXT 应为 recognized: %+v", got.Results[1:3])
	}
	if got.Results[3].Status != booksource.StatusUnknownType {
		t.Errorf("未知条目 status = %q，期望 unknown_type", got.Results[3].Status)
	}
	if len(got.Skipped) != 0 {
		t.Errorf("该集合没有不可用书源，skipped 应为空: %+v", got.Skipped)
	}
}

// stubImportFetcher 是 ruleimport.Fetcher 的测试替身：不出网，直接返回固定内容。
type stubImportFetcher struct {
	body []byte
	err  error
}

func (s stubImportFetcher) Get(ctx context.Context, rawURL string) (*fetch.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &fetch.Response{Status: 200, Body: s.body, FinalURL: rawURL}, nil
}

// TestLegadoImportViaLink raw 是 legado:// 链接：取回后预览，响应带 via=legado。
func TestLegadoImportViaLink(t *testing.T) {
	body := `[{"bookSourceUrl":"https://example.invalid/a","bookSourceName":"示例源"}]`
	e := newEnvWith(t, Config{
		DisableAccessLog: true,
		RuleImportOptions: ruleimport.Options{
			Fetcher: stubImportFetcher{body: []byte(body)},
		},
	}, testFS())
	rr := e.postLegadoImport(t, map[string]any{
		"raw": "legado://import/bookSource?src=https://example.invalid/sources.json",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[sourceImportResponse](t, rr)
	if got.Via != "legado" {
		t.Errorf("via = %q，期望 legado", got.Via)
	}
	if len(got.Results) != 1 || got.Results[0].Type != booksource.TypeBookSource {
		t.Fatalf("取回结果未识别成书源: %+v", got.Results)
	}
}

// TestLegadoImportFetchErrors 取回错误映射成 400，消息按类型区分。
func TestLegadoImportFetchErrors(t *testing.T) {
	e := newEnv(t)
	rr := e.postLegadoImport(t, map[string]any{"raw": "ftp://example.invalid/x"})
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	if msg := decodeBody[errResp](t, rr).Error.Message; !strings.Contains(msg, "支持 JSON 文本") {
		t.Errorf("消息 %q 未列出支持的形态", msg)
	}

	blocked := newEnvWith(t, Config{
		DisableAccessLog: true,
		RuleImportOptions: ruleimport.Options{
			Fetcher: stubImportFetcher{err: fetch.ErrBlocked},
		},
	}, testFS())
	rr2 := blocked.postLegadoImport(t, map[string]any{"raw": "https://example.invalid/x"})
	requireError(t, rr2, http.StatusBadRequest, codeInvalidParam)
	if msg := decodeBody[errResp](t, rr2).Error.Message; !strings.Contains(msg, "不允许访问") {
		t.Errorf("消息 %q 未说明目标地址不允许访问", msg)
	}
}

// TestLegadoImportBadRaw 空 raw / 坏 JSON → 400。
func TestLegadoImportBadRaw(t *testing.T) {
	e := newEnv(t)
	requireError(t, e.postLegadoImport(t, map[string]any{"raw": "  "}),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, e.postLegadoImport(t, map[string]any{"raw": "{not json"}),
		http.StatusBadRequest, codeInvalidParam)
}
