package api

import (
	"net/http"
	"sort"
	"strings"

	"norvels/internal/observe"
)

// route 是一条后端路由：方法 + 路径模式 + 处理函数。
//
// 它同时承担两件事，这也是本文件存在的理由：
//   - 注册：newServer 调 registerRoutes 把它挂到 http.ServeMux 上；
//   - 405 判定：registerRoutes 累积出的表就是「这个路径存在、方法对不对」的唯一事实源。
//
// 只写一份而不是「注册一遍、再为 405 手写一份清单」：两份必然会脱钩，
// 脱钩的直接后果是路径存在却没判成 405（掉 404），前端「动词用错」的分支随之失效。
type route struct {
	method  string
	pattern string
	handler http.HandlerFunc
}

// newRoute 构造一条路由（构造器写法让各领域函数里的字面量保持一行一条）。
func newRoute(method, pattern string, handler http.HandlerFunc) route {
	return route{method: method, pattern: pattern, handler: handler}
}

// registerRoutes 把五组领域路由统一注册到 mux，并返回累积成的路由表。
//
// 累积与注册是同一个循环、同一份数据：不存在「注册了但没进表」或「进了表但没注册」。
func registerRoutes(mux *http.ServeMux, s *Server) []route {
	groups := [][]route{
		s.bookRoutes(),
		s.libraryRoutes(),
		s.importRoutes(),
		s.miscRoutes(),
		s.recommendRoutes(),
	}
	var all []route
	for _, group := range groups {
		for _, rt := range group {
			// 观测标签是「方法 + 模板」（如 "GET /api/books/{id}"）：按真实路径聚合会被书籍 id
			// 撑成上万条序列，模板才能给出「哪一类接口慢」。
			label := rt.method + " " + rt.pattern
			mux.HandleFunc(label, observe.Wrap(label, rt.handler))
			all = append(all, rt)
		}
	}
	return all
}

// handleAPINotFound 处理未命中的 /api 请求：路径在表里只是方法不对 → 405 + Allow，
// 否则 404。全表遍历交给 allowedMethods，这里只做分流（保持单函数短小）。
func (s *Server) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	if allow := s.allowedMethods(r.URL.Path); len(allow) > 0 {
		w.Header().Set("Allow", strings.Join(allow, ", "))
		writeError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed, "不支持的请求方法")
		return
	}
	writeError(w, http.StatusNotFound, codeNotFound, "接口不存在")
}

// allowedMethods 返回该路径已注册的方法（路径不存在时返回 nil）。
//
// 去重并按字母序输出：Allow 头是给人读的，顺序稳定才好写断言；
// 同一路径多方法只出现一次（表里本来就可能有多条同路径不同方法的记录）。
func (s *Server) allowedMethods(p string) []string {
	var out []string
	seen := make(map[string]bool, 2)
	for _, rt := range s.routes {
		if seen[rt.method] || !routePathMatch(rt.pattern, p) {
			continue
		}
		seen[rt.method] = true
		out = append(out, rt.method)
	}
	sort.Strings(out)
	return out
}

// routePathMatch 按段比较路由模式与路径，{xxx} 匹配任意一段。
//
// 语义与 Go 1.22 ServeMux 的 {id} 占位段一致：只比段数与非占位段，占位段不校验取值。
func routePathMatch(pattern, p string) bool {
	ps := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	xs := strings.Split(strings.TrimPrefix(p, "/"), "/")
	if len(ps) != len(xs) {
		return false
	}
	for i := range ps {
		if strings.HasPrefix(ps[i], "{") && strings.HasSuffix(ps[i], "}") {
			continue
		}
		if ps[i] != xs[i] {
			return false
		}
	}
	return true
}
