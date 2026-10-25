package acceptance

// P1-B 导入侧（词表打标 / 单文件重试 / dry-run 预检）的验收步骤。
// 与既有场景一致：全部 fixture 合成，走真实 api + SQLite，不 mock。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cucumber/godog"
)

// registerP1BSteps 注册 P1-B 的三组步骤。
func registerP1BSteps(sc *godog.ScenarioContext, w *world) {
	registerTagRuleSteps(sc, w)
	registerDryRunSteps(sc, w)
	registerRetrySteps(sc, w)
}

// registerTagRuleSteps 词表规则与自动打标。
func registerTagRuleSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^我保存两条词表规则$", func(ctx context.Context) error {
		return w.saveTagRules([]string{"奇幻", "科幻"}, "")
	})
	sc.Step(`^词表规则应当有 (\d+) 条且首个标签为 "(.+)"$`, func(ctx context.Context, n, tag string) error {
		return w.assertTagRules(atoiSafe(n), tag)
	})
	sc.Step("^非法词表规则应当被拒绝$", func(ctx context.Context) error {
		return w.assertBadTagRules()
	})
	sc.Step(`^我保存词表规则标签 "(.+)" 关键词 "(.+)"$`, func(ctx context.Context, tag, kw string) error {
		return w.saveTagRules([]string{tag}, kw)
	})
	sc.Step(`^这本书的标签应当包含 "(.+)"$`, func(ctx context.Context, want string) error {
		return w.assertBookHasTag(want)
	})
	sc.Step(`^这本书的标签应当只有 (\d+) 个$`, func(ctx context.Context, n string) error {
		return w.assertBookTagCount(atoiSafe(n))
	})
	sc.Step(`^我把这本书的标签设为 "(.+)"$`, func(ctx context.Context, tag string) error {
		return w.setBookTags([]string{tag})
	})
	sc.Step("^我在覆盖模式下再次导入合成书稿$", func(ctx context.Context) error {
		return w.importDirForce()
	})
}

// ---------- world 方法 ----------

// saveTagRules 全量保存词表规则（可选给关键词）。
func (w *world) saveTagRules(tags []string, keyword string) error {
	if keyword == "" {
		keyword = "词"
	}
	rules := make([]map[string]any, 0, len(tags))
	for _, tag := range tags {
		rules = append(rules, map[string]any{"tag": tag, "keywords": []string{keyword}})
	}
	payload, err := json.Marshal(map[string]any{"rules": rules})
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPut, "/api/settings/tag-rules", string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusNoContent {
		return fmt.Errorf("PUT tag-rules 状态码 = %d，body=%s", code, body)
	}
	return nil
}

// assertTagRules 断言回显的条数与首个标签。
func (w *world) assertTagRules(want int, firstTag string) error {
	var res struct {
		Rules []struct {
			Tag string `json:"tag"`
		} `json:"rules"`
	}
	code, body, err := w.getJSON("/api/settings/tag-rules", &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET tag-rules 状态码 = %d，body=%s", code, body)
	}
	if len(res.Rules) != want || res.Rules[0].Tag != firstTag {
		return fmt.Errorf("词表规则 = %+v，期望 %da 条且首个为 %q", res.Rules, want, firstTag)
	}
	return nil
}

// assertBadTagRules 造 51 条规则，断言 400 参数错误。
func (w *world) assertBadTagRules() error {
	tags := make([]string, 0, maxTagRulesForTest+1)
	for i := 0; i <= maxTagRulesForTest; i++ {
		tags = append(tags, fmt.Sprintf("标签%d", i))
	}
	payload, err := json.Marshal(map[string]any{"rules": tagRulesJSON(tags)})
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPut, "/api/settings/tag-rules", string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusBadRequest {
		return fmt.Errorf("51 条规则状态码 = %d，期望 400，body=%s", code, body)
	}
	return w.assertParamError(http.StatusBadRequest)
}

// tagRulesJSON 把标签名列表转成规则对象列表。
func tagRulesJSON(tags []string) []map[string]any {
	out := make([]map[string]any, 0, len(tags))
	for _, tag := range tags {
		out = append(out, map[string]any{"tag": tag, "keywords": []string{"词"}})
	}
	return out
}

// assertBookHasTag 断言详情 tags 含指定标签。
func (w *world) assertBookHasTag(want string) error {
	tags, err := w.bookTags()
	if err != nil {
		return err
	}
	for _, tag := range tags {
		if tag == want {
			return nil
		}
	}
	return fmt.Errorf("书籍标签 = %v，期望包含 %q", tags, want)
}

// assertBookTagCount 断言详情 tags 条数。
func (w *world) assertBookTagCount(want int) error {
	tags, err := w.bookTags()
	if err != nil {
		return err
	}
	if len(tags) != want {
		return fmt.Errorf("书籍标签 = %v，期望 %d 个", tags, want)
	}
	return nil
}

// bookTags 读当前书的标签。
func (w *world) bookTags() ([]string, error) {
	var detail struct {
		Tags []string `json:"tags"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &detail)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("GET 详情状态码 = %d，body=%s", code, body)
	}
	return detail.Tags, nil
}

// setBookTags 通过 PATCH 全量替换标签。
func (w *world) setBookTags(tags []string) error {
	payload, err := json.Marshal(map[string]any{"tags": tags})
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPatch, fmt.Sprintf("/api/books/%d", w.lastBookID), string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("PATCH tags 状态码 = %d，body=%s", code, body)
	}
	return nil
}

// importDirForce 覆盖模式重导合成书稿（会重新打标）。
func (w *world) importDirForce() error {
	payload, err := json.Marshal(map[string]any{"dir": w.libsr, "recursive": false, "force": true})
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPost, "/api/import", string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusAccepted {
		return fmt.Errorf("覆盖导入状态码 = %d，body=%s", code, body)
	}
	var resp struct {
		JobID int64 `json:"jobId"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	if _, err := w.waitJob(resp.JobID, 15*time.Second); err != nil {
		return err
	}
	return w.captureBook()
}

// maxTagRulesForTest 与 api 层的词表条数上限一致（这里只用来造非法输入）。
const maxTagRulesForTest = 50
