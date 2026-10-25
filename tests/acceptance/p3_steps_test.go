package acceptance

// P3 增量（评分 / 书单 / 系列聚合 / 阅读时长与历史 / 管理侧补全）的验收步骤。
// 与既有场景一致：全部走真实 api + SQLite，fixture 全是合成数据。
// 按领域拆文件是为了守住单文件 400 行门禁。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"
)

// registerP3Steps 按领域分组注册 P3 步骤（单函数保持短小）。
func registerP3Steps(sc *godog.ScenarioContext, w *world) {
	registerP3RatingSteps(sc, w)
	registerP3CollectionSteps(sc, w)
	registerP3SeriesSteps(sc, w)
	registerP3ReadingSteps(sc, w)
	registerP3AdminSteps(sc, w)
}

func registerP3RatingSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我把这本书的评分设为 (\d+) 分$`, func(ctx context.Context, n string) error {
		return w.patchBookJSON(fmt.Sprintf(`{"rating":%s}`, n))
	})
	sc.Step(`^书架按评分倒序时这本书应当排第一$`, func(ctx context.Context) error {
		return w.assertRatingSortFirst()
	})
	sc.Step(`^按评分 (\d+) 筛选应当命中 (\d+) 本书$`, func(ctx context.Context, rating, want string) error {
		return w.assertRatingFilter(atoiSafe(rating), atoiSafe(want))
	})
	sc.Step(`^书籍详情里的评分应当是 (\d+)$`, func(ctx context.Context, want string) error {
		return w.assertBookRating(atoiSafe(want))
	})
	sc.Step(`^我批量把这本书的评分设为 (\d+) 分$`, func(ctx context.Context, n string) error {
		return w.batchRateBook(atoiSafe(n))
	})
	sc.Step(`^批量结果应当是更新 (\d+) 本、失败 (\d+) 本$`, func(ctx context.Context, updated, failed string) error {
		return w.assertBatchResult(atoiSafe(updated), atoiSafe(failed))
	})
}

func registerP3CollectionSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我把这本书加入名为 "(.+)" 的书单$`, func(ctx context.Context, name string) error {
		return w.addBookToCollection(name)
	})
	sc.Step(`^书单 "(.+)" 里应当有 (\d+) 本书$`, func(ctx context.Context, name, want string) error {
		return w.assertCollectionBookCount(name, atoiSafe(want))
	})
	sc.Step(`^按书单 "(.+)" 筛选应当命中 (\d+) 本书$`, func(ctx context.Context, name, want string) error {
		return w.assertCollectionFilter(name, atoiSafe(want))
	})
}

func registerP3SeriesSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我把这本书的系列设为 "(.+)"$`, func(ctx context.Context, name string) error {
		return w.patchBookJSON(fmt.Sprintf(`{"series":%q,"seriesIndex":1}`, name))
	})
	sc.Step(`^系列列表里应当有 "(.+)"$`, func(ctx context.Context, name string) error {
		return w.assertSeriesListed(name)
	})
	sc.Step(`^系列 "(.+)" 的详情应当包含这本书$`, func(ctx context.Context, name string) error {
		return w.assertSeriesDetail(name)
	})
}

// patchBookJSON 改当前这本书的元信息，期望 200。
func (w *world) patchBookJSON(body string) error {
	code, data, err := w.doJSON(http.MethodPatch, fmt.Sprintf("/api/books/%d", w.lastBookID), body)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("PATCH /api/books/%d 状态码 = %d，body=%s", w.lastBookID, code, data)
	}
	return nil
}

// bookSummaryPage 是 GET /api/books 的响应（只取断言需要的字段）。
type bookSummaryPage struct {
	Total int `json:"total"`
	Items []struct {
		ID     int64 `json:"id"`
		Rating int   `json:"rating"`
	} `json:"items"`
}

// assertRatingSortFirst 断言 sort=rating 时当前这本书排第一。
func (w *world) assertRatingSortFirst() error {
	var res bookSummaryPage
	code, body, err := w.getJSON("/api/books?sort=rating", &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books?sort=rating 状态码 = %d，body=%s", code, body)
	}
	if len(res.Items) == 0 || res.Items[0].ID != w.lastBookID {
		return fmt.Errorf("sort=rating 首本 = %+v，期望当前书 %d", res.Items, w.lastBookID)
	}
	return nil
}

// assertRatingFilter 断言按评分筛选的总数。
func (w *world) assertRatingFilter(rating, want int) error {
	var res bookSummaryPage
	code, body, err := w.getJSON(fmt.Sprintf("/api/books?rating=%d", rating), &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books?rating= 状态码 = %d，body=%s", code, body)
	}
	if res.Total != want {
		return fmt.Errorf("评分 %d 命中 %d 本，期望 %d", rating, res.Total, want)
	}
	return nil
}

// bookRating 读当前这本书的评分。
func (w *world) bookRating() (int, error) {
	var detail struct {
		Rating int `json:"rating"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &detail)
	if err != nil {
		return 0, err
	}
	if code != http.StatusOK {
		return 0, fmt.Errorf("GET 书详情状态码 = %d，body=%s", code, body)
	}
	return detail.Rating, nil
}

func (w *world) assertBookRating(want int) error {
	got, err := w.bookRating()
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("书籍评分 = %d，期望 %d", got, want)
	}
	return nil
}

// batchRateBook 批量改当前这本书的评分并记录结果条数。
func (w *world) batchRateBook(rating int) error {
	payload := fmt.Sprintf(`{"ids":[%d],"patch":{"rating":%d}}`, w.lastBookID, rating)
	code, data, err := w.doJSON(http.MethodPost, "/api/books/batch", payload)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("POST /api/books/batch 状态码 = %d，body=%s", code, data)
	}
	var res struct {
		Updated []int64 `json:"updated"`
		Failed  []struct {
			ID int64 `json:"id"`
		} `json:"failed"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return fmt.Errorf("解析批量结果失败: %w", err)
	}
	w.p3BatchUpdated, w.p3BatchFailed = len(res.Updated), len(res.Failed)
	return nil
}

func (w *world) assertBatchResult(updated, failed int) error {
	if w.p3BatchUpdated != updated || w.p3BatchFailed != failed {
		return fmt.Errorf("批量结果 = 更新 %d / 失败 %d，期望 更新 %d / 失败 %d",
			w.p3BatchUpdated, w.p3BatchFailed, updated, failed)
	}
	return nil
}
