package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// registerSteps 注册全部步骤定义。
//
// 注意：godog 会先剥掉「假定/当/那么/并且/但是」这类关键字，正则只匹配剩下的步骤正文，
// 所以这里的正则一律不带关键字前缀。
// 具体断言拆到各个 register*Steps 与 world 方法里，是为了让本文件自身也满足
// 「单函数 ≤ 70 行」的门禁口径。
func registerSteps(sc *godog.ScenarioContext, w *world) {
	registerBackgroundSteps(sc, w)
	registerReadingSteps(sc, w)
	registerSearchSteps(sc, w)
	registerImportFailureSteps(sc, w)
	registerUploadSteps(sc, w)
	registerP1Steps(sc, w)
	registerCommonAssertSteps(sc, w)
}

// registerBackgroundSteps 注册 feature 里的公共背景。
func registerBackgroundSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^一个空的图书馆数据库$", func(ctx context.Context) error {
		return w.ensureDB()
	})
	sc.Step("^我已导入合成书稿$", func(ctx context.Context) error {
		if err := w.ensureDB(); err != nil {
			return err
		}
		if err := w.writeSynthBook(); err != nil {
			return fmt.Errorf("写入合成书稿失败: %w", err)
		}
		return w.importDir(w.libsr)
	})
}

// registerReadingSteps 注册「导入 → 书架 → 阅读」主流程的步骤。
func registerReadingSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^我把合成书稿目录导入图书馆$", func(ctx context.Context) error {
		if err := w.ensureDB(); err != nil {
			return err
		}
		if err := w.writeSynthBook(); err != nil {
			return fmt.Errorf("写入合成书稿失败: %w", err)
		}
		return w.importDir(w.libsr)
	})
	sc.Step(`^导入任务状态应为 "(.+)"$`, func(ctx context.Context, want string) error {
		if w.lastJobStatus != want {
			return fmt.Errorf("最近一次导入任务 %d 状态 = %q，期望 %q", w.lastJobID, w.lastJobStatus, want)
		}
		return nil
	})
	sc.Step("^书架里应当有 (\\d+) 本书$", func(ctx context.Context, want string) error {
		return w.assertShelfCount(atoiSafe(want))
	})
	sc.Step("^书籍详情应当包含 (\\d+) 章$", func(ctx context.Context, want string) error {
		return w.assertChapterCount(atoiSafe(want))
	})
	sc.Step("^第一章正文应当包含合成正文标记$", func(ctx context.Context) error {
		return w.assertFirstChapterBody()
	})
}

// registerSearchSteps 注册全文搜索的步骤。
func registerSearchSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我搜索关键词 "(.+)"$`, func(ctx context.Context, kw string) error {
		return w.search(kw)
	})
	sc.Step("^搜索结果应当至少命中 (\\d+) 条章节$", func(ctx context.Context, want string) error {
		if w.hitChapters < atoiSafe(want) {
			return fmt.Errorf("章节命中数 = %d，期望 >= %s", w.hitChapters, want)
		}
		return nil
	})
	sc.Step("^命中结果应当带书籍标题与章节号$", func(ctx context.Context) error {
		if w.hitTitle != synthBookTitle {
			return fmt.Errorf("命中结果书名 = %q，期望 %q", w.hitTitle, synthBookTitle)
		}
		if w.hitIdx < 0 {
			return fmt.Errorf("命中结果章节号 = %d，期望 >= 0", w.hitIdx)
		}
		return nil
	})
	sc.Step("^搜索结果应当为空$", func(ctx context.Context) error {
		if w.hitChapters != 0 || w.hitBooks != 0 {
			return fmt.Errorf("搜索结果非空：章节 %d 条、书籍 %d 条", w.hitChapters, w.hitBooks)
		}
		return nil
	})
	sc.Step("^我用 (\\d+) 个字的关键词请求搜索$", func(ctx context.Context, n string) error {
		return w.requestTooShortQuery(atoiSafe(n))
	})
}

// registerImportFailureSteps 注册「导入失败可查」的步骤。
func registerImportFailureSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^我导入一个没有任何 txt 文件的空目录$", func(ctx context.Context) error {
		return w.importEmptyDir()
	})
	sc.Step("^任务详情里应当能查到失败原因$", func(ctx context.Context) error {
		return w.assertJobFailureMessage()
	})
	sc.Step("^我导入一个不存在的目录$", func(ctx context.Context) error {
		return w.importMissingDir()
	})
}

// registerCommonAssertSteps 注册通用断言步骤。
func registerCommonAssertSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^接口应当返回 (\\d+) 参数错误$", func(ctx context.Context, want string) error {
		return w.assertParamError(atoiSafe(want))
	})
}

// ---------- world 上的步骤实现 ----------

// assertShelfCount 断言书架总数与首本书名。
func (w *world) assertShelfCount(want int) error {
	var res struct {
		Total int `json:"total"`
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	code, body, err := w.getJSON("/api/books", &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books 状态码 = %d，body=%s", code, body)
	}
	if res.Total != want {
		return fmt.Errorf("书架总数 = %d，期望 %d", res.Total, want)
	}
	if len(res.Items) == 0 {
		return fmt.Errorf("书架 items 为空")
	}
	if res.Items[0].Title != synthBookTitle {
		return fmt.Errorf("书架书名 = %q，期望合成书名 %q", res.Items[0].Title, synthBookTitle)
	}
	return nil
}

// assertChapterCount 断言书籍详情的章节总数。
func (w *world) assertChapterCount(want int) error {
	if w.lastBookID == 0 {
		return fmt.Errorf("尚未导入书籍")
	}
	var detail struct {
		Volumes []struct {
			Chapters []json.RawMessage `json:"chapters"`
		} `json:"volumes"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books/{id} 状态码 = %d，body=%s", code, body)
	}
	n := 0
	for _, v := range detail.Volumes {
		n += len(v.Chapters)
	}
	if n != want {
		return fmt.Errorf("书籍详情章节数 = %d，期望 %d", n, want)
	}
	return nil
}

