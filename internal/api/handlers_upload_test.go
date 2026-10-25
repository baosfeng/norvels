package api

import (
	"archive/zip"
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"norvels/internal/store"
)

// uploadFile 是一个待上传的文件（内容为文本或小体积二进制）。
type uploadFile struct {
	name    string
	content string
}

// sampleTxt 造一段可被现有 parser 识别的虚构文本（三章）。
func sampleTxt(title, author string) string {
	para := "风从城楼上吹过，他抬头看了看天，觉得这条路还很长。\n\n"
	var b strings.Builder
	b.WriteString("书名：" + title + "\n作者：" + author + "\n\n")
	for _, heading := range []string{"第一章 起", "第二章 承", "第三章 合"} {
		b.WriteString(heading + "\n")
		for i := 0; i < 3; i++ {
			b.WriteString(para)
		}
	}
	return b.String()
}

// multipartRequest 构造 multipart 请求体与 Content-Type。
func multipartRequest(t *testing.T, files []uploadFile, force string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		w, err := mw.CreateFormFile("files", f.name)
		if err != nil {
			t.Fatalf("CreateFormFile(%q): %v", f.name, err)
		}
		if _, err := w.Write([]byte(f.content)); err != nil {
			t.Fatalf("写入 %q: %v", f.name, err)
		}
	}
	if force != "" {
		if err := mw.WriteField("force", force); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("关闭 multipart: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

// uploadEnv 建一个上传环境，返回 env 与上传根目录。
func uploadEnv(t *testing.T, mutate func(*Config)) (*testEnv, string) {
	t.Helper()
	root := t.TempDir()
	cfg := Config{Version: "slim", DisableAccessLog: true, UploadDir: root}
	if mutate != nil {
		mutate(&cfg)
	}
	return newEnvWith(t, cfg, testFS()), root
}

// doUpload 发一次上传请求。
func doUpload(t *testing.T, env *testEnv, files []uploadFile, force string) *httptest.ResponseRecorder {
	t.Helper()
	body, ct := multipartRequest(t, files, force)
	return env.do(http.MethodPost, "/api/import/upload", body, map[string]string{"Content-Type": ct})
}

// waitUploadDirEmpty 等后台导入把临时目录删干净（收尾在任务终态之后）。
func waitUploadDirEmpty(t *testing.T, root string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		entries, err := os.ReadDir(root)
		if err == nil && len(entries) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("上传临时目录未清理: entries=%v err=%v", entries, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// assertUploadRootEmpty 断言上传根目录下没有残留目录（不存在也算干净）。
func assertUploadRootEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("读上传根目录: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("上传临时目录未清理: %v", entries)
	}
}

// testEncryptedEPUB 在内存里合成一个带 META-INF/encryption.xml 的 EPUB。
func testEncryptedEPUB(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "META-INF/container.xml", "<container/>")
	addZipFile(t, zw, "META-INF/encryption.xml", "<encryption><EncryptedData/></encryption>")
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip: %v", err)
	}
	return buf.Bytes()
}

// addZipFile 往 zip 里写一个条目。
func addZipFile(t *testing.T, zw *zip.Writer, name, body string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip.Create(%s): %v", name, err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("zip 写入 %s: %v", name, err)
	}
}

