package acceptance

// P1 增量（ZIP 批量 / 合集拆分 / 追更）的验收步骤与合成 fixture。
// 与既有场景一样：全部内容合成，走真实 api + SQLite，不 mock。

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

const (
	zipPackName       = "合成合订包.zip"
	collectionName    = "合成合集"
	updateBookName    = "合成追更"
	collectionFirst   = 20
	collectionSecond  = 12
	updateFirstCount  = 10
	updateSecondCount = 15
)

// registerP1Steps 注册 P1 增量的验收步骤（分组注册，避免单函数过长）。
func registerP1Steps(sc *godog.ScenarioContext, w *world) {
	registerZipUploadSteps(sc, w)
	registerSplitSteps(sc, w)
	registerUpdateSteps(sc, w)
}

// registerZipUploadSteps 注册 ZIP 上传相关步骤。
func registerZipUploadSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^我上传一个包含合成 txt 与 pdf 的 zip 包$", func(ctx context.Context) error {
		return w.uploadSyntheticZip()
	})
	sc.Step("^上传结果应有 (\\d+) 个条目被接受$", func(ctx context.Context, n int) error {
		if len(w.uploadAccepted) != n {
			return fmt.Errorf("accepted = %d 条，期望 %d（%+v）", len(w.uploadAccepted), n, w.uploadAccepted)
		}
		return nil
	})
	sc.Step(`^被接受的条目名应以 "(.+)" 开头$`, func(ctx context.Context, prefix string) error {
		for _, a := range w.uploadAccepted {
			if !strings.HasPrefix(a.Name, prefix) {
				return fmt.Errorf("accepted 条目名 = %q，期望以 %q 开头", a.Name, prefix)
			}
		}
		return nil
	})
	sc.Step(`^上传结果应有 (\d+) 个条目被拒绝并带错误码 "(.+)"$`, func(ctx context.Context, n int, code string) error {
		if len(w.uploadRejected) != n {
			return fmt.Errorf("rejected = %d 条，期望 %d（%+v）", len(w.uploadRejected), n, w.uploadRejected)
		}
		for _, r := range w.uploadRejected {
			if r.Code != code {
				return fmt.Errorf("rejected 错误码 = %q，期望 %q", r.Code, code)
			}
		}
		return nil
	})
}

// registerSplitSteps 注册合集拆分相关步骤。
func registerSplitSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^我导入一个含两段章号重置的合成合集$", func(ctx context.Context) error {
		return w.importSyntheticCollection()
	})
	sc.Step("^书架里应有 (\\d+) 本书$", func(ctx context.Context, n int) error {
		counts, err := w.bookChapterCounts()
		if err != nil {
			return err
		}
		if len(counts) != n {
			return fmt.Errorf("书架 = %d 本，期望 %d（章数 %v）", len(counts), n, counts)
		}
		return nil
	})
	sc.Step("^两本书的章数应分别为 (\\d+) 与 (\\d+)$", func(ctx context.Context, first, second int) error {
		counts, err := w.bookChapterCounts()
		if err != nil {
			return err
		}
		if len(counts) != 2 {
			return fmt.Errorf("书架 = %d 本，期望 2（章数 %v）", len(counts), counts)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(counts)))
		if counts[0] != first || counts[1] != second {
			return fmt.Errorf("两本书章数 = %v，期望 %d 与 %d", counts, first, second)
		}
		return nil
	})
}

// registerUpdateSteps 注册追更相关步骤。
func registerUpdateSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^我把合成书稿的 (\\d+) 章版本导入图书馆$", func(ctx context.Context, n int) error {
		return w.importUpdateVersion(n, false)
	})
	sc.Step("^我以追更方式导入同一本书的 (\\d+) 章版本$", func(ctx context.Context, n int) error {
		return w.importUpdateVersion(n, true)
	})
	sc.Step("^这本书的章节数应为 (\\d+)$", func(ctx context.Context, n int) error {
		return w.assertChapterCount(n)
	})
}

// uploadSyntheticZip 上传一个含合成 txt 与 pdf 的 zip 包。
func (w *world) uploadSyntheticZip() error {
	packed, err := synthZipBytes()
	if err != nil {
		return err
	}
	return w.uploadFiles([]synthFile{{name: zipPackName, data: packed}})
}

// synthZipBytes 合成 zip：一个 txt 书稿 + 一个 pdf。
func synthZipBytes() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entries := [][2]string{
		{"合成zip书.txt", "书名：合成zip书\n作者：某合成\n\n第一章 起\n" + synthChapterOneBody + "\n"},
		{"说明.pdf", "not a book"},
	}
	for _, e := range entries {
		file, err := zw.Create(e[0])
		if err != nil {
			return nil, err
		}
		if _, err := file.Write([]byte(e[1])); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// importSyntheticCollection 导入一个两段章号重置的合成合集。
func (w *world) importSyntheticCollection() error {
	if err := w.ensureDB(); err != nil {
		return err
	}
	if err := os.MkdirAll(w.libsr, 0o755); err != nil {
		return err
	}
	text := "书名：" + collectionName + "\n作者：某合集\n\n" +
		synthChapters(collectionFirst, "甲") + "书名：合成合集续\n作者：某续\n\n" +
		synthChapters(collectionSecond, "乙")
	path := filepath.Join(w.libsr, collectionName+".txt")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return err
	}
	return w.importDir(w.libsr)
}

// synthChapters 合成 n 章「第N章 标签」正文。
func synthChapters(n int, tag string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("第" + strconv.Itoa(i) + "章 " + tag + "段\n")
		b.WriteString("这是" + tag + "段的合成正文，与真实作品无关。\n\n")
	}
	return b.String()
}

// importUpdateVersion 导入合成的追更书稿：update=false 首次导入，true 走追更。
func (w *world) importUpdateVersion(n int, update bool) error {
	if err := w.ensureDB(); err != nil {
		return err
	}
	if err := os.MkdirAll(w.libsr, 0o755); err != nil {
		return err
	}
	text := "书名：" + updateBookName + "\n作者：某追\n\n" + synthChapters(n, "旧")
	path := filepath.Join(w.libsr, updateBookName+".txt")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"dir": w.libsr, "recursive": false, "update": update})
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
		return fmt.Errorf("解析 jobId 失败: %w", err)
	}
	w.lastJobID = resp.JobID
	status, err := w.waitJob(resp.JobID, 15*time.Second)
	if err != nil {
		return err
	}
	w.lastJobStatus = status
	return w.captureBook()
}

// bookChapterCounts 返回书架上每本书的章节数。
func (w *world) bookChapterCounts() ([]int, error) {
	var list struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	code, body, err := w.getJSON("/api/books", &list)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("GET /api/books 状态码 = %d，body=%s", code, body)
	}
	counts := make([]int, 0, len(list.Items))
	for _, item := range list.Items {
		var detail struct {
			Volumes []struct {
				Chapters []json.RawMessage `json:"chapters"`
			} `json:"volumes"`
		}
		code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", item.ID), &detail)
		if err != nil {
			return nil, err
		}
		if code != http.StatusOK {
			return nil, fmt.Errorf("GET /api/books/%d 状态码 = %d，body=%s", item.ID, code, body)
		}
		n := 0
		for _, v := range detail.Volumes {
			n += len(v.Chapters)
		}
		counts = append(counts, n)
	}
	return counts, nil
}
