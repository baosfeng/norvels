package booksource

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"norvels/internal/source"
)

// sampleJSON 是虚构的 Legado 书源（站点 example.invalid，不含真实书名/站点）。
const sampleJSON = `{
  "bookSourceUrl": "https://example.invalid/books",
  "bookSourceName": "示例书源",
  "bookSourceGroup": "示例组",
  "bookSourceType": 0,
  "enabled": false,
  "customOrder": 42,
  "searchUrl": "https://example.invalid/search?q={{key}}&page={{page}}",
  "exploreUrl": "https://example.invalid/explore",
  "bookUrlPattern": "/book/\\d+",
  "ruleSearch": {"bookList": ".item", "name": ".title", "author": ".author"},
  "ruleExplore": {"bookList": ".explore-item"},
  "ruleBookInfo": {"name": ".book-name"},
  "ruleToc": {"chapterList": ".chapter"},
  "ruleContent": {"content": ".content"},
  "header": {"User-Agent": "示例"},
  "loginUrl": "https://example.invalid/login",
  "cookie": "session=示例",
  "webJs": "document.cookie"
}`

// TestParseLegadoSingleObject 单对象形态：字段映射、id 派生、不支持字段收集。
func TestParseLegadoSingleObject(t *testing.T) {
	profiles, issues, err := ParseLegado([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("ParseLegado: %v", err)
	}
	if len(profiles) != 1 || len(issues) != 1 {
		t.Fatalf("profiles/issues = %d/%d，期望 1/1", len(profiles), len(issues))
	}
	requireLegadoIssue(t, issues[0])
	requireLegadoOnline(t, profiles[0])
}

// requireLegadoIssue 断言单条结果的 id / 状态 / 不支持字段。
func requireLegadoIssue(t *testing.T, issue ImportIssue) {
	t.Helper()
	if issue.ID != SourceID("https://example.invalid/books") {
		t.Errorf("id = %q，期望由 bookSourceUrl 派生", issue.ID)
	}
	if issue.Status != StatusOK || issue.Reason != "" {
		t.Errorf("status/reason = %q/%q，期望 ok/空", issue.Status, issue.Reason)
	}
	if want := []string{"header", "loginUrl", "cookie", "webJs"}; !equalStrings(issue.UnsupportedFields, want) {
		t.Errorf("unsupportedFields = %v，期望 %v", issue.UnsupportedFields, want)
	}
}

// requireLegadoOnline 断言 Profile 的身份字段与 online 段映射。
func requireLegadoOnline(t *testing.T, p source.Profile) {
	t.Helper()
	if p.Kind != source.KindOnline || p.Priority != 42 || p.Enabled {
		t.Errorf("kind/priority/enabled = %q/%d/%v", p.Kind, p.Priority, p.Enabled)
	}
	if p.Online == nil || p.Online.BookSourceURL != "https://example.invalid/books" {
		t.Fatalf("online 段缺失: %+v", p.Online)
	}
	if p.Online.SearchURL != "https://example.invalid/search?q={{key}}&page={{page}}" {
		t.Errorf("searchUrl = %q", p.Online.SearchURL)
	}
	if p.Online.RuleExplore["bookList"] != ".explore-item" {
		t.Errorf("ruleExplore = %v", p.Online.RuleExplore)
	}
	if p.Online.BookURLPattern != "/book/\\d+" {
		t.Errorf("bookUrlPattern = %q", p.Online.BookURLPattern)
	}
	if len(p.Online.UnsupportedFields) != 4 {
		t.Errorf("online.unsupportedFields = %v，期望 4 项", p.Online.UnsupportedFields)
	}
}

// TestParseLegadoEnvelopes 数组 / bookSources / sources 三种打包形态等价。
func TestParseLegadoEnvelopes(t *testing.T) {
	cases := map[string]string{
		"数组":          "[" + sampleJSON + "]",
		"bookSources": `{"bookSources":[` + sampleJSON + `]}`,
		"sources":     `{"sources":[` + sampleJSON + `]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			profiles, issues, err := ParseLegado([]byte(raw))
			if err != nil {
				t.Fatalf("ParseLegado: %v", err)
			}
			if len(profiles) != 1 || len(issues) != 1 {
				t.Fatalf("profiles/issues = %d/%d，期望 1/1", len(profiles), len(issues))
			}
			if profiles[0].ID != SourceID("https://example.invalid/books") {
				t.Errorf("id = %q", profiles[0].ID)
			}
		})
	}
}

// TestParseLegadoRejectsNonText 音频/图片/文件源明确拒绝并说明原因。
func TestParseLegadoRejectsNonText(t *testing.T) {
	for _, typ := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("type=%d", typ), func(t *testing.T) {
			raw := fmt.Sprintf(`{"bookSourceUrl":"https://example.invalid/%d","bookSourceType":%d}`, typ, typ)
			profiles, issues, err := ParseLegado([]byte(raw))
			if err != nil {
				t.Fatalf("ParseLegado: %v", err)
			}
			if len(profiles) != 0 {
				t.Fatalf("非文本书源不该入库: %+v", profiles)
			}
			if issues[0].Status != StatusUnsupportedType || !strings.Contains(issues[0].Reason, "只支持文本书源") {
				t.Errorf("status/reason = %q/%q", issues[0].Status, issues[0].Reason)
			}
		})
	}
}

// TestParseLegadoRejectsInvalidURL 缺 URL / 非 http(s) 都拒绝。
func TestParseLegadoRejectsInvalidURL(t *testing.T) {
	cases := map[string]string{
		"缺 url":  `{"bookSourceName":"示例"}`,
		"非 http": `{"bookSourceUrl":"ftp://example.invalid/x"}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			profiles, issues, err := ParseLegado([]byte(raw))
			if err != nil {
				t.Fatalf("ParseLegado: %v", err)
			}
			if len(profiles) != 0 || issues[0].Status != StatusInvalid {
				t.Fatalf("期望 invalid 且不入库: %+v", issues)
			}
			if issues[0].Type != TypeBookSource {
				t.Errorf("type = %q，期望 book_source", issues[0].Type)
			}
			if issues[0].Reason == "" {
				t.Error("invalid 必须带原因")
			}
		})
	}
}

