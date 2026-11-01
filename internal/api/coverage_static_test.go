package api

// static.go 的边界路径：缓存命中/负缓存、目录回退、非法路径、
// MIME 兜底表与 If-None-Match 解析。都是纯函数或单层分支，直接打比绕 HTTP 更准。

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// TestCleanStaticName 路径规范化：根路径归一成空串，多余分隔符被 Clean 掉。
func TestCleanStaticName(t *testing.T) {
	cases := map[string]string{
		"/":              "",
		"":               "",
		"/assets/app.js": "assets/app.js",
		"//a//b//":       "a/b",
		"/../etc/passwd": "etc/passwd", // path.Clean 把 .. 收敛在根内
	}
	for in, want := range cases {
		if got := cleanStaticName(in); got != want {
			t.Errorf("cleanStaticName(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestReadStaticInvalidName fs.ValidPath 拒绝的名字直接当不存在（不 panic、不越界）。
func TestReadStaticInvalidName(t *testing.T) {
	env := newEnv(t)
	for _, name := range []string{"../secret", "a/../../b"} {
		if _, err := env.srv.readStatic(name); err == nil {
			t.Errorf("readStatic(%q) 应当报错", name)
		}
	}
}

// TestReadStaticNilFS 没有前端产物（fsys=nil）时退回「不存在」而不是 panic。
func TestReadStaticNilFS(t *testing.T) {
	env := newEnv(t)
	env.srv.fsys = nil
	if _, err := env.srv.readStatic(""); err == nil {
		t.Error("fsys 为 nil 时 readStatic 应返回错误")
	}
}

// TestLookupStaticNegativeCache 不存在的资源做负缓存：第二次仍判不存在，且不再读 FS。
func TestLookupStaticNegativeCache(t *testing.T) {
	env := newEnv(t)
	if _, ok := env.srv.lookupStatic("assets/missing.js"); ok {
		t.Fatal("缺失资源不应命中")
	}
	// 负缓存生效的判据：直接把 FS 换掉，仍然走缓存判不存在。
	env.srv.fsys = fstest.MapFS{"assets/missing.js": {Data: []byte("late")}}
	if _, ok := env.srv.lookupStatic("assets/missing.js"); ok {
		t.Error("负缓存应命中：不该因为 FS 变化就重新读出内容")
	}
}

// TestReadStaticDirectoryFallback 目录请求回退到该目录下的 index.html。
func TestReadStaticDirectoryFallback(t *testing.T) {
	env := newEnvWith(t, Config{Version: "slim", DisableAccessLog: true}, fstest.MapFS{
		"docs/index.html": {Data: []byte("<html>docs</html>")},
	})
	asset, err := env.srv.readStatic("docs")
	if err != nil {
		t.Fatalf("readStatic(docs): %v", err)
	}
	if !strings.Contains(string(asset.data), "docs") {
		t.Errorf("目录回退内容 = %q", string(asset.data))
	}
}

// brokenFS 在 Open 之后让 Stat 失败，用来覆盖 readStatic 的 Stat 错误分支。
type brokenFS struct{}

func (brokenFS) Open(name string) (fs.File, error) { return brokenFile{}, nil }

type brokenFile struct{}

func (brokenFile) Stat() (fs.FileInfo, error) { return nil, fs.ErrInvalid }
func (brokenFile) Read([]byte) (int, error)   { return 0, fs.ErrInvalid }
func (brokenFile) Close() error               { return nil }

func TestReadStaticStatError(t *testing.T) {
	env := newEnvWith(t, Config{Version: "slim", DisableAccessLog: true}, brokenFS{})
	if _, err := env.srv.readStatic("index.html"); err == nil {
		t.Error("Stat 失败时 readStatic 应返回错误")
	}
}

// TestContentTypeForFallback mime 包认不出时按后缀兜底（含默认二进制流）。
func TestContentTypeForFallback(t *testing.T) {
	// 前几项 mime 包自己认得（走的是 mime 分支，不是下面的 switch）；
	// .json/.map/.webmanifest 在不同平台上可能认得也可能认不得，因此只断言「以 json 开头」。
	cases := map[string]string{
		"a.js":         "text/javascript; charset=utf-8",
		"a.mjs":        "text/javascript; charset=utf-8",
		"a.css":        "text/css; charset=utf-8",
		"a.html":       "text/html; charset=utf-8",
		"a.svg":        "image/svg+xml",
		"a.woff2":      "font/woff2",
		"a.woff":       "font/woff",
		"a.txt":        "text/plain; charset=utf-8",
		"a.unknownext": "application/octet-stream",
	}
	for name, want := range cases {
		if got := contentTypeFor(name); got != want {
			t.Errorf("contentTypeFor(%q) = %q，期望 %q", name, got, want)
		}
	}
	// 以 json 结尾的后缀在两条路径下都该是 JSON：mime 认得就原样返回，
	// 认不得则落到 switch 的 .json/.map/.webmanifest 分支。
	for _, name := range []string{"a.json", "a.map", "a.webmanifest"} {
		if got := contentTypeFor(name); !strings.HasPrefix(got, "application/json") {
			t.Errorf("contentTypeFor(%q) = %q，期望 application/json*", name, got)
		}
	}
}

// TestCacheControlFor 三类缓存策略：index 不缓存、assets 长缓存、其余短缓存。
func TestCacheControlFor(t *testing.T) {
	cases := map[string]string{
		"index.html":  "no-cache",
		"assets/x.js": "public, max-age=31536000, immutable",
		"favicon.ico": "public, max-age=3600",
	}
	for name, want := range cases {
		if got := cacheControlFor(name); got != want {
			t.Errorf("cacheControlFor(%q) = %q，期望 %q", name, got, want)
		}
	}
}

// TestEtagMatchEdgeCases If-None-Match 的 * / 弱校验 / 多值 / 不匹配。
func TestEtagMatchEdgeCases(t *testing.T) {
	tag := "\"abc\""
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"*", true},
		{"\"abc\"", true},
		{"W/\"abc\"", true},
		{"\"x\", W/\"abc\", \"y\"", true},
		{"\"nope\"", false},
	}
	for _, c := range cases {
		if got := etagMatch(c.header, tag); got != c.want {
			t.Errorf("etagMatch(%q) = %v，期望 %v", c.header, got, c.want)
		}
	}
}

// TestWritePlaceholderWithoutData 前端未构建且没有占位页时返回 404 而不是空 200。
func TestWritePlaceholderWithoutData(t *testing.T) {
	env := newEnv(t)
	env.srv.placeholder = nil
	rr := httptest.NewRecorder()
	env.srv.writePlaceholder(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	requireError(t, rr, http.StatusNotFound, codeNotFound)
}

// TestServeStaticDirRequest 目录路径的静态请求回退到目录下的 index.html。
func TestServeStaticDirRequest(t *testing.T) {
	env := newEnvWith(t, Config{Version: "slim", DisableAccessLog: true}, fstest.MapFS{
		"index.html":      {Data: []byte("<html>root</html>")},
		"docs/index.html": {Data: []byte("<html>docs</html>")},
	})
	rr := env.get("/docs/")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /docs/ = %d，期望 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "docs") {
		t.Errorf("正文 = %q，期望 docs 目录的 index.html", rr.Body.String())
	}
}
