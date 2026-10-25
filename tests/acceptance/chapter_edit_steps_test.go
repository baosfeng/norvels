package acceptance

// 章节结构编辑（M1/M2）的动作步骤：改标题、拆章、批量删除、合并、按序号重命名。
// 与既有场景同一口径：走真实 api + SQLite，不 mock；失败路径只发请求，状态码交给断言步骤。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"
)

// registerChapterStructureSteps 注册章节结构编辑的步骤。
func registerChapterStructureSteps(sc *godog.ScenarioContext, w *world) {
	registerChapterEditSteps(sc, w)
	registerChapterEditAssertSteps(sc, w)
}

// registerChapterEditSteps 注册会改库的动作。
func registerChapterEditSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^我已导入结构合成书稿$", func(ctx context.Context) error {
		return w.importStructBook()
	})
	sc.Step(`^我改第 (\d+) 章的标题为 "(.+)"$`, func(ctx context.Context, n, title string) error {
		return w.patchChapterTitle(atoiSafe(n), title)
	})
	sc.Step(`^我把第 (\d+) 章在第 (\d+) 个段落之后拆开$`, func(ctx context.Context, n, para string) error {
		return w.splitChapterAfter(atoiSafe(n), atoiSafe(para))
	})
	sc.Step(`^我批量删除第 (\d+)、(\d+) 章$`, func(ctx context.Context, a, b string) error {
		return w.batchDeleteChapters(atoiSafe(a), atoiSafe(b))
	})
	sc.Step(`^我合并第 (\d+)、(\d+) 章$`, func(ctx context.Context, a, b string) error {
		return w.mergeChapters(atoiSafe(a), atoiSafe(b))
	})
	sc.Step("^我把整本按序号重命名$", func(ctx context.Context) error {
		return w.renumberBook()
	})
	sc.Step("^我另导入一本结构无关的短书$", func(ctx context.Context) error {
		return w.importShortBook()
	})
	sc.Step("^我用本书的 id 批量删除短书的章节$", func(ctx context.Context) error {
		return w.batchDeleteOtherBookChapter()
	})
}

