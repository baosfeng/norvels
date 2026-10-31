package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"norvels/internal/store"
)

// AuthCookieName 是旧共享口令的 Cookie 名（HttpOnly + SameSite=Lax + Path=/）。
//
// 它与 SessionCookieName 是两件事、两个名字，互不复用：
//   - AuthCookieName   = 「已经过了共享口令这一关」（管理侧、部署脚本用）；
//   - SessionCookieName = 「我是哪个账号」（个人数据归属用）。
//
// 用不同名字而不是同一个 Cookie 塞两种值：旧部署里已经存在 norvels_auth，
// 复用会让升级瞬间把已登录的会话判成无效，也会让「口令对不对」与「你是谁」
// 两件事纠缠在一起，后面谁都不敢改。
const AuthCookieName = "norvels_auth"

// SessionCookieName 是新账号会话的 Cookie 名。
const SessionCookieName = "norvels_session"

// LoginPath 是登录表单页路径。
const LoginPath = "/login"

// logoutPath 是登出路径（清鉴权 Cookie）。
const logoutPath = "/logout"

// authPublicPath 报告该路径是否免鉴权。
//
// 免鉴权清单刻意很小：容器健康检查（/api/health）、登录页/登录提交、两个登出、
// 以及登录页的账号列表（GET /api/auth/users）。
//
// 账号端点里只有 /api/auth/logout 与 GET /api/auth/users 免鉴权：
//   - logout 只清 Cookie、不返回任何数据，且「会话已过期 / 从未登录」时也必须
//     能走到登出，否则前端会被卡在登不出去；
//   - users 是登录页「选账号」列表，登录页本身就在门外，要求凭证才能拿到列表
//     等于要求先登录才能登录。取舍与代价见 handleListUsers 的注释。
//
// /api/auth/register、/api/auth/login、/api/auth/me 都要求先过鉴权：
// 共享口令是「能不能进这个库」，账号是「进来之后你是谁」——口令开启时，
// 不给未持凭证的人一个能自助建号再拿到会话 Cookie 的入口，否则口令形同虚设。
func authPublicPath(p string) bool {
	switch p {
	case "/api/health", LoginPath, logoutPath, "/api/auth/logout", "/api/auth/users":
		return true
	}
	return false
}

// authMiddleware 在启用鉴权时校验 Bearer Token 或 Cookie。
//
// 未通过时：/api/* 返回 401 JSON；页面请求 303 跳到 /login?next=<原路径>。
// 令牌比较使用 crypto/subtle 恒定时间比较，避免按字节泄露前缀。
//
// 通过之后还会把「当前账号」注入 context（见 resolveUser）：即使请求是靠旧的
// 共享口令进来的，个人数据也需要一个明确的归属。
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	if s.cfg.AuthToken == "" {
		// 开放模式：没有「能不能进门」这回事，但「你是谁」仍然要解析——
		// 注册过的账号带会话 Cookie 时，个人数据按该账号隔离。
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(s.userScope(r)))
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authPublicPath(r.URL.Path) || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r.WithContext(s.userScope(r)))
			return
		}
		if ctx, ok := s.authorize(r); ok {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		s.rejectUnauthorized(w, r)
	})
}

// authorize 判定这次请求能否通过鉴权，并返回带账号归属的 context。
//
// 两类凭证等价放行（口径：不破坏现有部署，同时让新账号体系可用）：
//  1. 旧的共享口令（Bearer 头或 norvels_auth Cookie）——管理侧、部署脚本、健康检查继续用；
//  2. 新的账号会话 Cookie（norvels_session）——普通读者用它登录。
func (s *Server) authorize(r *http.Request) (context.Context, bool) {
	if tokenMatches(s.cfg.AuthToken, requestToken(r)) {
		return s.userScope(r), true
	}
	if tok := sessionToken(r); tok != "" {
		if u, err := s.st.UserBySessionToken(r.Context(), tok); err == nil {
			return store.WithUserID(r.Context(), u.ID), true
		}
	}
	return r.Context(), false
}

// userScope 解析「这个请求代表哪个账号」并写回 context。
//
// 解析顺序：有效的新会话 Cookie → 默认账号（存量数据归属）。
// 会话无效（过期 / 被撤销 / 从未登录）时回落到默认账号而不是报错：
// 靠共享口令进来的管理侧调用、部署脚本、旧 Cookie 必须继续能用，
// 不能因为「没注册过账号」就把整个库锁死。
func (s *Server) userScope(r *http.Request) context.Context {
	if tok := sessionToken(r); tok != "" {
		if u, err := s.st.UserBySessionToken(r.Context(), tok); err == nil {
			return store.WithUserID(r.Context(), u.ID)
		}
	}
	return store.WithUserID(r.Context(), store.DefaultUserID())
}

// sessionToken 从请求里取新会话 Cookie 的值（没有则空串）。
func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(SessionCookieName); err == nil {
		return strings.TrimSpace(c.Value)
	}
	return ""
}

// rejectUnauthorized 按请求类型给出 401 或跳转登录页。
func (s *Server) rejectUnauthorized(w http.ResponseWriter, r *http.Request) {
	if isAPIPath(r.URL.Path) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="norvels", charset="UTF-8"`)
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

// requestToken 依次从 Authorization 头、旧口令 Cookie 中取出候选令牌。
//
// 只认 AuthCookieName：norvels_session 是会话凭据，由 sessionToken 单独解析，
// 两条路径必须分得开——把会话 token 当成口令去比对，只会让它莫名失效。
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
