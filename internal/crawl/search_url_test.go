package crawl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"norvels/internal/source"
)

// TestResolveSearchURL 覆盖：相对路径 + base、绝对 URL 放行、base 带 fragment、
// base 非法、相对路径带查询串。
func TestResolveSearchURL(t *testing.T) {
	cases := []struct {
		name, base, raw, want string
		warn                  bool
	}{
		{"相对路径 + base", "https://www.example.invalid", "/search?query=x",
			"https://www.example.invalid/search?query=x", false},
		{"无前导斜杠的相对路径", "https://www.example.invalid/sub/", "search?q=1",
			"https://www.example.invalid/sub/search?q=1", false},
		{"绝对 URL 放行", "https://www.example.invalid", "https://other.example/search?q=1",
			"https://other.example/search?q=1", false},
		{"base 带 fragment", "https://www.example.invalid##类别", "/s?q=1",
			"https://www.example.invalid/s?q=1", false},
		{"base 非法", "not-a-url", "/s?q=1", "/s?q=1", true},
		{"相对路径带查询串", "https://www.example.invalid", "/search?query=%E7%9B%98%E9%BE%99&page=1",
			"https://www.example.invalid/search?query=%E7%9B%98%E9%BE%99&page=1", false},
		{"协议相对", "https://www.example.invalid", "//cdn.example.invalid/s?q=1",
			"https://cdn.example.invalid/s?q=1", false},
		{"空地址", "https://www.example.invalid", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, warning := resolveSearchURL(c.base, c.raw)
			if got != c.want {
				t.Fatalf("resolveSearchURL(%q, %q) = %q，期望 %q", c.base, c.raw, got, c.want)
			}
			if (warning != "") != c.warn {
				t.Fatalf("warning = %q，期望有降级=%v", warning, c.warn)
			}
		})
	}
}

// TestPrepareSearchURL 展开变量、拆选项块、拼 base 三件事一起做。
func TestPrepareSearchURL(t *testing.T) {
	prof := source.Profile{ID: "src-a", Online: &source.Online{
		BookSourceURL: "https://www.example.invalid##类别",
		SearchURL:     "/search?q={{key}},{\"charset\":\"gbk\",\"webView\":true}",
	}}
	got, opts, degraded := prepareSearchURL(prof, "盘龙")
	if got != "https://www.example.invalid/search?q=%E7%9B%98%E9%BE%99" {
		t.Fatalf("URL = %q", got)
	}
	if opts.Charset != "gbk" {
		t.Errorf("charset = %q，期望 gbk", opts.Charset)
	}
	if len(degraded) != 1 || !strings.Contains(degraded[0], "urlOptions.webView") {
		t.Errorf("degraded = %v，期望回显未实现的选项", degraded)
	}
}

// TestPrepareSearchURLInvalidBase base 不可用时保留原样并记降级，不抛错。
func TestPrepareSearchURLInvalidBase(t *testing.T) {
	prof := source.Profile{ID: "src-a", Online: &source.Online{
		BookSourceURL: "not-a-url", SearchURL: "/s?q={{key}}",
	}}
	got, _, degraded := prepareSearchURL(prof, "盘龙")
	if got != "/s?q=%E7%9B%98%E9%BE%99" {
		t.Errorf("URL = %q，期望保留原样", got)
	}
	if len(degraded) != 1 || !strings.Contains(degraded[0], "无法作为 base") {
		t.Errorf("degraded = %v，期望说明 base 不可用", degraded)
	}
}

// TestRelativeSearchURLRequestsBase 端到端：相对 searchUrl 实际请求到 base 主机与路径，
// Probe 与 enrich 两条路径都走同一套拼装。
func TestRelativeSearchURLRequestsBase(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(sampleHTML))
	}))
	t.Cleanup(srv.Close)
	port, p := serverPort(t, srv)
	prof := profileFor("src-a", port)
	prof.Online.SearchURL = "/search?q={{key}}"
	e, err := NewEngine([]source.Profile{prof}, Options{QPS: -1, AllowedPorts: []int{p},
		DialContext: dialerFor(t, srv.Listener.Addr().String())})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.SearchBook(context.Background(), "示例书一")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("命中 = %d，期望 1", len(res.Hits))
	}
	if gotPath != "/search" || !strings.Contains(gotQuery, "q=") {
		t.Errorf("请求未拼到 base 上：path=%q query=%q", gotPath, gotQuery)
	}

	probe, err := e.Probe(context.Background(), "src-a", "示例书一")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	wantPrefix := "http://" + publicIP + ":" + port + "/search?q="
	if !strings.HasPrefix(probe.URL, wantPrefix) || probe.Status != 200 {
		t.Errorf("Probe URL/状态不对：%q / %d", probe.URL, probe.Status)
	}
}

// TestBaseURLFragmentUsedByAbs baseUrl 带 ##类别 时，abs() 拼出的书籍链接不能带 fragment。
func TestBaseURLFragmentUsedByAbs(t *testing.T) {
	srv, _ := newSearchServer(t, sampleHTML)
	port, p := serverPort(t, srv)
	base := "http://" + publicIP + ":" + port
	prof := profileFor("src-a", port)
	prof.Online.BookSourceURL = base + "##类别"
	prof.Online.SearchURL = "/search?q={{key}}"
	e, err := NewEngine([]source.Profile{prof}, Options{QPS: -1, AllowedPorts: []int{p},
		DialContext: dialerFor(t, srv.Listener.Addr().String())})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.SearchBook(context.Background(), "示例书一")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("命中 = %d，期望 1", len(res.Hits))
	}
	want := base + "/book/1"
	if got := res.Hits[0].BookURL; got != want {
		t.Errorf("bookUrl = %q，期望 %q（fragment 不应进链接）", got, want)
	}
}
