package acceptance

// P1 阅读体验（阅读状态 + 书签）的验收步骤。
// 与既有场景一致：全部 fixture 合成，走真实 api + SQLite，不 mock。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"
)

// bookmarkParaIndex 是验收场景里固定的书签段落下标。
const bookmarkParaIndex = 2

// registerBookmarkSteps 注册阅读状态与书签的步骤（分组注册，保持单函数短小）。
func registerBookmarkSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我把这本书的阅读状态设为 "(.+)"$`, func(ctx context.Context, status string) error {
		return w.patchReadStatus(status)
	})
	sc.Step(`^我用非法阅读状态 "(.+)" 更新这本书$`, func(ctx context.Context, status string) error {
		return w.patchReadStatus(status)
	})
	sc.Step(`^书籍详情里的阅读状态应为 "(.+)"$`, func(ctx context.Context, want string) error {
		return w.assertReadStatus(want)
	})
	sc.Step(`^按阅读状态 "(.+)" 筛选书架应只返回 (\d+) 本书$`, func(ctx context.Context, status, want string) error {
		return w.assertShelfFilter(status, atoiSafe(want))
	})
	sc.Step("^我把阅读进度更新到第一章$", func(ctx context.Context) error {
		return w.saveFirstChapterProgress()
	})
	registerBookmarkMutationSteps(sc, w)
}

// registerBookmarkMutationSteps 注册书签增删与删书场景的步骤。
func registerBookmarkMutationSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^我两次在同一位置添加书签$", func(ctx context.Context) error {
		return w.addBookmarkTwice()
	})
	sc.Step("^书签总数应为 (\\d+)$", func(ctx context.Context, want string) error {
		return w.assertBookmarkTotal(atoiSafe(want))
	})
	sc.Step("^第二次添加应返回 (\\d+)$", func(ctx context.Context, want string) error {
		return w.assertLastCode(atoiSafe(want))
	})
	sc.Step("^我添加一条书签$", func(ctx context.Context) error {
		return w.addBookmarkOnce()
	})
	sc.Step("^我删除这条书签$", func(ctx context.Context) error {
		return w.deleteLastBookmark()
	})
	sc.Step("^书籍详情里的书签数应为 (\\d+)$", func(ctx context.Context, want string) error {
		return w.assertDetailBookmarkCount(atoiSafe(want))
	})
	sc.Step("^再次删除同一条书签应返回 (\\d+)$", func(ctx context.Context, want string) error {
		return w.deleteLastBookmarkThenAssert(atoiSafe(want))
	})
	sc.Step("^我删除这本书$", func(ctx context.Context) error {
		return w.deleteBook()
	})
	sc.Step("^书架应当为空$", func(ctx context.Context) error {
		return w.assertShelfEmpty()
	})
	sc.Step("^书签列表应当返回 (\\d+)$", func(ctx context.Context, want string) error {
		return w.assertBookmarkListCode(atoiSafe(want))
	})
}

// patchReadStatus 发一次 PATCH readStatus（合法与否都由调用方断言状态码）。
func (w *world) patchReadStatus(status string) error {
	if w.lastBookID == 0 {
		return fmt.Errorf("尚未导入书籍")
	}
	payload, err := json.Marshal(map[string]string{"readStatus": status})
	if err != nil {
		return err
	}
	_, _, err = w.doJSON(http.MethodPatch, fmt.Sprintf("/api/books/%d", w.lastBookID), string(payload))
	return err
}

// assertReadStatus 读详情并断言 readStatus。
func (w *world) assertReadStatus(want string) error {
	var detail struct {
		ReadStatus string `json:"readStatus"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 详情状态码 = %d，body=%s", code, body)
	}
	if detail.ReadStatus != want {
		return fmt.Errorf("readStatus = %q，期望 %q", detail.ReadStatus, want)
	}
	return nil
}

// assertShelfFilter 按阅读状态筛选书架并断言总数。
func (w *world) assertShelfFilter(status string, want int) error {
	var res struct {
		Total int `json:"total"`
	}
	code, body, err := w.getJSON("/api/books?readStatus="+status, &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books?readStatus=%s 状态码 = %d，body=%s", status, code, body)
	}
	if res.Total != want {
		return fmt.Errorf("按 %q 筛选总数 = %d，期望 %d", status, res.Total, want)
	}
	return nil
}

// saveFirstChapterProgress 写一次阅读进度（percent=0.3）。
func (w *world) saveFirstChapterProgress() error {
	payload, err := json.Marshal(map[string]any{
		"bookId": w.lastBookID, "chapterId": w.firstChapter, "percent": 0.3,
	})
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPut, "/api/reading/progress", string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusNoContent {
		return fmt.Errorf("PUT 进度状态码 = %d，期望 204，body=%s", code, body)
	}
	return nil
}

