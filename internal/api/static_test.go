package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestStaticIndexAndSPAFallback(t *testing.T) {
	env := newEnv(t)

	rr := env.get("/")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET / 状态码 = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q，期望 text/html", ct)
	}
	if !strings.Contains(rr.Body.String(), "root") {
		t.Fatalf("index.html 内容异常: %s", rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("index.html Cache-Control = %q，期望 no-cache", got)
	}

	// SPA 路由回退到 index.html（200）
	for _, target := range []string{"/books", "/books/42", "/books/42/chapters/7", "/authors/3"} {
		spa := env.get(target)
		if spa.Code != http.StatusOK {
			t.Fatalf("%s 状态码 = %d，期望 200", target, spa.Code)
		}
		if !strings.Contains(spa.Body.String(), "root") {
			t.Fatalf("%s 未回退到 index.html: %s", target, spa.Body.String())
		}
	}
}

func TestStaticAssetLongCacheAndETag(t *testing.T) {
	env := newEnv(t)

	rr := env.get("/assets/app-abc123.js")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rr.Code)
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("Content-Type = %q，期望 javascript", ct)
	}
	etag := rr.Header().Get("ETag")
	if etag == "" {
		t.Fatal("缺少 ETag")
	}
	if rr.Body.String() != "console.log('norvels')" {
		t.Fatalf("资源内容 = %q", rr.Body.String())
	}

	// If-None-Match 命中 → 304
	notModified := env.do(http.MethodGet, "/assets/app-abc123.js", nil, map[string]string{"If-None-Match": etag})
	if notModified.Code != http.StatusNotModified {
		t.Fatalf("ETag 命中状态码 = %d，期望 304", notModified.Code)
	}
	if notModified.Body.Len() != 0 {
		t.Fatalf("304 不应有响应体，实际 %q", notModified.Body.String())
	}

	// 弱校验前缀也接受
	weak := env.do(http.MethodGet, "/assets/app-abc123.js", nil, map[string]string{"If-None-Match": "W/" + etag})
	if weak.Code != http.StatusNotModified {
		t.Fatalf("弱 ETag 状态码 = %d，期望 304", weak.Code)
	}

	// HEAD：只有头，无正文
	head := env.do(http.MethodHead, "/assets/app-abc123.js", nil, nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD 结果 = %d，body=%q", head.Code, head.Body.String())
	}
	if head.Header().Get("ETag") == "" {
		t.Fatal("HEAD 也应带 ETag")
	}
}

func TestStaticMissingResource(t *testing.T) {
	env := newEnv(t)
	requireError(t, env.get("/assets/missing.css"), http.StatusNotFound, codeNotFound)
	requireError(t, env.get("/missing.js"), http.StatusNotFound, codeNotFound)
	requireError(t, env.get("/favicon.ico"), http.StatusNotFound, codeNotFound)
}

func TestStaticMethodNotAllowed(t *testing.T) {
	env := newEnv(t)
	requireError(t, env.do(http.MethodPost, "/", nil, nil), http.StatusMethodNotAllowed, codeMethodNotAllowed)
}

func TestStaticPlaceholderWhenFrontendMissing(t *testing.T) {
	env := newEnvWith(t, Config{Version: "slim", DisableAccessLog: true}, emptyFS())

	rr := env.get("/")
	if rr.Code != http.StatusOK {
		t.Fatalf("占位页状态码 = %d，期望 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("占位页 Content-Type = %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "<") {
		t.Fatalf("占位页内容异常: %s", rr.Body.String())
	}

	// 深链接同样回退到占位页
	deep := env.get("/books/42")
	if deep.Code != http.StatusOK {
		t.Fatalf("深链接状态码 = %d，期望 200", deep.Code)
	}
	// 但资源后缀仍应 404
	requireError(t, env.get("/assets/app.js"), http.StatusNotFound, codeNotFound)
}
