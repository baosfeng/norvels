package acceptance

// 书籍分类（0015）的验收步骤：手工 PATCH 分类 → 详情 → 书架筛选 → 分类树。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"
)

// registerCategorySteps 注册分类相关步骤。
func registerCategorySteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我把这本书的分类设为 "(.+)" / "(.+)"$`, func(ctx context.Context, l1, l2 string) error {
		return w.patchCategory(map[string]string{"categoryL1": l1, "categoryL2": l2})
	})
	sc.Step("^我把这本书的大分类清空$", func(ctx context.Context) error {
		return w.patchCategory(map[string]string{"categoryL1": ""})
	})
	sc.Step(`^书籍详情里的大分类应为 "(.*)"$`, func(ctx context.Context, want string) error {
		return w.assertCategoryField("categoryL1", want)
	})
	sc.Step(`^书籍详情里的小分类应为 "(.*)"$`, func(ctx context.Context, want string) error {
		return w.assertCategoryField("categoryL2", want)
	})
	sc.Step(`^按大分类 "(.+)" 筛选书架应只返回 (\d+) 本书$`, func(ctx context.Context, l1, want string) error {
		return w.assertCategoryShelf(l1, atoiSafe(want))
	})
	sc.Step(`^分类树里 "(.+)" 下应有 "(.+)" 且书数为 (\d+)$`, func(ctx context.Context, l1, l2, want string) error {
		return w.assertCategoryTree(l1, l2, atoiSafe(want))
	})
}

// patchCategory 对最近导入的书发一次分类补丁。
func (w *world) patchCategory(fields map[string]string) error {
	if w.lastBookID == 0 {
		return fmt.Errorf("尚未导入书籍")
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPatch, fmt.Sprintf("/api/books/%d", w.lastBookID), string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("PATCH 分类状态码 = %d，body=%s", code, body)
	}
	return nil
}

// assertCategoryField 读详情并断言分类字段。
func (w *world) assertCategoryField(field, want string) error {
	var detail map[string]any
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 详情状态码 = %d，body=%s", code, body)
	}
	got, _ := detail[field].(string)
	if got != want {
		return fmt.Errorf("%s = %q，期望 %q", field, got, want)
	}
	return nil
}

// assertCategoryShelf 按大分类筛选书架并断言总数。
func (w *world) assertCategoryShelf(l1 string, want int) error {
	var res struct {
		Total int `json:"total"`
	}
	code, body, err := w.getJSON("/api/books?categoryL1="+l1, &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books?categoryL1=%s 状态码 = %d，body=%s", l1, code, body)
	}
	if res.Total != want {
		return fmt.Errorf("按 %q 筛选总数 = %d，期望 %d", l1, res.Total, want)
	}
	return nil
}

// assertCategoryTree 读分类树并断言某个 (l1, l2) 节点的书数。
func (w *world) assertCategoryTree(l1, l2 string, want int) error {
	var tree struct {
		Items []struct {
			L1    string `json:"l1"`
			Items []struct {
				L2    string `json:"l2"`
				Count int    `json:"count"`
			} `json:"items"`
		} `json:"items"`
	}
	code, body, err := w.getJSON("/api/categories", &tree)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/categories 状态码 = %d，body=%s", code, body)
	}
	for _, node := range tree.Items {
		if node.L1 != l1 {
			continue
		}
		for _, leaf := range node.Items {
			if leaf.L2 == l2 && leaf.Count == want {
				return nil
			}
		}
		return fmt.Errorf("分类 %q 下没有 %q 书数 %d 的节点（%+v）", l1, l2, want, node.Items)
	}
	return fmt.Errorf("分类树里没有大分类 %q（%+v）", l1, tree.Items)
}
