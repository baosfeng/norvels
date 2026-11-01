package api

// gzip 判定链的单元测试：中间件只做了「协商 + panic 兜底」，
// 压缩与否的决策全在 gzipResponseWriter 里，因此直接打各个分支比绕经 HTTP 更准。

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// plainWriter 屏蔽 Flush：用来覆盖「底层 writer 不支持 Flush」的分支。
type plainWriter struct{ http.ResponseWriter }

// newGzipWriter 造一个直接可调用的 gzip 响应写入器。
func newGzipWriter(hdr map[string]string) (*gzipResponseWriter, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	w := &gzipResponseWriter{ResponseWriter: rec, status: http.StatusOK}
	for k, v := range hdr {
		w.Header().Set(k, v)
	}
	return w, rec
}

// gunzip 解压 recorder 里的正文。
func gunzip(t *testing.T, raw []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	return string(out)
}

// TestCompressibleStatus 状态码白名单：2xx 可压缩，1xx/204/304/206/3xx 不可。
func TestCompressibleStatus(t *testing.T) {
	yes := []int{http.StatusOK, http.StatusCreated, http.StatusTeapot}
	no := []int{100, http.StatusNoContent, http.StatusNotModified, http.StatusPartialContent, http.StatusFound}
	for _, code := range yes {
		if !compressibleStatus(code) {
			t.Errorf("compressibleStatus(%d) = false，期望 true", code)
		}
	}
	for _, code := range no {
		if compressibleStatus(code) {
			t.Errorf("compressibleStatus(%d) = true，期望 false", code)
		}
	}
}

// TestCompressibleContentType 精确 MIME 表 + text/* 兜底。
func TestCompressibleContentType(t *testing.T) {
	cases := map[string]bool{
		"":                         false,
		"application/json":         true,
		"Application/JSON":         true, // 大小写不敏感
		"text/html; charset=utf-8": true,
		"image/svg+xml":            true,
		"image/png":                false,
		"application/octet-stream": false,
	}
	for ct, want := range cases {
		if got := compressibleContentType(ct); got != want {
			t.Errorf("compressibleContentType(%q) = %v，期望 %v", ct, got, want)
		}
	}
}

// TestGzipWriterHeaderIdempotent 响应头只认第一次：重复 WriteHeader 与「先 Write 再 WriteHeader」都不改写状态。
func TestGzipWriterHeaderIdempotent(t *testing.T) {
	w, rec := newGzipWriter(nil)
	w.WriteHeader(http.StatusCreated)
	w.WriteHeader(http.StatusInternalServerError) // 第二次应被忽略
	if w.status != http.StatusCreated {
		t.Fatalf("重复 WriteHeader 后 status = %d，期望 201", w.status)
	}
	w.buf = nil
	_ = w.Close()
	if rec.Code != http.StatusCreated {
		t.Errorf("底层状态码 = %d，期望 201", rec.Code)
	}

	w2, _ := newGzipWriter(nil)
	_, _ = w2.Write([]byte("x"))
	w2.WriteHeader(http.StatusNotFound) // 已写过正文，忽略
	if w2.status != http.StatusOK {
		t.Errorf("Write 之后再 WriteHeader，status = %d，期望 200", w2.status)
	}
}

