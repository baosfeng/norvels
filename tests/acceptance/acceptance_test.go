// Package acceptance 是 norvels 的 Gherkin 验收测试（godog）。
//
// 设计要点：
//   - 全部 fixture 都是**合成文本**（见下方 synth* 常量与 tests/features/*.feature），不读任何真实语料；
//   - 走真实链路：临时目录 SQLite + internal/api 的完整 handler + httptest，
//     不 mock store/importer，避免验收测试与实现各说各话；
//   - 每个场景独立环境（新的临时目录 + 新库），场景之间无耦合。
package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"norvels/internal/api"
	"norvels/internal/store"
)

// 合成书稿的内容：所有断言都基于这些常量，便于一眼看出数据全是造出来的。
const (
	synthBookTitle       = "合成样例书"
	synthAuthor          = "合成作者"
	synthChapterOneTitle = "第一章 灯塔初现"
	synthChapterTwoTitle = "第二章 雾散时分"
	// synthChapterOneBody 含检索关键词「灯塔」。
	synthChapterOneBody  = "这是合成正文第一段，灯塔的光扫过海面。"
	synthChapterTwoBody  = "这是合成正文第二段，雾散了，天亮了。"
	synthBodyMarker      = "这是合成正文"
	searchHitKeyword     = "灯塔"
	searchMissKeyword    = "不存在的词条xyz"
	importStatusFailed   = "failed"
	importStatusComplete = "completed"
)

// world 是一个场景的执行状态。
type world struct {
	root  string // 场景临时根目录
	libsr string // 合成书稿目录
	db    *store.Store
	srv   *httptest.Server

	// 最近一次导入
	lastJobID     int64
	lastJobStatus string
	lastBookID    int64
	firstChapter  int64

	// 最近一次检索
	hitBooks    int
	hitChapters int
	hitTitle    string
	hitIdx      int

	// 最近一次裸请求（用于断言错误响应）
	lastCode int
	lastBody []byte

	// 最近一次上传（POST /api/import/upload）
	uploadAccepted []uploadFileResult
	uploadRejected []uploadFileResult
	coverType      string // 最近一次封面响应的 Content-Type

	// 书签（P1 阅读体验）
	lastBookmarkID int64

	// 导入侧 P1-B（词表 / 预检 / 重试）
	p1bReport     p1bReport
	p1bRetryID    int64
	p1bRunningID  int64
	p1bJobsBefore int
}

// reset 关闭上一个场景留下的资源。
func (w *world) reset() {
	if w.srv != nil {
		w.srv.Close()
		w.srv = nil
	}
	if w.db != nil {
		_ = w.db.Close()
		w.db = nil
	}
	if w.root != "" {
		_ = os.RemoveAll(w.root)
		w.root = ""
	}
	*w = world{}
}

// ensureDB 打开一个空库并起 httptest 服务（每个场景只做一次）。
func (w *world) ensureDB() error {
	if w.srv != nil {
		return nil
	}
	root, err := os.MkdirTemp("", "norvels-acceptance-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	w.root = root
	w.libsr = filepath.Join(root, "library-src")
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(root, "library.db"))
	if err != nil {
		return fmt.Errorf("store.Open: %w", err)
	}
	if err := st.Migrate(ctx); err != nil {
		_ = st.Close()
		return fmt.Errorf("store.Migrate: %w", err)
	}
	w.db = st
	w.srv = httptest.NewServer(api.New(st, api.Config{
		Version:          "acceptance",
		Commit:           "gherkin",
		BuiltAt:          "2026-01-01T00:00:00Z",
		DisableAccessLog: true,
		// 上传上限注小值：验收测试用少量字节就能触发 413/400，跑得快；
		// 合成 epub 约 1 KiB，仍在单文件 8 KiB 上限内。
		MaxUploadFileBytes:  8 << 10,
		MaxUploadTotalBytes: 32 << 10,
		MaxUploadFiles:      20,
		UploadDir:           filepath.Join(root, "uploads"),
	}))
	return nil
}