// assertFirstChapterBody 断言首章正文含合成正文标记。
func (w *world) assertFirstChapterBody() error {
	if w.lastBookID == 0 || w.firstChapter == 0 {
		return fmt.Errorf("尚未导入书籍或未取到首章 id")
	}
	var ch struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d/chapters/%d", w.lastBookID, w.firstChapter), &ch)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 章节正文状态码 = %d，body=%s", code, body)
	}
	if !strings.Contains(ch.Content, synthBodyMarker) {
		return fmt.Errorf("章节 %q 正文未包含合成正文标记 %q，正文前 40 字 = %q",
			ch.Title, synthBodyMarker, truncate(ch.Content, 40))
	}
	return nil
}

// requestTooShortQuery 用 n 个字的短关键词请求搜索，期望被拒绝。
func (w *world) requestTooShortQuery(n int) error {
	kw := []rune(searchHitKeyword)
	if n <= 0 || n > len(kw) {
		return fmt.Errorf("短关键词长度 %d 超出合成关键词长度 %d", n, len(kw))
	}
	code, body, err := w.doJSON(http.MethodGet, "/api/search?q="+string(kw[:n]), "")
	if err != nil {
		return err
	}
	if code == http.StatusOK {
		return fmt.Errorf("关键词长度 %d 的请求返回 200，期望被拒绝（body=%s）", n, body)
	}
	return nil
}

// importEmptyDir 导入一个只有非 txt 文件的目录，任务应标记失败。
func (w *world) importEmptyDir() error {
	if err := w.ensureDB(); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(w.root, "empty-dir-*")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("no txt here"), 0o644); err != nil {
		return err
	}
	return w.importDir(dir)
}

// assertJobFailureMessage 断言失败任务能查到非空原因。
func (w *world) assertJobFailureMessage() error {
	var detail struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/import/jobs/%d", w.lastJobID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 任务详情状态码 = %d，body=%s", code, body)
	}
	if strings.TrimSpace(detail.Message) == "" {
		return fmt.Errorf("任务 %d 状态 %q 但 message 为空，无法查失败原因", w.lastJobID, detail.Status)
	}
	return nil
}

// importMissingDir 请求导入一个不存在的目录，期望被拒绝。
func (w *world) importMissingDir() error {
	if err := w.ensureDB(); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"dir": filepath.Join(w.root, "no-such-dir")})
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPost, "/api/import", string(payload))
	if err != nil {
		return err
	}
	if code == http.StatusAccepted {
		return fmt.Errorf("导入不存在的目录返回 202，期望被拒绝（body=%s）", body)
	}
	return nil
}

// assertParamError 断言状态码与错误码都属于参数错误。
func (w *world) assertParamError(want int) error {
	if w.lastCode != want {
		return fmt.Errorf("状态码 = %d，期望 %d，body=%s", w.lastCode, want, truncate(string(w.lastBody), 200))
	}
	var eb struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.lastBody, &eb); err != nil {
		return fmt.Errorf("错误响应不是预期 JSON: %w (body=%s)", err, w.lastBody)
	}
	switch eb.Error.Code {
	case "invalid_param", "query_too_short":
		return nil
	}
	return fmt.Errorf("错误码 = %q，期望参数类错误码（invalid_param / query_too_short）", eb.Error.Code)
}

// search 执行一次检索并记录结果。
func (w *world) search(kw string) error {
	var res struct {
		Books struct {
			Total int `json:"total"`
		} `json:"books"`
		Chapters struct {
			Total int `json:"total"`
			Items []struct {
				BookTitle  string `json:"bookTitle"`
				ChapterIdx int    `json:"chapterIdx"`
			} `json:"items"`
		} `json:"chapters"`
	}
	code, body, err := w.getJSON("/api/search?q="+kw+"&type=all", &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/search 状态码 = %d，body=%s", code, body)
	}
	w.hitBooks, w.hitChapters = res.Books.Total, res.Chapters.Total
	w.hitTitle, w.hitIdx = "", -1
	if len(res.Chapters.Items) > 0 {
		w.hitTitle = res.Chapters.Items[0].BookTitle
		w.hitIdx = res.Chapters.Items[0].ChapterIdx
	}
	return nil
}

// atoiSafe 把数字字符串转 int；非数字返回 0（feature 里只会写数字字面量）。
func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// truncate 截断长字符串，避免断言消息刷屏。
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
