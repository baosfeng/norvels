package api

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"norvels/internal/importer"
	"norvels/internal/store"
)

// 合成 zip 上传样本：书名/作者/正文全部虚构（隐私红线）。

// zipUploadBytes 在内存里合成一个 zip（key 是包内路径）。
func zipUploadBytes(t *testing.T, entries [][2]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e[0])
		if err != nil {
			t.Fatalf("创建 zip 条目 %s: %v", e[0], err)
		}
		if _, err := w.Write([]byte(e[1])); err != nil {
			t.Fatalf("写入 zip 条目 %s: %v", e[0], err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip: %v", err)
	}
	return buf.String()
}

// TestUploadZIPListsEntries 覆盖 zip 上传：包内 txt 逐条进 accepted、pdf 进 rejected，
// 条目名带「包名.zip:包内路径」，format 取包内真实格式。
func TestUploadZIPListsEntries(t *testing.T) {
	env, root := uploadEnv(t, nil)
	packed := zipUploadBytes(t, [][2]string{
		{"甲.txt", sampleTxt("合成甲", "某甲")},
		{"乙.txt", sampleTxt("合成乙", "某乙")},
		{"说明.pdf", "not a book"},
	})
	rr := doUpload(t, env, []uploadFile{{name: "合订包.zip", content: packed}}, "")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("状态码 = %d，期望 202，body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeBody[uploadResponse](t, rr)
	assertZipUploadEntries(t, resp)
	env.waitJobDone(resp.JobID, 10*time.Second)
	waitUploadDirEmpty(t, root)

	// 包内两本书都应入库，来源名带包内路径。
	books, err := env.srv.st.ListBooks(context.Background(), store.BookQuery{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if books.Total != 2 {
		t.Errorf("入库 %d 本，期望 2", books.Total)
	}
}

// assertZipUploadEntries 校验包内条目的 accepted/rejected 分流。
func assertZipUploadEntries(t *testing.T, resp uploadResponse) {
	t.Helper()
	if len(resp.Accepted) != 2 || len(resp.Rejected) != 1 {
		t.Fatalf("accepted/rejected = %d/%d，期望 2/1（resp=%+v）", len(resp.Accepted), len(resp.Rejected), resp)
	}
	for _, a := range resp.Accepted {
		if !strings.HasPrefix(a.Name, "合订包.zip:") || a.Format != importer.FormatTXT {
			t.Errorf("accepted = %+v，期望「包名.zip:包内路径」且格式 txt", a)
		}
	}
	if got := resp.Rejected[0]; got.Name != "合订包.zip:说明.pdf" || got.Code != codeUnsupportedFormat {
		t.Errorf("rejected = %+v，期望包内 pdf 按不支持拒绝", got)
	}
}

// TestUploadInvalidZIPRejected 覆盖坏 zip：进 rejected（invalid_zip），不建任务。
func TestUploadInvalidZIPRejected(t *testing.T) {
	env, root := uploadEnv(t, nil)
	rr := doUpload(t, env, []uploadFile{{name: "坏包.zip", content: "not a zip at all"}}, "")
	requireError(t, rr, http.StatusBadRequest, codeUnsupportedFormat)
	_ = env
	waitUploadDirEmpty(t, root)
}
