package api

import (
	"net/http"
	"strings"
)

// loginPageHTML 是单用户登录表单页（不依赖前端构建产物，纯内联 HTML/CSS）。
//
// 占位符 {{NEXT}} / {{ERROR}} 由 writeLoginPage 替换。
const loginPageHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>norvels · 登录</title>
<style>
  :root { color-scheme: light dark; }
  body { margin:0; min-height:100vh; display:grid; place-items:center;
         font:15px/1.6 system-ui, -apple-system, "PingFang SC", sans-serif;
         background:#faf8f4; color:#2b2723; }
  @media (prefers-color-scheme: dark) { body { background:#1b1917; color:#eae5de; } }
  form { width:min(340px, 88vw); padding:26px 24px; border-radius:14px;
         background:color-mix(in srgb, currentColor 6%, transparent); }
  h1 { margin:0 0 6px; font-size:17px; letter-spacing:.01em; }
  p  { margin:0 0 18px; opacity:.65; font-size:13px; }
  input[type=password] { width:100%; box-sizing:border-box; padding:10px 12px;
         border-radius:8px; border:1px solid color-mix(in srgb, currentColor 25%, transparent);
         background:transparent; color:inherit; font-size:15px; }
  button { margin-top:14px; width:100%; padding:10px 12px; border-radius:8px;
         border:0; background:#8a5a2b; color:#fff; font-size:15px; cursor:pointer; }
  .err { margin-top:12px; color:#b3423a; font-size:13px; }
</style>
</head>
<body>
<form method="post" action="/login">
  <h1>norvels</h1>
  <p>输入共享口令以访问书库</p>
  <input type="password" name="token" autocomplete="current-password" autofocus required>
  <input type="hidden" name="next" value="{{NEXT}}">
  <button type="submit">进入</button>
  {{ERROR}}
</form>
</body>
</html>
`

// handleLoginPage 渲染登录表单。
func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AuthEnabled() {
		// 开放模式没有登录这回事，直接回首页，避免误导。
		http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusSeeOther)
		return
	}
	s.writeLoginPage(w, r.URL.Query().Get("next"), "")
}

// handleLoginSubmit 校验表单口令；成功则下发 HttpOnly Cookie 并跳回原页面。
func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AuthEnabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.writeLoginPage(w, "/", "表单格式不正确")
		return
	}
	next := r.PostFormValue("next")
	if !tokenMatches(s.cfg.AuthToken, strings.TrimSpace(r.PostFormValue("token"))) {
		w.WriteHeader(http.StatusUnauthorized)
		s.writeLoginPage(w, next, "口令不正确")
		return
	}
	http.SetCookie(w, authCookie(r, s.cfg.AuthToken, 30*24*3600))
	http.Redirect(w, r, safeNext(next), http.StatusSeeOther)
}

// handleLogout 清掉鉴权 Cookie。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, authCookie(r, "", -1))
	http.Redirect(w, r, LoginPath, http.StatusSeeOther)
}

// authCookie 构造鉴权 Cookie：HttpOnly + SameSite=Lax + Path=/。
// maxAge 为负表示立即过期（登出）。
func authCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     AuthCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   maxAge,
	}
}

// writeLoginPage 渲染登录页并写入 next 与错误提示。
func (s *Server) writeLoginPage(w http.ResponseWriter, next, errMsg string) {
	body := strings.ReplaceAll(loginPageHTML, "{{NEXT}}", escapeHTMLText(safeNext(next)))
	body = strings.ReplaceAll(body, "{{ERROR}}", loginError(errMsg))
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(body))
}

// loginError 把错误提示渲染成 HTML 片段（空则不渲染）。
func loginError(msg string) string {
	if msg == "" {
		return ""
	}
	return `<div class="err">` + escapeHTMLText(msg) + `</div>`
}

// escapeHTMLText 转义 HTML 文本节点与属性值。
func escapeHTMLText(s string) string {
	return htmlTextEscaper.Replace(s)
}

// htmlTextEscaper 是 HTML 文本/属性转义表。
var htmlTextEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;",
)
