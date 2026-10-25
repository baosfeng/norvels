package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"norvels/internal/store"
)

// chapterRulesPath 是规则接口路径（拼一次，避免用例里散落字面量）。
const chapterRulesPath = "/api/settings/chapter-rules"

// TestChapterRulesDefaultEmpty 没配置时返回 {"rules":[]}（不是 null）。
func TestChapterRulesDefaultEmpty(t *testing.T) {
	e := newEnv(t)
	rr := e.get(chapterRulesPath)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET 状态码 = %d", rr.Code)
	}
	var raw struct {
		Rules json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if string(raw.Rules) != "[]" {
		t.Errorf("rules = %s，期望 []", raw.Rules)
	}
}

// TestChapterRulesPutAndGet 全量替换后能原样读回。
func TestChapterRulesPutAndGet(t *testing.T) {
	e := newEnv(t)
	rules := []store.ChapterRule{
		{Name: "分节符", Pattern: "^=== 第\\d+ 节 ===$"},
		{Name: "", Pattern: "^第[一二三四五六七八九十]+卷"},
	}
	rr := e.putJSON(chapterRulesPath, map[string]any{"rules": rules})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 状态码 = %d，期望 204，body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Errorf("204 不该有响应体，实际 %q", rr.Body.String())
	}
	got := decodeBody[chapterRulesBody](t, e.get(chapterRulesPath))
	if len(got.Rules) != 2 {
		t.Fatalf("规则数 = %d，期望 2: %+v", len(got.Rules), got.Rules)
	}
	if got.Rules[0] != rules[0] {
		t.Errorf("第 1 条 = %+v，期望 %+v", got.Rules[0], rules[0])
	}
	// 没起名的规则被补成「规则2」，避免前端列表出现空标题。
	if got.Rules[1].Name != "规则2" {
		t.Errorf("空名规则 name = %q，期望「规则2」", got.Rules[1].Name)
	}

	// 全量替换：空列表即清空。
	if rr := e.putJSON(chapterRulesPath, map[string]any{"rules": []store.ChapterRule{}}); rr.Code != http.StatusNoContent {
		t.Fatalf("清空 PUT 状态码 = %d，期望 204", rr.Code)
	}
	if got := decodeBody[chapterRulesBody](t, e.get(chapterRulesPath)); len(got.Rules) != 0 {
		t.Errorf("清空后仍有规则: %+v", got.Rules)
	}
}

// TestChapterRulesInvalidPattern 非法正则 → 400 invalid_pattern，且不落库。
func TestChapterRulesInvalidPattern(t *testing.T) {
	e := newEnv(t)
	bad := map[string]any{"rules": []store.ChapterRule{{Name: "坏正则", Pattern: "^第(\\d+章"}}}
	requireError(t, e.putJSON(chapterRulesPath, bad), http.StatusBadRequest, codeInvalidPattern)
	if got := decodeBody[chapterRulesBody](t, e.get(chapterRulesPath)); len(got.Rules) != 0 {
		t.Errorf("非法规则不该落库: %+v", got.Rules)
	}
}

// TestChapterRulesLimits 条数 >20 与单条 >200 字都按 400 invalid_param 拒绝。
func TestChapterRulesLimits(t *testing.T) {
	e := newEnv(t)

	tooMany := make([]store.ChapterRule, 0, 21)
	for i := 0; i < 21; i++ {
		tooMany = append(tooMany, store.ChapterRule{Name: fmt.Sprintf("r%d", i), Pattern: "^第\\d+章"})
	}
	requireError(t, e.putJSON(chapterRulesPath, map[string]any{"rules": tooMany}),
		http.StatusBadRequest, codeInvalidParam)

	tooLong := []store.ChapterRule{{Name: "太长", Pattern: "^" + strings.Repeat("a", 199) + "$"}}
	requireError(t, e.putJSON(chapterRulesPath, map[string]any{"rules": tooLong}),
		http.StatusBadRequest, codeInvalidParam)

	// 正好 200 字是可接受的边界值。
	atLimit := []store.ChapterRule{{Name: "边界", Pattern: "^" + strings.Repeat("a", 197) + "$"}}
	if rr := e.putJSON(chapterRulesPath, map[string]any{"rules": atLimit}); rr.Code != http.StatusNoContent {
		t.Fatalf("200 字边界状态码 = %d，期望 204，body=%s", rr.Code, rr.Body.String())
	}
}
