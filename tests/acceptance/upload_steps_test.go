package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// uploadFileResult 对应 POST /api/import/upload 的 accepted/rejected 条目。
type uploadFileResult struct {
	Name    string `json:"name"`
	Format  string `json:"format"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// uploadResponse 是上传接口的 202 响应。
type uploadResponse struct {
	JobID    int64              `json:"jobId"`
	Accepted []uploadFileResult `json:"accepted"`
	Rejected []uploadFileResult `json:"rejected"`
}

// bookDetailDTO 只取验收需要的字段。
type bookDetailDTO struct {
	Title    string   `json:"title"`
	Tags     []string `json:"tags"`
	HasCover bool     `json:"hasCover"`
}

// synthFile 是一个待上传的合成文件。
type synthFile struct {
	name string
	data []byte
}

// registerUploadSteps 注册上传 / EPUB / 元数据编辑相关步骤。
func registerUploadSteps(sc *godog.ScenarioContext, w *world) {
	registerUploadActionSteps(sc, w)
	registerUploadAssertSteps(sc, w)
	registerMetaActionSteps(sc, w)
	registerMetaAssertSteps(sc, w)
}

// registerUploadActionSteps 注册「上传某类文件」的步骤。
func registerUploadActionSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我上传一个 txt 文件$`, func(ctx context.Context) error {
		return w.uploadFiles([]synthFile{{name: synthUploadTitle + ".txt", data: synthTXT()}})
	})
	sc.Step(`^我上传一个合成 epub 文件$`, func(ctx context.Context) error {
		return w.uploadFiles([]synthFile{{name: "示例电子书.epub", data: synthEPUB(false)}})
	})
	sc.Step(`^我上传一个 txt 和一个 pdf 文件$`, func(ctx context.Context) error {
		return w.uploadFiles([]synthFile{
			{name: synthUploadTitle + ".txt", data: synthTXT()},
			{name: "不支持的样例.pdf", data: []byte("%PDF-1.4 synthetic")},
		})
	})
	sc.Step(`^我上传一个 pdf 文件$`, func(ctx context.Context) error {
		return w.uploadFiles([]synthFile{{name: "不支持的样例.pdf", data: []byte("%PDF-1.4 synthetic")}})
	})
	sc.Step(`^我上传一个加密的 epub 文件$`, func(ctx context.Context) error {
		return w.uploadFiles([]synthFile{{name: "加密样例.epub", data: synthEPUB(true)}})
	})
	sc.Step(`^我上传一个超过上限的 txt 文件$`, func(ctx context.Context) error {
		big := append(synthTXT(), bytes.Repeat([]byte("字"), oversizePadding)...)
		return w.uploadFiles([]synthFile{{name: "超大样例.txt", data: big}})
	})
}

// registerUploadAssertSteps 注册上传结果的断言步骤。
func registerUploadAssertSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^上传应当被接受$`, func(ctx context.Context) error {
		if w.lastCode != http.StatusAccepted {
			return fmt.Errorf("上传状态码 = %d，期望 202，body=%s", w.lastCode, truncate(string(w.lastBody), 300))
		}
		return nil
	})
	sc.Step(`^被拒绝的文件应当提示 "(.+)"$`, func(ctx context.Context, want string) error {
		for _, r := range w.uploadRejected {
			if strings.Contains(r.Message, want) {
				return nil
			}
		}
		return fmt.Errorf("没有一条 rejected 的 message 含 %q：%+v", want, w.uploadRejected)
	})
	sc.Step(`^接口应当返回 (\d+)$`, func(ctx context.Context, want string) error {
		if w.lastCode != atoiSafe(want) {
			return fmt.Errorf("状态码 = %d，期望 %s，body=%s", w.lastCode, want, truncate(string(w.lastBody), 300))
		}
		return nil
	})
	sc.Step(`^错误码应当是 "(.+)"$`, func(ctx context.Context, want string) error {
		code, err := errorCodeOf(w.lastBody)
		if err != nil {
			return err
		}
		if code != want {
			return fmt.Errorf("错误码 = %q，期望 %q，body=%s", code, want, truncate(string(w.lastBody), 300))
		}
		return nil
	})
	sc.Step(`^错误信息应当包含 "(.+)"$`, func(ctx context.Context, want string) error {
		if !strings.Contains(string(w.lastBody), want) {
			return fmt.Errorf("错误信息未包含 %q，body=%s", want, truncate(string(w.lastBody), 300))
		}
		return nil
	})
	sc.Step(`^书架里应当有 (\d+) 本书且书名为 "(.+)"$`, func(ctx context.Context, n, title string) error {
		return w.assertShelfTitle(atoiSafe(n), title)
	})
}

// registerMetaActionSteps 注册元数据编辑动作。
func registerMetaActionSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我把这本书的书名改为 "(.+)"$`, func(ctx context.Context, title string) error {
		return w.patchBook(map[string]any{"title": title})
	})
	sc.Step(`^我给这本书加上标签 "(.+)"$`, func(ctx context.Context, tag string) error {
		return w.patchBook(map[string]any{"tags": []string{tag}})
	})
}