// TestUploadImportTxtAccepted 覆盖正常入库：202、接受清单、任务完成、书落库、临时目录删除。
func TestUploadImportTxtAccepted(t *testing.T) {
	env, root := uploadEnv(t, nil)
	rr := doUpload(t, env, []uploadFile{{name: "样例.txt", content: sampleTxt("样例之书", "测试作者")}}, "false")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("状态码 = %d，期望 202，body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeBody[uploadResponse](t, rr)
	requireSingleAccepted(t, resp, "样例.txt")
	if detail := env.waitJobDone(resp.JobID, 5*time.Second); detail == nil {
		t.Fatalf("任务 %d 未在超时内离开 running", resp.JobID)
	}
	requireUploadedBook(t, env, "样例.txt")
	waitUploadDirEmpty(t, root)
}

// requireSingleAccepted 断言响应里恰好一个被接受文件且没有拒绝项。
func requireSingleAccepted(t *testing.T, resp uploadResponse, wantName string) {
	t.Helper()
	if resp.JobID <= 0 || len(resp.Accepted) != 1 || len(resp.Rejected) != 0 {
		t.Fatalf("响应 = %+v，期望一个被接受文件与 jobId", resp)
	}
	if resp.Accepted[0].Name != wantName || resp.Accepted[0].Format != "txt" {
		t.Fatalf("accepted = %+v，期望 name=%s format=txt", resp.Accepted[0], wantName)
	}
}

// requireUploadedBook 断言书已入库且来源名保留为上传时的原名。
func requireUploadedBook(t *testing.T, env *testEnv, wantName string) {
	t.Helper()
	page, err := env.st.ListBooks(context.Background(), store.BookQuery{Page: 1, Size: 10, Sort: "updated"})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("入库书本数 = %d，期望 1", page.Total)
	}
	book, err := env.st.GetBook(context.Background(), page.Items[0].ID)
	if err != nil || book == nil {
		t.Fatalf("GetBook: %v", err)
	}
	if book.SourceName != wantName {
		t.Fatalf("sourceName = %q，期望落盘后的原文件名", book.SourceName)
	}
}