// TestParseLegadoJSHard hard 字段的 @js → unusable，逐字段回显。
func TestParseLegadoJSHard(t *testing.T) {
	raw := `{"bookSourceUrl":"https://example.invalid/hard","ruleSearch":{"bookList":"@js:java.ajax(url)"}}`
	profiles, issues, err := ParseLegado([]byte(raw))
	if err != nil {
		t.Fatalf("ParseLegado: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("unusable 不该产出可落库的规则包: %+v", profiles)
	}
	if issues[0].Status != StatusUnusable {
		t.Errorf("status = %q，期望 unusable", issues[0].Status)
	}
	if !contains(issues[0].DegradedFields, "ruleSearch.bookList") {
		t.Errorf("degradedFields = %v，缺 ruleSearch.bookList", issues[0].DegradedFields)
	}
}

// TestParseLegadoJSSoft soft 字段的 @js → partial。
func TestParseLegadoJSSoft(t *testing.T) {
	raw := `{"bookSourceUrl":"https://example.invalid/soft","ruleBookInfo":{"author":"@js:x"}}`
	profiles, issues, err := ParseLegado([]byte(raw))
	if err != nil || len(profiles) != 1 {
		t.Fatalf("profiles=%d err=%v", len(profiles), err)
	}
	if issues[0].Status != StatusPartial {
		t.Errorf("status = %q，期望 partial", issues[0].Status)
	}
}

// TestParseLegadoSearchURLJS searchUrl 是 URL 模板，含 @js 按 hard 字段处理。
func TestParseLegadoSearchURLJS(t *testing.T) {
	raw := `{"bookSourceUrl":"https://example.invalid/s","searchUrl":"@js:url"}`
	_, issues, err := ParseLegado([]byte(raw))
	if err != nil {
		t.Fatalf("ParseLegado: %v", err)
	}
	if issues[0].Status != StatusUnusable || !contains(issues[0].DegradedFields, "searchUrl") {
		t.Errorf("status/degraded = %q/%v", issues[0].Status, issues[0].DegradedFields)
	}
}

// TestParseEntryTypes 通用导入：按字段识别书源 / 净化 / TXT / 未知，只落书源。
func TestParseEntryTypes(t *testing.T) {
	raw := `[
	  {"bookSourceUrl":"https://example.invalid/a","bookSourceName":"示例书源"},
	  {"name":"示例净化","pattern":"广告.*","replacement":""},
	  {"name":"示例TXT规则","rule":"^第\\d+章.*$"},
	  {"foo":"bar"}
	]`
	profiles, issues, err := ParseLegado([]byte(raw))
	if err != nil {
		t.Fatalf("ParseLegado: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("只有书源该落库，实际 %d 份: %+v", len(profiles), profiles)
	}
	wantTypes := []string{TypeBookSource, TypeReplaceRule, TypeTXTTocRule, TypeUnknown}
	wantStatus := []string{StatusOK, StatusRecognized, StatusRecognized, StatusUnknownType}
	if len(issues) != len(wantTypes) {
		t.Fatalf("结果数 = %d，期望 %d", len(issues), len(wantTypes))
	}
	for i := range wantTypes {
		if issues[i].Type != wantTypes[i] {
			t.Errorf("第 %d 条 type = %q，期望 %q", i+1, issues[i].Type, wantTypes[i])
		}
		if issues[i].Status != wantStatus[i] {
			t.Errorf("第 %d 条 status = %q，期望 %q", i+1, issues[i].Status, wantStatus[i])
		}
	}
	if issues[3].Reason == "" {
		t.Error("unknown_type 必须带原因")
	}
}

// TestSourceIDDerivation id 幂等：同 url 同 id，不同 url 不同 id。
func TestSourceIDDerivation(t *testing.T) {
	a := SourceID("https://example.invalid/a")
	if a != SourceID("https://example.invalid/a") {
		t.Error("同一 url 的 id 必须稳定")
	}
	if a == SourceID("https://example.invalid/b") {
		t.Error("不同 url 的 id 不该相同")
	}
	if !regexp.MustCompile(`^src-[0-9a-f]{8}$`).MatchString(a) {
		t.Errorf("id 形状 = %q，期望 src-<8 位十六进制>", a)
	}
}

// TestParseLegadoBadRaw 坏 JSON / 空输入 / 空数组都返回错误。
func TestParseLegadoBadRaw(t *testing.T) {
	for name, raw := range map[string]string{
		"坏 JSON": "{not json",
		"空白":     "   ",
		"空数组":    "[]",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ParseLegado([]byte(raw)); err == nil {
				t.Fatal("应返回错误")
			}
		})
	}
}

// TestImportIssueJSON 结果的 JSON 字段名与契约一致（前端按这些字段读）。
func TestImportIssueJSON(t *testing.T) {
	raw, err := json.Marshal(ImportIssue{
		Index: 0, ID: "src-00000000", Name: "示例", Status: StatusPartial,
		DegradedFields: []string{"ruleToc.chapterList"}, UnsupportedFields: []string{"header"}, Reason: "示例",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{"index", "id", "name", "status", "degradedFields", "unsupportedFields", "reason"} {
		if !strings.Contains(string(raw), "\""+key+"\"") {
			t.Errorf("JSON 缺字段 %q: %s", key, raw)
		}
	}
}

// equalStrings 比较两个字符串切片。
func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// contains 判断切片是否包含目标字符串。
func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