// registerChapterEditAssertSteps 注册守恒与错误路径的断言。
func registerChapterEditAssertSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^该书第 (\d+) 章的标题应为 "(.+)"$`, func(ctx context.Context, n, want string) error {
		return w.assertChapterTitle(atoiSafe(n), want)
	})
	sc.Step(`^该书的章序应为 "(.+)"$`, func(ctx context.Context, want string) error {
		return w.assertChapterOrder(want)
	})
	sc.Step(`^该书的章节数应为 (\d+)$`, func(ctx context.Context, want string) error {
		return w.assertStructureChapterTotal(atoiSafe(want))
	})
	sc.Step(`^书籍详情里的章节计数应为 (\d+)$`, func(ctx context.Context, want string) error {
		return w.assertBookChapterCount(atoiSafe(want))
	})
	sc.Step(`^我已记下第 (\d+) 章的字数$`, func(ctx context.Context, n string) error {
		return w.rememberWords(atoiSafe(n))
	})
	sc.Step(`^我已记下第 (\d+)、(\d+) 章的字数之和$`, func(ctx context.Context, a, b string) error {
		return w.rememberWords(atoiSafe(a), atoiSafe(b))
	})
	sc.Step(`^拆分后的第 (\d+)、(\d+) 章字数之和应等于记下的第 (\d+) 章字数$`,
		func(ctx context.Context, a, b, base string) error {
			return w.assertSplitWordsConserved(atoiSafe(a), atoiSafe(b), atoiSafe(base))
		})
	sc.Step(`^拆分后的第 (\d+)、(\d+) 章正文都应含合成正文标记$`,
		func(ctx context.Context, a, b string) error {
			return w.assertBodiesMarked(atoiSafe(a), atoiSafe(b))
		})
	sc.Step(`^合并接口应返回 (\d+) 章$`, func(ctx context.Context, want string) error {
		return w.assertLastIntField("chapters", atoiSafe(want))
	})
	sc.Step(`^合并后的第 (\d+) 章字数应等于记下的第 (\d+)、(\d+) 章字数之和$`,
		func(ctx context.Context, n, a, b string) error {
			return w.assertMergedWords(atoiSafe(n), atoiSafe(a), atoiSafe(b))
		})
	sc.Step(`^合并后的第 (\d+) 章正文应同时含两段合成正文$`, func(ctx context.Context, n string) error {
		return w.assertMergedBody(atoiSafe(n))
	})
	sc.Step(`^重命名接口应返回 (\d+) 章$`, func(ctx context.Context, want string) error {
		return w.assertLastIntField("renamed", atoiSafe(want))
	})
	sc.Step(`^接口应返回 (\d+) 且错误码为 "(.+)"$`, func(ctx context.Context, code, wantCode string) error {
		return w.assertLastCodeAndError(atoiSafe(code), wantCode)
	})
}

// patchChapterTitle 改一章标题：PATCH 返回 204 且无响应体。
func (w *world) patchChapterTitle(n int, title string) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	ch, err := w.chapterAt(n)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPatch,
		fmt.Sprintf("/api/books/%d/chapters/%d", w.lastBookID, ch.ID), string(body))
	if err != nil {
		return err
	}
	if code != http.StatusNoContent {
		return fmt.Errorf("PATCH 章节标题状态码 = %d，期望 204，body=%s", code, truncate(string(data), 200))
	}
	return nil
}

// splitChapterAfter 在第 para 个段落（1 起）之后拆开一章，并校验返回的章数。
func (w *world) splitChapterAfter(n, para int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	ch, err := w.chapterAt(n)
	if err != nil {
		return err
	}
	if para < 1 {
		return fmt.Errorf("段落序号必须从 1 起，收到 %d", para)
	}
	body, err := json.Marshal(map[string]any{"cutAfter": []int{para - 1}})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPost,
		fmt.Sprintf("/api/books/%d/chapters/%d/split", w.lastBookID, ch.ID), string(body))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("POST 拆分章节状态码 = %d，期望 200，body=%s", code, truncate(string(data), 200))
	}
	// 回包里的 chapters 是「这次拆成了几章」（切点数+1），不是全书章数：
	// 只切一刀就必然拆成 2 章，全书章数另有断言覆盖。
	return w.assertLastIntField("chapters", 2)
}

// batchDeleteChapters 批量删两章：204；其后序号由服务端重排。
func (w *world) batchDeleteChapters(a, b int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	ids := make([]int64, 0, 2)
	for _, n := range []int{a, b} {
		ch, err := w.chapterAt(n)
		if err != nil {
			return err
		}
		ids = append(ids, ch.ID)
	}
	return w.postChapterIDs("/chapters/batch-delete", ids, http.StatusNoContent)
}

// mergeChapters 合并两章：成功 200 + 全书章数，跨卷/不连续 409 not_mergeable。
//
// 这里不校验状态码：同一个步骤既用于成功路径也用于错误路径，
// 状态码由紧随其后的断言步骤检查（「合并接口应返回 N 章」或「接口应返回 409 ...」）。
func (w *world) mergeChapters(a, b int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	ids := make([]int64, 0, 2)
	for _, n := range []int{a, b} {
		ch, err := w.chapterAt(n)
		if err != nil {
			return err
		}
		ids = append(ids, ch.ID)
	}
	return w.postChapterIDsRaw("/chapters/merge", ids)
}

// postChapterIDsRaw 只发一次 {"chapterIds":[...]} 请求，状态码交给断言步骤。
func (w *world) postChapterIDsRaw(path string, ids []int64) error {
	body, err := json.Marshal(map[string]any{"chapterIds": ids})
	if err != nil {
		return err
	}
	_, _, err = w.doJSON(http.MethodPost,
		fmt.Sprintf("/api/books/%d%s", w.lastBookID, path), string(body))
	return err
}

// renumberBook 按 idx+1 重写全书章节标题。
func (w *world) renumberBook() error {
	if w.lastBookID == 0 {
		return fmt.Errorf("尚未导入书籍")
	}
	body, err := json.Marshal(map[string]string{"scope": "book"})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPost,
		fmt.Sprintf("/api/books/%d/chapters/renumber", w.lastBookID), string(body))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("POST 重编号状态码 = %d，期望 200，body=%s", code, truncate(string(data), 200))
	}
	return nil
}

// batchDeleteOtherBookChapter 用本书的 id 去删短书的章节，期望被拒（404 not_found）。
func (w *world) batchDeleteOtherBookChapter() error {
	if w.lastBookID == 0 || w.otherChapterID == 0 {
		return fmt.Errorf("尚未导入短书或本书")
	}
	body, err := json.Marshal(map[string]any{"chapterIds": []int64{w.otherChapterID}})
	if err != nil {
		return err
	}
	_, _, err = w.doJSON(http.MethodPost,
		fmt.Sprintf("/api/books/%d/chapters/batch-delete", w.lastBookID), string(body))
	return err
}

// postChapterIDs 发一次 {"chapterIds":[...]} 请求；状态码不符即报错。
func (w *world) postChapterIDs(path string, ids []int64, want int) error {
	body, err := json.Marshal(map[string]any{"chapterIds": ids})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPost,
		fmt.Sprintf("/api/books/%d%s", w.lastBookID, path), string(body))
	if err != nil {
		return err
	}
	if code != want {
		return fmt.Errorf("POST %s 状态码 = %d，期望 %d，body=%s", path, code, want, truncate(string(data), 200))
	}
	return nil
}

// rememberWords 记下若干章的字数之和，作为拆章/合并的守恒基准。
func (w *world) rememberWords(ns ...int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	sum := 0
	for _, n := range ns {
		ch, err := w.chapterAt(n)
		if err != nil {
			return err
		}
		if ch.WordCount <= 0 {
			return fmt.Errorf("第 %d 章字数 = %d，无法作为守恒基准", n, ch.WordCount)
		}
		sum += ch.WordCount
	}
	w.wordBase = sum
	return nil
}