// TestUploadImportPartialRejected 覆盖混批：不支持的文件进 rejected，合法文件继续导入。
func TestUploadImportPartialRejected(t *testing.T) {
	env, _ := uploadEnv(t, nil)
	rr := doUpload(t, env, []uploadFile{
		{name: "样例.txt", content: sampleTxt("样例之书", "测试作者")},
		{name: "说明.pdf", content: "not a book"},
	}, "")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("状态码 = %d，期望 202，body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeBody[uploadResponse](t, rr)
	if len(resp.Accepted) != 1 || len(resp.Rejected) != 1 {
		t.Fatalf("响应 = %+v，期望 1 接受 1 拒绝", resp)
	}
	rejected := resp.Rejected[0]
	if rejected.Name != "说明.pdf" || rejected.Code != codeUnsupportedFormat {
		t.Fatalf("rejected = %+v，期望 unsupported_format 与原名", rejected)
	}
	if !strings.Contains(rejected.Message, "目前支持 TXT 与 EPUB") || !strings.Contains(rejected.Message, ".pdf") {
		t.Fatalf("rejected.message = %q，期望含 .pdf 与支持格式提示", rejected.Message)
	}
	env.waitJobDone(resp.JobID, 5*time.Second)
}

// TestUploadImportAllRejected 覆盖整批都不支持：400 unsupported_format，且不建任务。
func TestUploadImportAllRejected(t *testing.T) {
	env, root := uploadEnv(t, nil)
	rr := doUpload(t, env, []uploadFile{
		{name: "说明.pdf", content: "x"},
		{name: "报告.docx", content: "y"},
	}, "")
	requireError(t, rr, http.StatusBadRequest, codeUnsupportedFormat)
	body := decodeBody[errResp](t, rr)
	if !strings.Contains(body.Error.Message, "目前支持 TXT 与 EPUB") {
		t.Fatalf("message = %q，期望含支持格式提示", body.Error.Message)
	}
	assertUploadRootEmpty(t, root)
}

// TestUploadImportFileTooLarge 覆盖单文件超限：413 payload_too_large，消息带上限。
func TestUploadImportFileTooLarge(t *testing.T) {
	env, root := uploadEnv(t, func(c *Config) { c.MaxUploadFileBytes = 32 })
	rr := doUpload(t, env, []uploadFile{{name: "样例.txt", content: sampleTxt("样例之书", "测试作者")}}, "")
	requireError(t, rr, http.StatusRequestEntityTooLarge, codePayloadTooLarge)
	body := decodeBody[errResp](t, rr)
	if !strings.Contains(body.Error.Message, "上限") {
		t.Fatalf("message = %q，期望提示上限", body.Error.Message)
	}
	assertUploadRootEmpty(t, root)
}

// TestUploadImportTotalTooLarge 覆盖整批总量超限：413 payload_too_large。
func TestUploadImportTotalTooLarge(t *testing.T) {
	env, root := uploadEnv(t, func(c *Config) {
		c.MaxUploadFileBytes = 8 << 10
		c.MaxUploadTotalBytes = 64
	})
	rr := doUpload(t, env, []uploadFile{
		{name: "一.txt", content: sampleTxt("样例之书", "测试作者")},
		{name: "二.txt", content: sampleTxt("另一本书", "测试作者")},
	}, "")
	requireError(t, rr, http.StatusRequestEntityTooLarge, codePayloadTooLarge)
	body := decodeBody[errResp](t, rr)
	if !strings.Contains(body.Error.Message, "总量") {
		t.Fatalf("message = %q，期望提示总量上限", body.Error.Message)
	}
	assertUploadRootEmpty(t, root)
}

// TestUploadImportTooManyFiles 覆盖文件数超限：400 too_many_files。
func TestUploadImportTooManyFiles(t *testing.T) {
	env, root := uploadEnv(t, func(c *Config) { c.MaxUploadFiles = 2 })
	rr := doUpload(t, env, []uploadFile{
		{name: "一.txt", content: "a"},
		{name: "二.txt", content: "b"},
		{name: "三.txt", content: "c"},
	}, "")
	requireError(t, rr, http.StatusBadRequest, codeTooManyFiles)
	assertUploadRootEmpty(t, root)
}

// TestUploadImportConflict 覆盖已有导入任务：409，且不落任何上传文件。
func TestUploadImportConflict(t *testing.T) {
	env, root := uploadEnv(t, nil)
	if _, err := env.st.CreateImportJob(context.Background(), "/path/to/txt"); err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	rr := doUpload(t, env, []uploadFile{{name: "样例.txt", content: sampleTxt("样例之书", "测试作者")}}, "")
	requireError(t, rr, http.StatusConflict, codeConflict)
	assertUploadRootEmpty(t, root)
}

// TestUploadImportEncryptedEPUB 覆盖加密 EPUB：422 encrypted_epub，不建任务、不写半本书。
func TestUploadImportEncryptedEPUB(t *testing.T) {
	env, root := uploadEnv(t, nil)
	rr := doUpload(t, env, []uploadFile{{name: "加密.epub", content: string(testEncryptedEPUB(t))}}, "")
	requireError(t, rr, http.StatusUnprocessableEntity, codeEncryptedEPUB)
	assertUploadRootEmpty(t, root)
}

// TestUploadImportSanitizesFileName 覆盖文件名清洗：路径穿越被收敛成 base 名。
func TestUploadImportSanitizesFileName(t *testing.T) {
	env, _ := uploadEnv(t, nil)
	raw := "../../样例之书.txt"
	rr := doUpload(t, env, []uploadFile{{name: raw, content: sampleTxt("样例之书", "测试作者")}}, "")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("状态码 = %d，期望 202，body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeBody[uploadResponse](t, rr)
	if len(resp.Accepted) != 1 {
		t.Fatalf("响应 = %+v，期望 1 个接受文件", resp)
	}
	name := resp.Accepted[0].Name
	if name != "样例之书.txt" {
		t.Fatalf("accepted.name = %q，期望只剩 base 名（去掉路径）", name)
	}
	env.waitJobDone(resp.JobID, 5*time.Second)
}

// TestUploadImportRequiresMultipart 覆盖非 multipart 请求：400 invalid_param。
func TestUploadImportRequiresMultipart(t *testing.T) {
	env, _ := uploadEnv(t, nil)
	rr := env.do(http.MethodPost, "/api/import/upload", strings.NewReader("not multipart body"),
		map[string]string{"Content-Type": "application/json"})
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
}
