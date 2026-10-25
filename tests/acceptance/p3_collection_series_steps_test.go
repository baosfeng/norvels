package acceptance

// P3 书单与系列聚合的验收断言（与评分共用 registerP3Steps 的分组注册）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// collectionIDByName 找到书单 id 与书数。
func (w *world) collectionIDByName(name string) (int64, int, error) {
	var res struct {
		Items []struct {
			ID        int64  `json:"id"`
			Name      string `json:"name"`
			BookCount int    `json:"bookCount"`
		} `json:"items"`
	}
	code, body, err := w.getJSON("/api/collections", &res)
	if err != nil {
		return 0, 0, err
	}
	if code != http.StatusOK {
		return 0, 0, fmt.Errorf("GET /api/collections 状态码 = %d，body=%s", code, body)
	}
	for _, it := range res.Items {
		if it.Name == name {
			return it.ID, it.BookCount, nil
		}
	}
	return 0, 0, fmt.Errorf("书单 %q 不存在（现有 %+v）", name, res.Items)
}

// addBookToCollection 建书单（已存在则复用）并全量替换为当前这本书。
func (w *world) addBookToCollection(name string) error {
	id, _, err := w.collectionIDByName(name)
	if err != nil {
		payload, _ := json.Marshal(map[string]string{"name": name})
		code, body, cerr := w.doJSON(http.MethodPost, "/api/collections", string(payload))
		if cerr != nil {
			return cerr
		}
		if code != http.StatusCreated {
			return fmt.Errorf("建书单状态码 = %d，body=%s", code, body)
		}
		var created struct {
			ID int64 `json:"id"`
		}
		if uerr := json.Unmarshal(body, &created); uerr != nil {
			return fmt.Errorf("解析书单响应失败: %w", uerr)
		}
		id = created.ID
	}
	payload := fmt.Sprintf(`{"bookIds":[%d]}`, w.lastBookID)
	code, body, err := w.doJSON(http.MethodPut, fmt.Sprintf("/api/collections/%d/books", id), payload)
	if err != nil {
		return err
	}
	if code != http.StatusNoContent {
		return fmt.Errorf("PUT 书单书目状态码 = %d，body=%s", code, body)
	}
	return nil
}

func (w *world) assertCollectionBookCount(name string, want int) error {
	_, count, err := w.collectionIDByName(name)
	if err != nil {
		return err
	}
	if count != want {
		return fmt.Errorf("书单 %q 书数 = %d，期望 %d", name, count, want)
	}
	return nil
}

func (w *world) assertCollectionFilter(name string, want int) error {
	id, _, err := w.collectionIDByName(name)
	if err != nil {
		return err
	}
	var res bookSummaryPage
	code, body, err := w.getJSON(fmt.Sprintf("/api/books?collection=%d", id), &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("按书单筛状态码 = %d，body=%s", code, body)
	}
	if res.Total != want {
		return fmt.Errorf("书单 %q 筛出 %d 本，期望 %d", name, res.Total, want)
	}
	return nil
}

func (w *world) assertSeriesListed(name string) error {
	var res struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	code, body, err := w.getJSON("/api/series", &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/series 状态码 = %d，body=%s", code, body)
	}
	for _, it := range res.Items {
		if it.Name == name {
			return nil
		}
	}
	return fmt.Errorf("系列列表里没有 %q（现有 %+v）", name, res.Items)
}

func (w *world) assertSeriesDetail(name string) error {
	var res struct {
		Books []struct {
			ID int64 `json:"id"`
		} `json:"books"`
	}
	code, body, err := w.getJSON("/api/series/"+url.PathEscape(name), &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 系列详情状态码 = %d，body=%s", code, body)
	}
	for _, b := range res.Books {
		if b.ID == w.lastBookID {
			return nil
		}
	}
	return fmt.Errorf("系列 %q 的书目里没有当前书 %d（%+v）", name, w.lastBookID, res.Books)
}
