package api

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
)

// ---------- gzip ----------

// gzipMinSize 是启用 gzip 的最小响应字节数（小于该值压缩收益为负）。
const gzipMinSize = 512

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