// doJSON 发送请求；body 非空时按 JSON 提交。
func (w *world) doJSON(method, path, body string) (int, []byte, error) {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, w.srv.URL+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := w.srv.Client().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	w.lastCode, w.lastBody = resp.StatusCode, data
	return resp.StatusCode, data, err
}

// getJSON 发 GET 并把 200 响应反序列化到 out。
func (w *world) getJSON(path string, out any) (int, []byte, error) {
	code, data, err := w.doJSON(http.MethodGet, path, "")
	if err != nil {
		return code, data, err
	}
	if out != nil && code == http.StatusOK {
		if jerr := json.Unmarshal(data, out); jerr != nil {
			return code, data, fmt.Errorf("解析 %s 响应失败: %w (body=%s)", path, jerr, data)
		}
	}
	return code, data, nil
}

// writeSynthBook 在临时目录写一份合成书稿（合成书名 + 两章）。
func (w *world) writeSynthBook() error {
	if err := os.MkdirAll(w.libsr, 0o755); err != nil {
		return err
	}
	content := strings.Join([]string{
		"书名：" + synthBookTitle,
		"作者：" + synthAuthor,
		"",
		synthChapterOneTitle,
		synthChapterOneBody,
		"",
		synthChapterTwoTitle,
		synthChapterTwoBody,
		"",
	}, "\n")
	// 文件名优先作为书名（parser 的既定规则），所以文件名同样用合成书名。
	return os.WriteFile(filepath.Join(w.libsr, synthBookTitle+".txt"), []byte(content), 0o644)
}

// importDir 发起导入并等待任务离开 running，随后抓取书 id 与首章 id。
func (w *world) importDir(dir string) error {
	payload, err := json.Marshal(map[string]any{"dir": dir, "recursive": false})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPost, "/api/import", string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusAccepted {
		return fmt.Errorf("POST /api/import 状态码 = %d，期望 202，body=%s", code, data)
	}
	var resp struct {
		JobID int64 `json:"jobId"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("解析 jobId 失败: %w (body=%s)", err, data)
	}
	w.lastJobID = resp.JobID
	status, err := w.waitJob(resp.JobID, 15*time.Second)
	if err != nil {
		return err
	}
	w.lastJobStatus = status
	if status == importStatusComplete || status == "completed_with_errors" {
		if err := w.captureBook(); err != nil {
			return err
		}
	}
	return nil
}

// waitJob 轮询任务直到状态不再为 running。
func (w *world) waitJob(id int64, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		code, data, err := w.doJSON(http.MethodGet, fmt.Sprintf("/api/import/jobs/%d", id), "")
		if err != nil {
			return "", err
		}
		if code == http.StatusOK {
			var detail struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(data, &detail); err != nil {
				return "", err
			}
			if detail.Status != "running" {
				return detail.Status, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("任务 %d 在 %s 内未离开 running", id, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// captureBook 从书架与书籍详情里取出书 id 与首章 id，供后续阅读步骤使用。
func (w *world) captureBook() error {
	var list struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	code, data, err := w.getJSON("/api/books", &list)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books 状态码 = %d，body=%s", code, data)
	}
	if len(list.Items) == 0 {
		return fmt.Errorf("导入成功但书架为空")
	}
	w.lastBookID = list.Items[0].ID

	var detail struct {
		Volumes []struct {
			Chapters []struct {
				ID  int64 `json:"id"`
				Idx int   `json:"idx"`
			} `json:"chapters"`
		} `json:"volumes"`
	}
	code, data, err = w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books/%d 状态码 = %d，body=%s", w.lastBookID, code, data)
	}
	for _, v := range detail.Volumes {
		for _, ch := range v.Chapters {
			w.firstChapter = ch.ID
			return nil
		}
	}
	return fmt.Errorf("书籍 %d 没有章节", w.lastBookID)
}

func TestFeatures(t *testing.T) {
	w := &world{}
	suite := godog.TestSuite{
		Name: "norvels acceptance",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
				w.reset()
				return ctx, nil
			})
			sc.After(func(ctx context.Context, s *godog.Scenario, err error) (context.Context, error) {
				w.reset()
				return ctx, nil
			})
			registerSteps(sc, w)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../features"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("godog 验收测试未全部通过")
	}
}
