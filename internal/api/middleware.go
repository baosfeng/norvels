package api

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"runtime/debug"
)

// gzipMinSize 是启用 gzip 的最小响应字节数（小于该值压缩收益为负）。
const gzipMinSize = 512

// wrap 组装中间件链（由外到内）：
// 请求 ID → CORS → 访问日志 → panic 恢复 → gzip → 路由（API / 静态 SPA）。
func (s *Server) wrap(apiHandler http.Handler) http.Handler {
	h := s.dispatch(apiHandler)
	h = gzipMiddleware(h)
	h = s.recoverMiddleware(h)
	h = s.accessLogMiddleware(h)
	h = s.corsMiddleware(h)
	h = requestIDMiddleware(h, s.cfg.RequestID)
	return h
}

// dispatch 按前缀分流：/api 走 API mux，其余走静态 SPA。
func (s *Server) dispatch(apiHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/api" || strings.HasPrefix(p, "/api/") {
			apiHandler.ServeHTTP(w, r)
			return
		}
		s.serveStatic(w, r)
	})
}

// ---------- 访问日志 ----------

type responseRecorder struct {
	http.ResponseWriter
	status  int
	written bool
	bytes   int64
}

func (r *responseRecorder) WriteHeader(code int) {
	if r.written {
		return
	}
	r.status = code
	r.written = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	if !r.written {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += int64(n)
	return n, err
}

// Written 报告是否已把响应头发给客户端。
func (r *responseRecorder) Written() bool { return r.written }

// Unwrap 让 http.ResponseController 能拿到底层 writer。
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) accessLogMiddleware(next http.Handler) http.Handler {
	if s.cfg.DisableAccessLog {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		status := rec.status
		if !rec.written {
			status = http.StatusOK
		}
		dur := time.Since(start)
		if rid := requestIDFrom(r.Context()); rid != "" {
			s.logf("api: %s %s %d %s rid=%s bytes=%d",
				r.Method, r.URL.RequestURI(), status, dur.Round(time.Microsecond), rid, rec.bytes)
			return
		}
		s.logf("api: %s %s %d %s bytes=%d",
			r.Method, r.URL.RequestURI(), status, dur.Round(time.Microsecond), rec.bytes)
	})
}

// ---------- panic 恢复 ----------

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler { //nolint:errorlint // 标准库约定的哨兵值
				panic(rec)
			}
			s.logf("api: panic %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
			if !headerWritten(w) {
				writeError(w, http.StatusInternalServerError, codeInternal, "内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// headerWritten 报告响应头是否已发出。
func headerWritten(w http.ResponseWriter) bool {
	if wr, ok := w.(interface{ Written() bool }); ok {
		return wr.Written()
	}
	return false
}

// ---------- gzip ----------

func acceptGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc := strings.ToLower(strings.TrimSpace(part))
		if enc == "gzip" || strings.HasPrefix(enc, "gzip;") {
			return true
		}
	}
	return false
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || r.Header.Get("Range") != "" || !acceptGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if rec := recover(); rec != nil {
				gw.abort()
				panic(rec)
			}
			_ = gw.Close()
		}()
		next.ServeHTTP(gw, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	status      int
	wroteHeader bool // handler 是否调用过 WriteHeader
	sent        bool // 是否已把响应头发给底层 writer
	decided     bool // 是否已决定压缩与否
	compress    bool
	buf         []byte
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.sent || w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = code
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.status = http.StatusOK
	}
	if w.decided {
		if w.compress {
			return w.gz.Write(p)
		}
		return w.ResponseWriter.Write(p)
	}
	if !w.canCompress() {
		w.start(false, p)
		return len(p), nil
	}
	if len(w.buf)+len(p) < gzipMinSize {
		w.buf = append(w.buf, p...)
		return len(p), nil
	}
	w.start(true, p)
	return len(p), nil
}

// compressibleMIMEs 是明确值得压缩的精确 MIME 类型；其余按 text/* 前缀兜底。
var compressibleMIMEs = map[string]bool{
	"application/json":          true,
	"application/javascript":    true,
	"application/xml":           true,
	"application/xhtml+xml":     true,
	"application/manifest+json": true,
	"image/svg+xml":             true,
}

// canCompress 依据已设置的响应头判断是否值得压缩。
func (w *gzipResponseWriter) canCompress() bool {
	h := w.Header()
	if h.Get("Content-Encoding") != "" {
		return false
	}
	if !compressibleStatus(w.status) {
		return false
	}
	if cl := h.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n < gzipMinSize {
			return false
		}
	}
	return compressibleContentType(h.Get("Content-Type"))
}

// compressibleStatus 判断状态码是否允许压缩：2xx 里只排除无正文/分段响应，
// 3xx 整体不压缩（多为跳转，正文很小）。
func compressibleStatus(status int) bool {
	switch {
	case status < 200:
		return false
	case status == http.StatusNoContent, status == http.StatusNotModified, status == http.StatusPartialContent:
		return false
	case status >= 300 && status < 400:
		return false
	}
	return true
}

// compressibleContentType 按 MIME 类型判断是否值得压缩：已知文本类精确匹配，
// 其余以 text/ 开头的一律认为可压缩。
func compressibleContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mime := strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	if compressibleMIMEs[mime] {
		return true
	}
	return strings.HasPrefix(mime, "text/")
}

// start 做出压缩决策并写出响应头与已缓冲内容。
func (w *gzipResponseWriter) start(compress bool, first []byte) {
	if w.decided {
		return
	}
	w.decided = true
	w.compress = compress
	if compress {
		h := w.Header()
		h.Del("Content-Length")
		h.Set("Content-Encoding", "gzip")
		h.Add("Vary", "Accept-Encoding")
		w.gz = gzip.NewWriter(w.ResponseWriter)
	}
	w.ResponseWriter.WriteHeader(w.status)
	w.sent = true
	chunk := w.buf
	w.buf = nil
	if len(first) > 0 {
		chunk = append(chunk, first...)
	}
	if len(chunk) == 0 {
		return
	}
	if compress {
		_, _ = w.gz.Write(chunk)
		return
	}
	_, _ = w.ResponseWriter.Write(chunk)
}

func (w *gzipResponseWriter) Flush() {
	if !w.decided {
		if !w.wroteHeader {
			w.wroteHeader = true
			w.status = http.StatusOK
		}
		w.start(w.canCompress(), nil)
	}
	if w.compress && w.gz != nil {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Close 结束压缩流；未写正文时补写响应头。
func (w *gzipResponseWriter) Close() error {
	if w.decided {
		if w.compress && w.gz != nil {
			return w.gz.Close()
		}
		return nil
	}
	if !w.wroteHeader {
		w.wroteHeader = true
		w.status = http.StatusOK
	}
	w.start(false, nil)
	return nil
}

// abort 丢弃已缓冲内容（panic 恢复时避免把半截响应发给客户端）。
func (w *gzipResponseWriter) abort() {
	w.buf = nil
	w.decided = true
	w.compress = false
}

// ---------- CORS ----------

// devCORSOrigins 是开发期允许的跨域来源（仅 EnableCORS=true 时生效）。
var devCORSOrigins = map[string]bool{
	"http://localhost:5173": true,
	"http://127.0.0.1:5173": true,
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	if !s.cfg.EnableCORS {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin != "" && devCORSOrigins[origin] {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Accept, X-Request-Id")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- 请求 ID ----------

type requestIDKey struct{}

// requestIDMiddleware 为开启时生成/透传 X-Request-Id。
func requestIDMiddleware(next http.Handler, enabled bool) http.Handler {
	if !enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
