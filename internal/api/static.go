package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// staticAsset 是缓存后的静态资源摘要。
type staticAsset struct {
	name         string
	data         []byte
	etag         string
	contentType  string
	cacheControl string
}

// assetExts 是「资源后缀」白名单：命中但不存在时返回 404，而不是退回 index.html。
var assetExts = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".map": true, ".json": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".ico": true, ".webp": true, ".avif": true, ".woff": true, ".woff2": true,
	".ttf": true, ".otf": true, ".eot": true, ".txt": true, ".xml": true,
	".webmanifest": true, ".wasm": true, ".pdf": true, ".mp4": true, ".webm": true,
}

func isAssetPath(name string) bool {
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, "assets/") {
		return true
	}
	return assetExts[strings.ToLower(path.Ext(name))]
}

// serveStatic 提供内嵌前端资源；未命中的非资源路径回退到 index.html（SPA）。
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed, "只支持 GET/HEAD")
		return
	}
	name := cleanStaticName(r.URL.Path)
	if asset, ok := s.lookupStatic(name); ok {
		s.writeStatic(w, r, asset)
		return
	}
	if isAssetPath(name) {
		writeError(w, http.StatusNotFound, codeNotFound, "资源不存在")
		return
	}
	if index, ok := s.lookupStatic("index.html"); ok {
		s.writeStatic(w, r, index)
		return
	}
	// 前端未构建：返回内嵌占位页，避免 500。
	s.writePlaceholder(w, r)
}

// cleanStaticName 把 URL 路径规范化为 FS 内路径。
func cleanStaticName(p string) string {
	name := strings.TrimPrefix(path.Clean("/"+p), "/")
	if name == "." {
		return ""
	}
	return name
}

// lookupStatic 读取并缓存静态资源；不存在时做负缓存。
func (s *Server) lookupStatic(name string) (*staticAsset, bool) {
	key := name
	if key == "" {
		key = "index.html"
	}
	if v, ok := s.staticCache.Load(key); ok {
		asset, _ := v.(*staticAsset)
		return asset, asset != nil
	}
	asset, err := s.readStatic(key)
	if err != nil {
		s.staticCache.Store(key, (*staticAsset)(nil))
		return nil, false
	}
	s.staticCache.Store(key, asset)
	return asset, true
}

// readStatic 读取单个文件并生成 ETag/Content-Type/缓存策略。
func (s *Server) readStatic(name string) (*staticAsset, error) {
	if s.fsys == nil {
		return nil, fs.ErrNotExist
	}
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) {
		return nil, fs.ErrNotExist
	}
	f, err := s.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return s.readStatic(path.Join(name, "index.html"))
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return newStaticAsset(name, data), nil
}

// newStaticAsset 构造资源摘要。
func newStaticAsset(name string, data []byte) *staticAsset {
	sum := sha256.Sum256(data)
	return &staticAsset{
		name:         name,
		data:         data,
		etag:         "\"" + hex.EncodeToString(sum[:8]) + "\"",
		contentType:  contentTypeFor(name),
		cacheControl: cacheControlFor(name),
	}
}

func contentTypeFor(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	switch ext {
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".json", ".map", ".webmanifest":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func cacheControlFor(name string) string {
	switch {
	case name == "index.html":
		return "no-cache"
	case strings.HasPrefix(name, "assets/"):
		return "public, max-age=31536000, immutable"
	default:
		return "public, max-age=3600"
	}
}

// writeStatic 输出静态资源（ETag/304/Last-Modified 交给 ServeContent）。
func (s *Server) writeStatic(w http.ResponseWriter, r *http.Request, asset *staticAsset) {
	h := w.Header()
	h.Set("ETag", asset.etag)
	h.Set("Cache-Control", asset.cacheControl)
	h.Set("Content-Type", asset.contentType)
	h.Set("X-Content-Type-Options", "nosniff")
	if etagMatch(r.Header.Get("If-None-Match"), asset.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(w, r, asset.name, time.Time{}, bytes.NewReader(asset.data))
}

// writePlaceholder 输出前端未构建时的占位页。
func (s *Server) writePlaceholder(w http.ResponseWriter, r *http.Request) {
	data := s.placeholder
	if len(data) == 0 {
		writeError(w, http.StatusNotFound, codeNotFound, "前端资源未构建")
		return
	}
	asset := newStaticAsset("index.html", data)
	asset.cacheControl = "no-cache"
	s.writeStatic(w, r, asset)
}

// etagMatch 判断 If-None-Match 是否命中（支持 * 与弱校验前缀）。
func etagMatch(ifNoneMatch, etag string) bool {
	raw := strings.TrimSpace(ifNoneMatch)
	if raw == "" {
		return false
	}
	if raw == "*" {
		return true
	}
	for _, part := range strings.Split(raw, ",") {
		tag := strings.TrimSpace(part)
		if strings.HasPrefix(tag, "W/") {
			tag = strings.TrimSpace(strings.TrimPrefix(tag, "W/"))
		}
		if tag == etag {
			return true
		}
	}
	return false
}

// apiRoutes 是 API 路由表，用于把「路径存在但方法不对」判成 405。
var apiRoutes = []struct {
	pattern string
	method  string
}{
	{"/api/health", http.MethodGet},
	{"/api/stats", http.MethodGet},
	{"/api/books", http.MethodGet},
	{"/api/books/{id}", http.MethodGet},
	{"/api/books/{id}/chapters", http.MethodGet},
	{"/api/books/{id}/chapters/{cid}", http.MethodGet},
	{"/api/authors", http.MethodGet},
	{"/api/authors/{id}", http.MethodGet},
	{"/api/search", http.MethodGet},
	{"/api/tags", http.MethodGet},
	{"/api/import", http.MethodPost},
	{"/api/import/jobs", http.MethodGet},
	{"/api/import/jobs/{id}", http.MethodGet},
	{"/api/version", http.MethodGet},
}

// handleAPINotFound 处理未命中的 /api 请求，统一返回 JSON 错误。
func (s *Server) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	if allow := allowedMethods(r.URL.Path); len(allow) > 0 {
		w.Header().Set("Allow", strings.Join(allow, ", "))
		writeError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed, "不支持的请求方法")
		return
	}
	writeError(w, http.StatusNotFound, codeNotFound, "接口不存在")
}

// allowedMethods 返回该路径支持的方法（路径不存在时为空）。
func allowedMethods(p string) []string {
	var out []string
	for _, route := range apiRoutes {
		if routePathMatch(route.pattern, p) {
			out = append(out, route.method)
		}
	}
	return out
}

// routePathMatch 按段比较路由模式与路径，{xxx} 匹配任意一段。
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