// registerMetaAssertSteps 注册封面与元数据断言。
func registerMetaAssertSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^这本书应当有封面$`, func(ctx context.Context) error {
		d, err := w.getBookDetail()
		if err != nil {
			return err
		}
		if !d.HasCover {
			return fmt.Errorf("书 %d 的 hasCover = false，期望 true", w.lastBookID)
		}
		return nil
	})
	sc.Step(`^这本书的分类应当包含 "(.+)"$`, func(ctx context.Context, want string) error {
		d, err := w.getBookDetail()
		if err != nil {
			return err
		}
		for _, t := range d.Tags {
			if t == want {
				return nil
			}
		}
		return fmt.Errorf("分类 = %v，期望包含 %q", d.Tags, want)
	})
	sc.Step(`^书籍详情里的书名应为 "(.+)"$`, func(ctx context.Context, want string) error {
		d, err := w.getBookDetail()
		if err != nil {
			return err
		}
		if d.Title != want {
			return fmt.Errorf("书名 = %q，期望 %q", d.Title, want)
		}
		return nil
	})
	sc.Step(`^这本书的封面接口应当返回 (\d+)$`, func(ctx context.Context, want string) error {
		return w.requestCover(atoiSafe(want))
	})
	sc.Step(`^封面响应的内容类型应当是 "(.+)"$`, func(ctx context.Context, want string) error {
		if !strings.HasPrefix(w.coverType, want) {
			return fmt.Errorf("封面 Content-Type = %q，期望前缀 %q", w.coverType, want)
		}
		return nil
	})
}

// ---------- world 上的上传/元数据实现 ----------

// uploadFiles 以 multipart 上传一批合成文件，并等待导入任务结束。
func (w *world) uploadFiles(files []synthFile) error {
	if err := w.ensureDB(); err != nil {
		return err
	}
	code, body, err := w.postUpload(files)
	if err != nil {
		return err
	}
	w.lastCode, w.lastBody = code, body
	w.uploadAccepted, w.uploadRejected = nil, nil
	if code != http.StatusAccepted {
		return nil
	}
	r, err := parseUploadResponse(body)
	if err != nil {
		return err
	}
	w.uploadAccepted, w.uploadRejected = r.Accepted, r.Rejected
	w.lastJobID = r.JobID
	return w.finishUpload(r.JobID, code, body)
}

// postUpload 发一次 multipart 请求，返回状态码与响应体。
func (w *world) postUpload(files []synthFile) (int, []byte, error) {
	body, contentType, err := buildMultipart(files)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, w.srv.URL+"/api/import/upload", body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := w.srv.Client().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, data, nil
}

// parseUploadResponse 解析 202 的 accepted/rejected。
func parseUploadResponse(data []byte) (uploadResponse, error) {
	var r uploadResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("解析上传响应失败: %w (body=%s)", err, data)
	}
	return r, nil
}

// finishUpload 等待任务结束并抓书；最后恢复上传响应，供「上传应当被接受」断言。
func (w *world) finishUpload(jobID int64, code int, body []byte) error {
	status, err := w.waitJob(jobID, 20*time.Second)
	if err != nil {
		return err
	}
	w.lastJobStatus = status
	if status != importStatusComplete && status != "completed_with_errors" {
		return nil
	}
	if err := w.captureBook(); err != nil {
		return err
	}
	w.lastCode, w.lastBody = code, body
	return nil
}

// buildMultipart 把合成文件拼成 multipart 请求体。
func buildMultipart(files []synthFile) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		fw, err := mw.CreateFormFile("files", f.name)
		if err != nil {
			return nil, "", err
		}
		if _, err := fw.Write(f.data); err != nil {
			return nil, "", err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return &buf, mw.FormDataContentType(), nil
}

// patchBook 对最近导入的书发 PATCH，要求 200。
func (w *world) patchBook(payload map[string]any) error {
	if w.lastBookID == 0 {
		return fmt.Errorf("尚未导入书籍")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPatch, fmt.Sprintf("/api/books/%d", w.lastBookID), string(body))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("PATCH /api/books/%d 状态码 = %d，body=%s", w.lastBookID, code, data)
	}
	return nil
}

// getBookDetail 读取最近导入书的详情。
func (w *world) getBookDetail() (bookDetailDTO, error) {
	var d bookDetailDTO
	if w.lastBookID == 0 {
		return d, fmt.Errorf("尚未导入书籍")
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &d)
	if err != nil {
		return d, err
	}
	if code != http.StatusOK {
		return d, fmt.Errorf("GET /api/books/%d 状态码 = %d，body=%s", w.lastBookID, code, body)
	}
	return d, nil
}

// requestCover 请求封面接口，记录状态码与 Content-Type。
func (w *world) requestCover(want int) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/books/%d/cover", w.srv.URL, w.lastBookID), nil)
	if err != nil {
		return err
	}
	resp, err := w.srv.Client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	w.coverType = resp.Header.Get("Content-Type")
	if resp.StatusCode != want {
		return fmt.Errorf("封面接口状态码 = %d，期望 %d", resp.StatusCode, want)
	}
	return nil
}

// assertShelfTitle 断言书架总数与首本书名。
func (w *world) assertShelfTitle(want int, title string) error {
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
	if len(res.Items) == 0 || res.Items[0].Title != title {
		return fmt.Errorf("书架首本书名 = %q，期望 %q", firstTitle(res.Items), title)
	}
	return nil
}

// firstTitle 取首本书名（空列表返回空串）。
func firstTitle(items []struct {
	Title string `json:"title"`
}) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].Title
}

// errorCodeOf 从统一错误响应里取 code。
func errorCodeOf(body []byte) (string, error) {
	var eb struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &eb); err != nil {
		return "", fmt.Errorf("错误响应不是预期 JSON: %w (body=%s)", err, truncate(string(body), 300))
	}
	return eb.Error.Code, nil
}
