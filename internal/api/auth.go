package api

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
)

// AuthCookieName 是鉴权 Cookie 名（HttpOnly + SameSite=Lax + Path=/）。
const AuthCookieName = "norvels_auth"

// LoginPath 是登录表单页路径。
const LoginPath = "/login"

// logoutPath 是登出路径（清鉴权 Cookie）。
const logoutPath = "/logout"

// authPublicPath 报告该路径是否免鉴权。
//
// 免鉴权清单刻意很小：容器健康检查（/api/health）、登录页/登录提交、登出。
// 其余 /api/* 与全部页面都要求凭证。
//
// /logout 免鉴权是有意的：它只清 Cookie、不返回任何数据，且允许「Cookie 已失效
// 或从未登录」时也能走到登录页，避免 401 卡住用户。
func authPublicPath(p string) bool {
	switch p {
	case "/api/health", LoginPath, logoutPath:
		return true
	}
	return false
}

// authMiddleware 在启用鉴权时校验 Bearer Token 或 Cookie。
//
// 未通过时：/api/* 返回 401 JSON；页面请求 303 跳到 /login?next=<原路径>。
// 令牌比较使用 crypto/subtle 恒定时间比较，避免按字节泄露前缀。
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	token := s.cfg.AuthToken
	if token == "" {
		return next // 开放模式
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authPublicPath(r.URL.Path) || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if tokenMatches(token, requestToken(r)) {
			next.ServeHTTP(w, r)
			return
		}
		s.rejectUnauthorized(w, r)
	})
}

// rejectUnauthorized 按请求类型给出 401 或跳转登录页。
func (s *Server) rejectUnauthorized(w http.ResponseWriter, r *http.Request) {
	if isAPIPath(r.URL.Path) {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"norvels\", charset=\"UTF-8\"")
		writeError(w, http.StatusUnauthorized, codeUnauthorized, "需要鉴权：请带上 Authorization: Bearer <token>")
		return
	}
	http.Redirect(w, r, LoginPath+"?next="+url.QueryEscape(safeNext(r.URL.RequestURI())), http.StatusSeeOther)
}

// isAPIPath 判断是否 API 路径。
func isAPIPath(p string) bool { return p == "/api" || strings.HasPrefix(p, "/api/") }

// safeNext 只接受站内相对路径，避免开放重定向。
func safeNext(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	return raw
}

// requestToken 依次从 Authorization 头、Cookie 中取出候选令牌。
func requestToken(r *http.Request) string {
	if tok := bearerToken(r.Header.Get("Authorization")); tok != "" {
		return tok
	}
	if c, err := r.Cookie(AuthCookieName); err == nil {
		return c.Value
	}
	return ""
}

// bearerToken 从 "Bearer xxx" 中取出 xxx（scheme 大小写不敏感）。
func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	scheme, rest, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(rest)
}

// tokenMatches 恒定时间比较令牌。
func tokenMatches(want, got string) bool {
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}
