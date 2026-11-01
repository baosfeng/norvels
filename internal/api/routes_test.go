package api

import (
	"net/http"
	"strings"
	"testing"
)

// httpMethods 是判定「错误动词」时依次尝试的方法（覆盖服务端支持的全部动词）。
var httpMethods = []string{
	http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
}

// concretePath 把模式里的 {xxx} 占位段换成固定值，得到一条可真实请求的路径。
func concretePath(t *testing.T, pattern string) string {
	t.Helper()
	segs := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	for i, seg := range segs {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			segs[i] = "1"
		}
	}
	return "/" + strings.Join(segs, "/")
}

// wrongMethod 返回一个 allowed 之外的动词；全都注册过时返回空串（调用方报错）。
func wrongMethod(allowed []string) string {
	for _, m := range httpMethods {
		if !containsMethod(allowed, m) {
			return m
		}
	}
	return ""
}

// containsMethod 判断方法列表里是否含某个动词（大小写敏感，与 HTTP 方法一致）。
func containsMethod(list []string, want string) bool {
	for _, m := range list {
		if m == want {
			return true
		}
	}
	return false
}

// allowSet 把 Allow 头解析成方法集合（顺序不参与比较）。
func allowSet(t *testing.T, raw string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		if m := strings.TrimSpace(part); m != "" {
			out[m] = true
		}
	}
	return out
}

// requireAllow 断言 Allow 头与期望方法集合完全一致（多一个、少一个都算错）。
func requireAllow(t *testing.T, raw string, want []string) {
	t.Helper()
	got := allowSet(t, raw)
	if len(got) != len(want) {
		t.Fatalf("Allow = %q，期望 %v", raw, want)
	}
	for _, m := range want {
		if !got[m] {
			t.Fatalf("Allow = %q，缺少 %s（期望 %v）", raw, m, want)
		}
	}
}

// TestRouteTableIntegrity 校验路由表的形状：非空、字段齐全、无重复、登录路由在内。
func TestRouteTableIntegrity(t *testing.T) {
	env := newEnv(t)
	routes := env.srv.routes
	if len(routes) == 0 {
		t.Fatal("路由表为空：registerRoutes 没有累积到任何路由")
	}
	seen := map[string]bool{}
	for _, rt := range routes {
		if rt.method == "" || rt.pattern == "" || rt.handler == nil {
			t.Fatalf("路由字段不完整: %+v", rt)
		}
		if !strings.HasPrefix(rt.pattern, "/") {
			t.Fatalf("路由模式必须以 / 开头: %q", rt.pattern)
		}
		key := rt.method + " " + rt.pattern
		if seen[key] {
			t.Fatalf("路由重复登记: %s", key)
		}
		seen[key] = true
	}
	// 登录 / 登出是后端路由，同样必须由注册产生（不能再有第三份手写清单）。
	for _, key := range []string{"GET " + LoginPath, "POST " + LoginPath, "POST " + logoutPath} {
		if !seen[key] {
			t.Fatalf("路由表缺少 %s", key)
		}
	}
}

// TestRouteWrongVerbReturns405 遍历路由表：每条路由用错误动词打一次，
// 必须 405 且 Allow 恰好是该路径已注册的方法集合。
func TestRouteWrongVerbReturns405(t *testing.T) {
	env := newEnv(t)
	done := map[string]bool{} // 键为「方法 路径」：同一路径多方法各测一次
	for _, rt := range env.srv.routes {
		p := concretePath(t, rt.pattern)
		key := rt.method + " " + p
		if done[key] {
			continue
		}
		done[key] = true
		want := env.srv.allowedMethods(p)
		method := wrongMethod(want)
		if method == "" {
			t.Fatalf("%s 已注册全部动词，无法构造错误动词", p)
		}
		t.Run(method+" "+p, func(t *testing.T) {
			rr := env.do(method, p, nil, nil)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s 状态码 = %d，期望 405，body=%s", method, p, rr.Code, rr.Body.String())
			}
			requireAllow(t, rr.Header().Get("Allow"), want)
		})
	}
}

// TestRouteUnknownPathReturns404 随机路径一律 404 + code=not_found（不能误判成 405）。
func TestRouteUnknownPathReturns404(t *testing.T) {
	env := newEnv(t)
	targets := []string{
		"/api/nope",
		"/api/v2/books",
		"/api/books/1/nope",
		"/api/authors/1/books/2",
		"/api/zzz/qqq/www",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			requireError(t, env.get(target), http.StatusNotFound, codeNotFound)
		})
	}
}

// TestRouteKnown405Fixes 覆盖本次收敛修掉的四个已知判定错误。
func TestRouteKnown405Fixes(t *testing.T) {
	env := newEnv(t)

	// POST /api/recommend：路径只注册了 GET，不能再掉进 404。
	rr := env.do(http.MethodPost, "/api/recommend", nil, nil)
	requireError(t, rr, http.StatusMethodNotAllowed, codeMethodNotAllowed)
	requireAllow(t, rr.Header().Get("Allow"), []string{http.MethodGet})

	// GET /api/events：只注册了 POST。
	rr = env.do(http.MethodGet, "/api/events", nil, nil)
	requireError(t, rr, http.StatusMethodNotAllowed, codeMethodNotAllowed)
	requireAllow(t, rr.Header().Get("Allow"), []string{http.MethodPost})

	// POST /api/books/{id}：Allow 必须列出全部三个已注册动词（旧表只列 GET）。
	rr = env.do(http.MethodPost, "/api/books/1", nil, nil)
	requireError(t, rr, http.StatusMethodNotAllowed, codeMethodNotAllowed)
	requireAllow(t, rr.Header().Get("Allow"),
		[]string{http.MethodGet, http.MethodPatch, http.MethodDelete})

	// PUT /api/books/{id}/chapters：Allow 必须同时列 GET 与 POST（旧表只列 GET）。
	rr = env.do(http.MethodPut, "/api/books/1/chapters", nil, nil)
	requireError(t, rr, http.StatusMethodNotAllowed, codeMethodNotAllowed)
	requireAllow(t, rr.Header().Get("Allow"), []string{http.MethodGet, http.MethodPost})
}
