package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"runtime/debug"
)

// wrap 组装中间件链（由外到内）：
// 请求 ID → CORS → 访问日志 → 鉴权 → panic 恢复 → gzip → 路由（API / 静态 SPA）。
//
// 鉴权放在访问日志之内、panic 恢复之外：401/303 都进访问日志，
// 且鉴权拒绝不会触发 panic 恢复的兜底响应。
func (s *Server) wrap(apiHandler http.Handler) http.Handler {
	h := s.dispatch(apiHandler)
	h = gzipMiddleware(h)
	h = s.recoverMiddleware(h)
	h = s.authMiddleware(h)
	h = s.accessLogMiddleware(h)
	h = s.corsMiddleware(h)
	h = requestIDMiddleware(h, s.cfg.RequestID)
	return h
}

// isBackendPath 判断请求是否由后端路由（而非前端静态资源）处理。
//
// /api/* 之外，登录相关路径（/login、/logout）也归后端：它们是服务端渲染的
// 表单页与鉴权入口，不能落到 SPA fallback 上（否则 POST /login 会被判 405）。
func isBackendPath(p string) bool {
	if isAPIPath(p) {
		return true
	}
	return p == LoginPath || p == logoutPath
}

// dispatch 按前缀分流：后端路由走 API mux，其余走静态 SPA。
func (s *Server) dispatch(apiHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isBackendPath(r.URL.Path) {
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