// TestGzipWriterSkipsUncompressed 不该压缩的四类情形：已编码、状态不允许、长度过小、类型未知。
func TestGzipWriterSkipsUncompressed(t *testing.T) {
	cases := []struct {
		name    string
		hdr     map[string]string
		status  int
		payload string
	}{
		{"已带 Content-Encoding", map[string]string{"Content-Type": "application/json", "Content-Encoding": "br"}, 0, strings.Repeat("a", 600)},
		{"204 无正文", map[string]string{"Content-Type": "application/json"}, http.StatusNoContent, strings.Repeat("a", 600)},
		{"3xx 跳转", map[string]string{"Content-Type": "text/plain"}, http.StatusFound, strings.Repeat("a", 600)},
		{"Content-Length 过小", map[string]string{"Content-Type": "application/json", "Content-Length": "10"}, 0, strings.Repeat("a", 600)},
		{"类型不可压缩", map[string]string{"Content-Type": "image/png"}, 0, strings.Repeat("a", 600)},
		{"无 Content-Type", nil, 0, strings.Repeat("a", 600)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, rec := newGzipWriter(c.hdr)
			if c.status != 0 {
				w.WriteHeader(c.status)
			}
			if n, err := w.Write([]byte(c.payload)); err != nil || n != len(c.payload) {
				t.Fatalf("Write = %d, %v", n, err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			// 只断言「没有叠加 gzip」：调用方自己设的 Content-Encoding（如 br）要原样保留。
			if got := rec.Header().Get("Content-Encoding"); got != c.hdr["Content-Encoding"] {
				t.Errorf("不该压缩，Content-Encoding = %q，期望 %q", got, c.hdr["Content-Encoding"])
			}
			if rec.Body.String() != c.payload {
				t.Errorf("正文被改写，长度 = %d", rec.Body.Len())
			}
		})
	}
}

// TestGzipWriterWriteAfterDecision 压缩决策只做一次：后续 Write 直接进 gz 流。
func TestGzipWriterWriteAfterDecision(t *testing.T) {
	w, rec := newGzipWriter(map[string]string{"Content-Type": "text/plain; charset=utf-8"})
	head := strings.Repeat("甲", 400)
	tail := strings.Repeat("乙", 400)
	if _, err := w.Write([]byte(head)); err != nil { // 超过 512 字节 → 决定压缩
		t.Fatalf("首次 Write: %v", err)
	}
	if !w.decided || !w.compress {
		t.Fatalf("decided=%v compress=%v，期望已决定且压缩", w.decided, w.compress)
	}
	if _, err := w.Write([]byte(tail)); err != nil {
		t.Fatalf("二次 Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := gunzip(t, rec.Body.Bytes()); got != head+tail {
		t.Errorf("解压后内容不一致，长度 %d", len(got))
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary = %q，期望包含 Accept-Encoding", rec.Header().Get("Vary"))
	}
}

// TestGzipWriterSmallThenUncompressible 先缓冲小正文、后决定不压缩：缓冲内容必须照样发出。
func TestGzipWriterSmallThenUncompressible(t *testing.T) {
	w, rec := newGzipWriter(map[string]string{"Content-Type": "image/png"})
	if _, err := w.Write([]byte("small")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := w.Write([]byte("more")); err != nil { // decided=false → 再次判定 → start(false)
		t.Fatalf("Write 2: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if rec.Body.String() != "smallmore" {
		t.Errorf("正文 = %q，期望 smallmore", rec.Body.String())
	}
}

// TestGzipWriterEmptyBody Close 未写正文时补写响应头，且不产生正文。
func TestGzipWriterEmptyBody(t *testing.T) {
	w, rec := newGzipWriter(map[string]string{"Content-Type": "application/json"})
	w.WriteHeader(http.StatusCreated)
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if rec.Code != http.StatusCreated || rec.Body.Len() != 0 {
		t.Errorf("结果 = %d, body=%q", rec.Code, rec.Body.String())
	}
}

// TestGzipWriterStartIdempotent 重复 start 不重复写响应头。
func TestGzipWriterStartIdempotent(t *testing.T) {
	w, rec := newGzipWriter(nil)
	w.start(true, nil)
	w.start(false, []byte("ignored"))
	if rec.Body.Len() != 0 {
		t.Errorf("重复 start 不该再写正文，实际 %q", rec.Body.String())
	}
}

// TestGzipWriterFlushDecides Flush 在未决定时先做决策，并把 Flush 透传到底层。
func TestGzipWriterFlushDecides(t *testing.T) {
	w, rec := newGzipWriter(map[string]string{"Content-Type": "text/event-stream; charset=utf-8"})
	w.Flush()
	if !w.decided || !w.compress {
		t.Fatalf("Flush 后 decided=%v compress=%v，期望已决定且压缩", w.decided, w.compress)
	}
	if !strings.Contains(rec.Header().Get("Content-Encoding"), "gzip") {
		t.Errorf("Content-Encoding = %q，期望 gzip", rec.Header().Get("Content-Encoding"))
	}

	// 决策后再 Flush：直接走 gz.Flush + 底层 Flush。
	if _, err := w.Write([]byte(strings.Repeat("a", 600))); err != nil {
		t.Fatalf("Write: %v", err)
	}
	w.Flush()
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestGzipWriterFlushWithoutFlusher 底层不支持 Flush 时不应 panic（只是不转发）。
func TestGzipWriterFlushWithoutFlusher(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &gzipResponseWriter{ResponseWriter: plainWriter{rec}, status: http.StatusOK}
	w.Header().Set("Content-Type", "application/json")
	w.Flush()
	if !w.decided {
		t.Fatal("Flush 之后应已做决策")
	}
}

// TestGzipMiddlewareAbortsBufferedOnPanic handler 在发出响应头之前 panic：
// 已缓冲的正文必须丢弃，客户端只看到恢复中间件写的 500 JSON。
func TestGzipMiddlewareAbortsBufferedOnPanic(t *testing.T) {
	env := newEnv(t)
	h := env.srv.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "POISON") // 小于 512，只进缓冲
		panic("测试用 panic")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/big", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d，期望 500", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "POISON") {
		t.Errorf("panic 前的缓冲正文不该发给客户端，实际 %q", rr.Body.String())
	}
}
