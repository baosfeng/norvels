package acceptance

// P3 管理侧补全（出版信息 / 新建章节 / 标签维护）的验收断言。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"
)

func registerP3AdminSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我把这本书的出版信息设为 "(.+)" / "(.+)"$`, func(ctx context.Context, publisher, date string) error {
		return w.patchBookJSON(fmt.Sprintf(`{"publisher":%q,"pubDate":%q}`, publisher, date))
	})
	sc.Step(`^书籍详情里的出版信息应当是 "(.+)" / "(.+)"$`, func(ctx context.Context, publisher, date string) error {
		return w.assertPublishing(publisher, date)
	})
	sc.Step("^提交非法出版日期应当返回 400$", func(ctx context.Context) error {
		return w.assertBadPubDateRejected()
	})
	sc.Step(`^我在书末新增一章 "(.+)"$`, func(ctx context.Context, title string) error {
		return w.createChapter(title)
	})
	sc.Step(`^最后一章的标题应当是 "(.+)"$`, func(ctx context.Context, title string) error {
		return w.assertLastChapterTitle(title)
	})
	sc.Step(`^我把这本书打上标签 "(.+)"$`, func(ctx context.Context, name string) error {
		return w.patchBookJSON(fmt.Sprintf(`{"tags":[%q]}`, name))
	})
	sc.Step(`^标签列表里应当有 "(.+)"$`, func(ctx context.Context, name string) error {
		return w.assertTagListed(name)
	})
	sc.Step(`^标签列表里不应当有 "(.+)"$`, func(ctx context.Context, name string) error {
		return w.assertTagAbsent(name)
	})
	sc.Step(`^标签 "(.+)" 应当挂在 (\d+) 本书上$`, func(ctx context.Context, name, want string) error {
		return w.assertTagBookCount(name, atoiSafe(want))
	})
	sc.Step(`^我把标签 "(.+)" 改名为 "(.+)"$`, func(ctx context.Context, oldName, newName string) error {
		return w.renameTag(oldName, newName)
	})
	sc.Step(`^我删除标签 "(.+)"$`, func(ctx context.Context, name string) error {
		return w.deleteTag(name)
	})
}

func (w *world) assertPublishing(publisher, date string) error {
	var detail struct {
		Publisher string `json:"publisher"`
		PubDate   string `json:"pubDate"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 书详情状态码 = %d，body=%s", code, body)
	}
	if detail.Publisher != publisher || detail.PubDate != date {
		return fmt.Errorf("出版信息 = %q / %q，期望 %q / %q",
			detail.Publisher, detail.PubDate, publisher, date)
	}
	return nil
}

func (w *world) assertBadPubDateRejected() error {
	code, body, err := w.doJSON(http.MethodPatch,
		fmt.Sprintf("/api/books/%d", w.lastBookID), `{"pubDate":"2024/05/01"}`)
	if err != nil {
		return err
	}
	if code != http.StatusBadRequest {
		return fmt.Errorf("非法 pubDate 状态码 = %d，期望 400，body=%s", code, body)
	}
	return nil
}

func (w *world) createChapter(title string) error {
	payload, err := json.Marshal(map[string]string{"title": title, "content": "合成新增章节正文。"})
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPost,
		fmt.Sprintf("/api/books/%d/chapters", w.lastBookID), string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusCreated {
		return fmt.Errorf("建章状态码 = %d，期望 201，body=%s", code, body)
	}
	return nil
}

func (w *world) assertLastChapterTitle(want string) error {
	var detail struct {
		Volumes []struct {
			Chapters []struct {
				Title string `json:"title"`
			} `json:"chapters"`
		} `json:"volumes"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 书详情状态码 = %d，body=%s", code, body)
	}
	last := ""
	for _, v := range detail.Volumes {
		for _, ch := range v.Chapters {
			last = ch.Title
		}
	}
	if last != want {
		return fmt.Errorf("最后一章标题 = %q，期望 %q", last, want)
	}
	return nil
}

// tagRow 是 GET /api/tags 的一行。
type tagRow struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"bookCount"`
}

func (w *world) tags() ([]tagRow, error) {
	var res struct {
		Items []tagRow `json:"items"`
	}
	code, body, err := w.getJSON("/api/tags", &res)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("GET /api/tags 状态码 = %d，body=%s", code, body)
	}
	return res.Items, nil
}

// tagIDByName 找标签 id 与书籍数。
func (w *world) tagIDByName(name string) (int64, int, error) {
	items, err := w.tags()
	if err != nil {
		return 0, 0, err
	}
	for _, it := range items {
		if it.Name == name {
			return it.ID, it.BookCount, nil
		}
	}
	return 0, 0, fmt.Errorf("标签 %q 不存在（现有 %+v）", name, items)
}

func (w *world) assertTagListed(name string) error {
	_, _, err := w.tagIDByName(name)
	return err
}

func (w *world) assertTagAbsent(name string) error {
	items, err := w.tags()
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Name == name {
			return fmt.Errorf("标签列表里仍有 %q", name)
		}
	}
	return nil
}

func (w *world) assertTagBookCount(name string, want int) error {
	_, count, err := w.tagIDByName(name)
	if err != nil {
		return err
	}
	if count != want {
		return fmt.Errorf("标签 %q 挂在 %d 本书上，期望 %d", name, count, want)
	}
	return nil
}

func (w *world) renameTag(oldName, newName string) error {
	id, _, err := w.tagIDByName(oldName)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"name": newName})
	code, body, err := w.doJSON(http.MethodPatch, fmt.Sprintf("/api/tags/%d", id), string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("改标签状态码 = %d，body=%s", code, body)
	}
	return nil
}

func (w *world) deleteTag(name string) error {
	id, _, err := w.tagIDByName(name)
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodDelete, fmt.Sprintf("/api/tags/%d", id), "")
	if err != nil {
		return err
	}
	if code != http.StatusNoContent {
		return fmt.Errorf("删标签状态码 = %d，期望 204，body=%s", code, body)
	}
	return nil
}
