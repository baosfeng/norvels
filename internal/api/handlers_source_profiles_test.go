package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"norvels/internal/source"
)

// sampleProfile 返回一份合法规则包；示例全虚构（/books/示例/**），不引用真实书名或目录。
func sampleProfile(id string) source.Profile {
	return source.Profile{
		ID: id, Name: "示例目录", SchemaVersion: 1, Enabled: true, Priority: 10,
		Kind:  source.KindLocal,
		Match: source.Match{Ext: []string{"txt"}, PathGlob: "/books/示例/**"},
		Filters: source.Filters{
			Extra: []source.FilterRule{{Name: "广告行", Priority: 30, Pattern: "^广告：.*$"}},
		},
		Split:  source.Split{Rules: []source.Rule{{Name: "分节符", Pattern: `^=== 第\d+ 节 ===$`}}},
		Tags:   []source.TagRule{{Tag: "奇幻", Keywords: []string{"魔法"}}},
		Limits: source.Limits{MaxChapterChars: 20000},
	}
}

// TestSourceProfilesDefaultEmpty 未配置时返回 {"profiles":[]}（不是 null）。
func TestSourceProfilesDefaultEmpty(t *testing.T) {
	e := newEnv(t)
	rr := e.get(sourceProfilesPath)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET 状态码 = %d", rr.Code)
	}
	var raw struct {
		Profiles json.RawMessage `json:"profiles"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if string(raw.Profiles) != "[]" {
		t.Errorf("profiles = %s，期望 []", raw.Profiles)
	}
}

// TestSourceProfilesPutAndGet 全量替换后原样读回；传空数组即清空。
func TestSourceProfilesPutAndGet(t *testing.T) {
	e := newEnv(t)
	online := sampleProfile("sample-online")
	online.Kind = source.KindOnline
	online.Online = &source.Online{
		BookSourceURL: "https://example.invalid/book",
		Group:         "示例组",
		RuleSearch:    map[string]string{"name": ".title"},
	}
	want := []source.Profile{sampleProfile("sample-dir"), online}

	if rr := e.putJSON(sourceProfilesPath, map[string]any{"profiles": want}); rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 状态码 = %d，期望 204，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath))
	if len(got.Profiles) != 2 {
		t.Fatalf("规则包数 = %d，期望 2: %+v", len(got.Profiles), got.Profiles)
	}
	if got.Profiles[0].ID != "sample-dir" || got.Profiles[1].ID != "sample-online" {
		t.Errorf("顺序或 id 不对: %+v", got.Profiles)
	}
	if got.Profiles[1].Online == nil || got.Profiles[1].Online.BookSourceURL != "https://example.invalid/book" {
		t.Errorf("online 段丢失: %+v", got.Profiles[1].Online)
	}

	if rr := e.putJSON(sourceProfilesPath, map[string]any{"profiles": []source.Profile{}}); rr.Code != http.StatusNoContent {
		t.Fatalf("清空 PUT 状态码 = %d，期望 204", rr.Code)
	}
	if got := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)); len(got.Profiles) != 0 {
		t.Errorf("清空后仍有规则包: %+v", got.Profiles)
	}
}

// TestSourceProfilesInvalidJSON 坏 JSON → 400 invalid_param，且不落库。
func TestSourceProfilesInvalidJSON(t *testing.T) {
	e := newEnv(t)
	rr := e.do(http.MethodPut, sourceProfilesPath, strings.NewReader("{not json"),
		map[string]string{"Content-Type": "application/json"})
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	if got := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)); len(got.Profiles) != 0 {
		t.Errorf("坏 JSON 不该落库: %+v", got.Profiles)
	}
}

// TestSourceProfilesInvalidPattern 坏正则 → 400 invalid_pattern，消息带第几份/第几条。
func TestSourceProfilesInvalidPattern(t *testing.T) {
	e := newEnv(t)
	bad := sampleProfile("sample-dir")
	bad.Split.Rules = []source.Rule{{Name: "坏正则", Pattern: "^第(\\d+章"}}
	rr := e.putJSON(sourceProfilesPath, map[string]any{"profiles": []source.Profile{bad}})
	requireError(t, rr, http.StatusBadRequest, codeInvalidPattern)
	msg := decodeBody[errResp](t, rr).Error.Message
	if !strings.Contains(msg, "第 1 份规则包") || !strings.Contains(msg, "第 1 条切章规则") {
		t.Errorf("消息 %q 未定位到具体规则", msg)
	}
	if got := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)); len(got.Profiles) != 0 {
		t.Errorf("非法规则包不该落库: %+v", got.Profiles)
	}
}

// TestSourceProfilesTooMany 超过 20 份 → 400 invalid_param。
func TestSourceProfilesTooMany(t *testing.T) {
	e := newEnv(t)
	many := make([]source.Profile, 0, source.MaxProfiles+1)
	for i := 0; i <= source.MaxProfiles; i++ {
		many = append(many, sampleProfile(fmt.Sprintf("p-%d", i)))
	}
	requireError(t, e.putJSON(sourceProfilesPath, map[string]any{"profiles": many}),
		http.StatusBadRequest, codeInvalidParam)
}

// TestSourceProfilesDuplicateID 重复 id → 400，消息指向后出现的那一份。
func TestSourceProfilesDuplicateID(t *testing.T) {
	e := newEnv(t)
	rr := e.putJSON(sourceProfilesPath, map[string]any{
		"profiles": []source.Profile{sampleProfile("dup"), sampleProfile("dup")},
	})
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	if msg := decodeBody[errResp](t, rr).Error.Message; !strings.Contains(msg, "第 2 份规则包") {
		t.Errorf("消息 %q 未指向第 2 份", msg)
	}
}

// TestSourceProfilesValidateOnly validate 端点逐条返回结果，且绝不落库。
func TestSourceProfilesValidateOnly(t *testing.T) {
	e := newEnv(t)
	good := sampleProfile("good")
	bad := sampleProfile("bad")
	bad.Match.NameRegex = "^("
	payload, err := json.Marshal(map[string]any{"profiles": []source.Profile{good, bad}})
	if err != nil {
		t.Fatalf("序列化请求体: %v", err)
	}
	rr := e.postJSON(sourceProfilesPath+"/validate", string(payload))
	if rr.Code != http.StatusOK {
		t.Fatalf("validate 状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[sourceValidateResponse](t, rr)
	if len(got.Results) != 2 {
		t.Fatalf("逐条结果数 = %d，期望 2: %+v", len(got.Results), got.Results)
	}
	if !got.Results[0].OK || got.Results[1].OK || got.Results[1].Reason == "" {
		t.Errorf("逐条结果不对: %+v", got.Results)
	}
	if got.Results[1].Index != 1 || got.Results[1].ID != "bad" {
		t.Errorf("位置/身份不对: %+v", got.Results[1])
	}
	if stored := decodeBody[sourceProfilesBody](t, e.get(sourceProfilesPath)); len(stored.Profiles) != 0 {
		t.Errorf("validate 不该落库: %+v", stored.Profiles)
	}
}
