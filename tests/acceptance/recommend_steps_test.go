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

// 发现页推荐（设计 §6）的验收步骤。
//
// 全部走真实链路：/api/events 上报 → 推荐库聚合 → /api/recommend 召回 + 打分。
// 断言只看「用户能看到什么」（卡片、理由、两次是否不同），不假设任何内部排序。

// recCard 是 /api/recommend 返回的一张推荐卡（只取断言需要的字段）。
type recCard struct {
	BookID int64   `json:"bookId"`
	Title  string  `json:"title"`
	Reason string  `json:"reason"`
	Score  float64 `json:"score"`
}

// recommendPayload 是 /api/recommend 的响应体。
type recommendPayload struct {
	Items []recCard `json:"items"`
}

const (
	secondBookTitle = "合成样例书二"
	secondAuthor    = "合成作者二"
)

// registerRecommendSteps 注册推荐相关的步骤定义。
func registerRecommendSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我把两本合成书稿导入图书馆$`, func(ctx context.Context) error {
		return w.importTwoBooks()
	})
	sc.Step(`^我把这本书的分类设为 "([^"]+)"$`, func(ctx context.Context, cat string) error {
		w.recCategory = cat
		return w.setBookCategory(w.lastBookID, cat)
	})
	sc.Step(`^我把另一本书的分类设为 "([^"]+)"$`, func(ctx context.Context, cat string) error {
		return w.setBookCategory(w.recOtherID, cat)
	})
	sc.Step(`^我上报一条 (click|read) 事件$`, func(ctx context.Context, kind string) error {
		return w.postEvent(kind, w.lastBookID)
	})
	sc.Step(`^我把这本书读到最后一页$`, func(ctx context.Context) error {
		return w.markFinished(w.lastBookID)
	})
	sc.Step(`^推荐结果应当包含这本书$`, func(ctx context.Context) error {
		return w.assertRecommendContains(w.lastBookID, true)
	})
	sc.Step(`^我连续请求两次推荐$`, func(ctx context.Context) error {
		return w.requestRecommendTwice()
	})
	sc.Step(`^推荐结果应当包含另一本书$`, func(ctx context.Context) error {
		return w.assertRecommendContains(w.recOtherID, true)
	})
	sc.Step(`^推荐结果不应包含这本书$`, func(ctx context.Context) error {
		return w.assertRecommendContains(w.lastBookID, false)
	})
	sc.Step(`^推荐理由应当提到 "([^"]+)"$`, func(ctx context.Context, want string) error {
		return w.assertRecommendReason(w.recOtherID, want)
	})
	sc.Step(`^推荐结果应当非空$`, func(ctx context.Context) error {
		return w.assertRecommendNotEmpty()
	})
	sc.Step(`^每张推荐卡都带理由$`, func(ctx context.Context) error {
		return w.assertEveryCardHasReason()
	})
	sc.Step(`^两次推荐的分数不完全相同$`, func(ctx context.Context) error {
		return w.assertScoresDiffer()
	})
}

// importTwoBooks 写两份合成书稿并一起导入。
//
// 推荐场景需要多本候选：只有一本书时「排序」「随机」「已读不推」都退化成
// 恒真断言，测不出任何东西。两本的第二本内容必须不同，否则会被内容指纹判重。
func (w *world) importTwoBooks() error {
	if err := w.ensureDB(); err != nil {
		return err
	}
	if err := w.writeSynthBook(); err != nil {
		return fmt.Errorf("写入第一本合成书稿失败: %w", err)
	}
	if err := w.writeSecondBook(); err != nil {
		return fmt.Errorf("写入第二本合成书稿失败: %w", err)
	}
	if err := w.importDir(w.libsr); err != nil {
		return err
	}
	return w.captureTwoBooks()
}

// writeSecondBook 在合成书稿目录写第二本（书名/作者/正文都与第一本不同）。
func (w *world) writeSecondBook() error {
	content := strings.Join([]string{
		"书名：" + secondBookTitle,
		"作者：" + secondAuthor,
		"",
		"第一章 第二本开篇",
		"这是第二本合成正文，风从山谷吹过。",
		"",
		"第二章 第二本收束",
		"这是第二本合成正文，云散了，路还在。",
		"",
	}, "\n")
	return os.WriteFile(filepath.Join(w.libsr, secondBookTitle+".txt"), []byte(content), 0o644)
}

// captureTwoBooks 记录两本书的 id（书架按最近更新倒序，第一本 = 最后导入的那本）。
func (w *world) captureTwoBooks() error {
	var list struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	code, data, err := w.getJSON("/api/books", &list)
	if err != nil {
		return err
	}
	if code != http.StatusOK || len(list.Items) < 2 {
		return fmt.Errorf("导入两本后书架应有 2 本书（code=%d，body=%s）", code, data)
	}
	w.lastBookID, w.recOtherID = list.Items[0].ID, list.Items[1].ID
	return nil
}

// setBookCategory 通过 PATCH 给书打分类（兴趣召回按分类查书）。
func (w *world) setBookCategory(bookID int64, category string) error {
	body, err := json.Marshal(map[string]string{"categoryL1": category})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPatch, fmt.Sprintf("/api/books/%d", bookID), string(body))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("PATCH /api/books/%d 状态码 = %d，期望 200，body=%s", bookID, code, data)
	}
	return nil
}