// postBookmark 发一次 POST 书签。
func (w *world) postBookmark(label string) (int, []byte, error) {
	payload, err := json.Marshal(map[string]any{
		"chapterId": w.firstChapter, "paraIndex": bookmarkParaIndex, "label": label,
	})
	if err != nil {
		return 0, nil, err
	}
	return w.doJSON(http.MethodPost, fmt.Sprintf("/api/books/%d/bookmarks", w.lastBookID), string(payload))
}

// addBookmarkTwice 在同一位置连发两次 POST；lastCode 停第二次的结果。
func (w *world) addBookmarkTwice() error {
	if _, _, err := w.postBookmark("第一次标注"); err != nil {
		return err
	}
	code, body, err := w.postBookmark("第二次标注")
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("第二次添加状态码 = %d，期望 200，body=%s", code, body)
	}
	return nil
}

// addBookmarkOnce 添加一条书签并记录 id。
func (w *world) addBookmarkOnce() error {
	code, body, err := w.postBookmark("样本书签")
	if err != nil {
		return err
	}
	if code != http.StatusCreated {
		return fmt.Errorf("POST 书签状态码 = %d，期望 201，body=%s", code, body)
	}
	var bm struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &bm); err != nil {
		return fmt.Errorf("解析书签响应失败: %w (body=%s)", err, body)
	}
	w.lastBookmarkID = bm.ID
	return nil
}

// assertBookmarkTotal 断言全书书签总数。
func (w *world) assertBookmarkTotal(want int) error {
	var res struct {
		Bookmarks []json.RawMessage `json:"bookmarks"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d/bookmarks", w.lastBookID), &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 书签列表状态码 = %d，body=%s", code, body)
	}
	if len(res.Bookmarks) != want {
		return fmt.Errorf("书签总数 = %d，期望 %d", len(res.Bookmarks), want)
	}
	return nil
}

// assertDetailBookmarkCount 断言详情里的 bookmarkCount。
func (w *world) assertDetailBookmarkCount(want int) error {
	var detail struct {
		BookmarkCount int `json:"bookmarkCount"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 详情状态码 = %d，body=%s", code, body)
	}
	if detail.BookmarkCount != want {
		return fmt.Errorf("bookmarkCount = %d，期望 %d", detail.BookmarkCount, want)
	}
	return nil
}

// deleteLastBookmark 删除已记录的书签，期望 204。
func (w *world) deleteLastBookmark() error {
	return w.deleteLastBookmarkThenAssert(http.StatusNoContent)
}

// deleteLastBookmarkThenAssert 删除已记录的书签并断言状态码。
func (w *world) deleteLastBookmarkThenAssert(want int) error {
	code, body, err := w.doJSON(http.MethodDelete, fmt.Sprintf("/api/bookmarks/%d", w.lastBookmarkID), "")
	if err != nil {
		return err
	}
	return w.assertLastCodeWith(want, code, body)
}

// deleteBook 删除当前书，期望 204。
func (w *world) deleteBook() error {
	code, body, err := w.doJSON(http.MethodDelete, fmt.Sprintf("/api/books/%d", w.lastBookID), "")
	if err != nil {
		return err
	}
	return w.assertLastCodeWith(http.StatusNoContent, code, body)
}

// assertBookmarkListCode 断言书签列表请求的状态码（书删掉后应为 404）。
func (w *world) assertBookmarkListCode(want int) error {
	code, body, err := w.doJSON(http.MethodGet, fmt.Sprintf("/api/books/%d/bookmarks", w.lastBookID), "")
	if err != nil {
		return err
	}
	return w.assertLastCodeWith(want, code, body)
}

// assertShelfEmpty 断言书架为空。
func (w *world) assertShelfEmpty() error {
	var res struct {
		Total int `json:"total"`
	}
	code, body, err := w.getJSON("/api/books", &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books 状态码 = %d，body=%s", code, body)
	}
	if res.Total != 0 {
		return fmt.Errorf("书架总数 = %d，期望 0", res.Total)
	}
	return nil
}

// assertLastCode 断言最近一次请求的状态码。
func (w *world) assertLastCode(want int) error {
	return w.assertLastCodeWith(want, w.lastCode, w.lastBody)
}

// assertLastCodeWith 断言给定状态码并带上响应体，便于定位。
func (w *world) assertLastCodeWith(want, got int, body []byte) error {
	if got != want {
		return fmt.Errorf("状态码 = %d，期望 %d，body=%s", got, want, truncate(string(body), 200))
	}
	return nil
}
