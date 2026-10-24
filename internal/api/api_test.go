package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"norvels/internal/model"
	"norvels/internal/store"
)

// testEnv 是一套「真实 SQLite + 完整 handler 链」的测试环境。
type testEnv struct {
	t   *testing.T
	st  *store.Store
	srv *Server
	h   http.Handler
}

// testFS 是测试用的前端产物（index.html + 带哈希的 assets）。
func testFS() fs.FS {
	return fstest.MapFS{
		"index.html":             {Data: []byte("<!doctype html><html lang=\"zh-CN\"><body><div id=\"root\"></div></body></html>")},
		"assets/app-abc123.js":   {Data: []byte("console.log('norvels')")},
		"assets/app-abc123.css":  {Data: []byte("body{margin:0}")},
		"assets/logo-abc123.svg": {Data: []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>")},
	}
}

// emptyFS 模拟「前端未构建」。
func emptyFS() fs.FS { return fstest.MapFS{} }

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	return newEnvWith(t, Config{
		Version:          "slim",
		Commit:           "cafe",
		BuiltAt:          "2026-10-24T00:00:00Z",
		DisableAccessLog: true,
	}, testFS())
}

func newEnvWith(t *testing.T, cfg Config, fsys fs.FS) *testEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "norvels-test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		_ = st.Close()
		t.Fatalf("store.Migrate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	srv, h := newServer(st, cfg, fsys)
	srv.log = log.New(io.Discard, "", 0) // 默认静音，专门测日志的用例再替换
	return &testEnv{t: t, st: st, srv: srv, h: h}
}

// do 发起一次请求并返回 recorder。
func (e *testEnv) do(method, target string, body io.Reader, hdr map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(method, target, body)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)
	return rr
}

func (e *testEnv) get(target string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodGet, target, nil, nil)
}

// decodeBody 解析 JSON 响应体。
func decodeBody[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
		t.Fatalf("解析 JSON 失败: %v\nbody=%s", err, rr.Body.String())
	}
	return v
}

// errResp 是错误响应结构。
type errResp struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func requireError(t *testing.T, rr *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("状态码 = %d，期望 %d，body=%s", rr.Code, status, rr.Body.String())
	}
	body := decodeBody[errResp](t, rr)
	if body.Error.Code != code {
		t.Fatalf("错误码 = %q，期望 %q，body=%s", body.Error.Code, code, rr.Body.String())
	}
	if body.Error.Message == "" {
		t.Fatalf("错误消息为空，body=%s", rr.Body.String())
	}
}

// seedBook 通过 store.SaveBook 写入一本含一卷三章的测试书。
func (e *testEnv) seedBook(title, author string) int64 {
	e.t.Helper()
	sum := sha256.Sum256([]byte("norvels-test:" + title + ":" + author))
	book := &model.ParsedBook{
		Title:      title,
		Author:     author,
		SourcePath: filepath.Join(e.t.TempDir(), title+".txt"),
		SourceName: title + ".txt",
		Encoding:   "utf-8",
		FileSize:   2048,
		SHA256:     hex.EncodeToString(sum[:]),
		WordCount:  24,
		Intro:      "这是一本用于接口测试的书。",
		Volumes:    []model.ParsedVolume{{Idx: 0, Title: "第一卷"}},
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: -1, Title: "序章", Content: "序章正文。", WordCount: 5},
			{Idx: 1, VolumeIdx: 0, Title: "第1章 起", Content: "第一章正文。", WordCount: 6},
			{Idx: 2, VolumeIdx: 0, Title: "第2章 承", Content: "第二章正文。", WordCount: 6},
		},
	}
	res, err := e.st.SaveBook(context.Background(), book, store.SaveOptions{})
	if err != nil {
		e.t.Fatalf("store.SaveBook: %v", err)
	}
	if res.BookID <= 0 {
		e.t.Fatalf("SaveBook 未返回合法 bookID: %+v", res)
	}
	return res.BookID
}

// newTestLogger 构造写入指定缓冲区的 logger。
func newTestLogger(w io.Writer) *log.Logger { return log.New(w, "", 0) }

// waitJobDone 轮询任务直到离开 running（或超时），避免测试清理时后台 goroutine 仍在写库。
func (e *testEnv) waitJobDone(jobID int64, d time.Duration) *store.ImportJobDetail {
	e.t.Helper()
	deadline := time.Now().Add(d)
	for {
		detail, err := e.st.GetImportJob(context.Background(), jobID)
		if err == nil && detail != nil && detail.Status != "running" {
			return detail
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}