// postEvent 上报一条埋点（read 事件带 600 秒停留，够到兴趣增量的第二档）。
func (w *world) postEvent(kind string, bookID int64) error {
	value := 0
	if kind == "read" {
		value = 600
	}
	payload := map[string]any{"events": []map[string]any{{
		"kind": kind, "bookId": bookID, "categoryL1": w.recCategory, "value": value,
	}}}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPost, "/api/events", string(body))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("POST /api/events 状态码 = %d，期望 200，body=%s", code, data)
	}
	return nil
}

// markFinished 把一本书的进度写到 100%（读完 → 推荐不再包含它）。
//
// 为什么不用 read 事件当「已读」：读了几分钟就退出是常态，真实数据里几乎每本书
// 都被 read 覆盖过；只有阅读器上报的进度能区分「翻了几页」与「读完了」。
func (w *world) markFinished(bookID int64) error {
	var detail struct {
		Volumes []struct {
			Chapters []struct {
				ID int64 `json:"id"`
			} `json:"chapters"`
		} `json:"volumes"`
	}
	code, data, err := w.getJSON(fmt.Sprintf("/api/books/%d", bookID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books/%d 状态码 = %d，body=%s", bookID, code, data)
	}
	var chapterID int64
	for _, v := range detail.Volumes {
		for _, ch := range v.Chapters {
			chapterID = ch.ID
		}
	}
	if chapterID <= 0 {
		return fmt.Errorf("书 %d 没有章节，无法记录读完", bookID)
	}
	body, err := json.Marshal(map[string]any{
		"bookId": bookID, "chapterId": chapterID, "percent": 1,
	})
	if err != nil {
		return err
	}
	code, data, err = w.doJSON(http.MethodPut, "/api/reading/progress", string(body))
	if err != nil {
		return err
	}
	if code != http.StatusNoContent {
		return fmt.Errorf("PUT /api/reading/progress 状态码 = %d，期望 204，body=%s", code, data)
	}
	return nil
}

// fetchRecommend 请求一次推荐，并把结果记到 world 上。
func (w *world) fetchRecommend() ([]recCard, error) {
	var payload recommendPayload
	code, data, err := w.getJSON("/api/recommend?limit=20", &payload)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("GET /api/recommend 状态码 = %d，期望 200，body=%s", code, data)
	}
	if payload.Items == nil {
		return nil, fmt.Errorf("推荐响应缺 items 字段（应为 [] 而不是 null）")
	}
	w.recItems = payload.Items
	return payload.Items, nil
}

// requestRecommendTwice 连续请求两次并记下分数，供「两次结果不完全相同」断言。
func (w *world) requestRecommendTwice() error {
	first, err := w.fetchRecommend()
	if err != nil {
		return err
	}
	w.recScoreA = scoresOf(first)
	second, err := w.fetchRecommend()
	if err != nil {
		return err
	}
	w.recScoreB = scoresOf(second)
	return nil
}

// assertRecommendContains 断言推荐里包含 / 不包含某本书。
func (w *world) assertRecommendContains(bookID int64, want bool) error {
	items, err := w.fetchRecommend()
	if err != nil {
		return err
	}
	if got := findCard(items, bookID); got != want {
		return fmt.Errorf("书 %d 是否在推荐里 = %v，期望 %v（items=%+v）", bookID, got, want, items)
	}
	return nil
}

// assertRecommendReason 断言某本书的推荐理由包含指定片段。
func (w *world) assertRecommendReason(bookID int64, want string) error {
	items, err := w.fetchRecommend()
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.BookID == bookID {
			if !strings.Contains(it.Reason, want) {
				return fmt.Errorf("书 %d 的理由 = %q，期望包含 %q", bookID, it.Reason, want)
			}
			return nil
		}
	}
	return fmt.Errorf("推荐里没有书 %d，无法断言理由（items=%+v）", bookID, items)
}

// assertRecommendNotEmpty 断言冷启动下也能给出推荐。
func (w *world) assertRecommendNotEmpty() error {
	items, err := w.fetchRecommend()
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("冷启动推荐为空")
	}
	return nil
}

// assertEveryCardHasReason 断言每张卡片都带可解释的理由。
func (w *world) assertEveryCardHasReason() error {
	items, err := w.fetchRecommend()
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Reason == "" {
			return fmt.Errorf("推荐卡 %d（%s）缺少理由", it.BookID, it.Title)
		}
	}
	return nil
}

// assertScoresDiffer 断言两次请求的分数不完全相同（随机扰动生效）。
func (w *world) assertScoresDiffer() error {
	if len(w.recScoreA) == 0 || len(w.recScoreA) != len(w.recScoreB) {
		return fmt.Errorf("两次推荐的条数不一致: %d / %d", len(w.recScoreA), len(w.recScoreB))
	}
	for i := range w.recScoreA {
		if w.recScoreA[i] != w.recScoreB[i] {
			return nil
		}
	}
	return fmt.Errorf("两次推荐分数完全相同（随机扰动没生效）: %v", w.recScoreA)
}

// scoresOf 取一批卡片的分数。
func scoresOf(items []recCard) []float64 {
	out := make([]float64, len(items))
	for i, it := range items {
		out[i] = it.Score
	}
	return out
}

// findCard 判断推荐里有没有这本书。
func findCard(items []recCard, bookID int64) bool {
	for _, it := range items {
		if it.BookID == bookID {
			return true
		}
	}
	return false
}
