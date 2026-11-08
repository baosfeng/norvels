package crawl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"norvels/internal/enrich"
	"norvels/internal/source"
)

// gbkSearchPage 是 GBK 编码的固定搜索页（虚构站点，书名/作者都能抽出）。
const gbkSearchPage = "<html><head><meta charset=\"gbk\"></head><body>" +
	"<ul class=\"list\"><li class=\"item\">" +
	"<a class=\"name\" href=\"/book/1\">示例书一</a>" +
	"<span class=\"author\">作者甲</span></li></ul></body></html>"

// requestSeen 记录服务端实际收到的关键请求信息。
type requestSeen struct {
	referer string
	query   string
}

// newGBKSearchServer 起一个只认 Referer、返回 GBK 页面的固定假站。
func newGBKSearchServer(t *testing.T) (*httptest.Server, *requestSeen) {
	t.Helper()
	body := mustGBK(t, gbkSearchPage)
	seen := &requestSeen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.referer, seen.query = r.Header.Get("Referer"), r.URL.RawQuery
		if seen.referer != "http://ref.example/" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=gbk")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

// newRequestLayerEngine 构造一个 header + URL 选项 + GBK 页面齐全的单源执行器。
func newRequestLayerEngine(t *testing.T, srv *httptest.Server) *Engine {
	t.Helper()
	port, p := serverPort(t, srv)
	prof := profileFor("src-a", port)
	prof.Online.Header = map[string]string{"Referer": "http://ref.example/"}
	prof.Online.SearchURL = "http://" + publicIP + ":" + port +
		"/search?q={{key}},{\"charset\":\"gbk\"}"
	prof.Online.RuleSearch = map[string]string{
		"bookList": "class.list@tag.li",
		"name":     "class.name@text",
		"author":   "class.author@text",
		"bookUrl":  "class.name@href | abs()",
	}
	e, err := NewEngine([]source.Profile{prof}, Options{QPS: -1, AllowedPorts: []int{p},
		DialContext: dialerFor(t, srv.Listener.Addr().String())})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// TestSearchBookHeadersOptionsEncoding 端到端覆盖三件事：
// header 生效（无 Referer 403）、URL 选项块被剥离、GBK 页面解码后能抽出书名/作者。
func TestSearchBookHeadersOptionsEncoding(t *testing.T) {
	srv, seen := newGBKSearchServer(t)
	res, err := newRequestLayerEngine(t, srv).SearchBook(context.Background(), "示例书一")
	if err != nil {
		t.Fatal(err)
	}
	requireRequestLayer(t, seen, res.Hits)
}

// requireRequestLayer 断言请求头、被剥离的选项块与解码后的命中字段。
func requireRequestLayer(t *testing.T, seen *requestSeen, hits []enrich.Hit) {
	t.Helper()
	if seen.referer != "http://ref.example/" {
		t.Errorf("header 未随请求发送：Referer=%q", seen.referer)
	}
	if strings.Contains(seen.query, "charset") || strings.Contains(seen.query, "%2C") {
		t.Errorf("URL 选项块应被剥离，实际 query=%q", seen.query)
	}
	if len(hits) != 1 {
		t.Fatalf("应命中 1 条，实际 %d：%+v", len(hits), hits)
	}
	h := hits[0]
	if h.Title != "示例书一" || h.Author != "作者甲" {
		t.Errorf("GBK 页面解码后字段不完整: %+v", h)
	}
	if !strings.HasSuffix(h.BookURL, "/book/1") {
		t.Errorf("bookUrl 应解析成绝对地址，实际 %q", h.BookURL)
	}
}

// TestSearchBookURLUnsupportedOption 未实现的选项键按降级回显，不影响检索。
func TestSearchBookURLUnsupportedOption(t *testing.T) {
	srv, _ := newSearchServer(t, sampleHTML)
	port, p := serverPort(t, srv)
	prof := profileFor("src-a", port)
	prof.Online.SearchURL = "http://" + publicIP + ":" + port +
		"/search?q={{key}},{\"webView\":true}"
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
		t.Fatalf("未实现的选项不该影响检索，实际命中 %d", len(res.Hits))
	}
	if !strings.Contains(strings.Join(res.Degraded, "；"), "urlOptions.webView") {
		t.Errorf("应回显未实现的选项，实际 degraded=%v", res.Degraded)
	}
}
