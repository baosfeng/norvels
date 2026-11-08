package crawl

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"norvels/internal/source"
)

// publicIP 是 IANA 保留的示例地址：fetch 的 SSRF 校验看到公网，实际连接走注入的 dialer
// （与 internal/fetch 的测试同一套办法；直接写 127.0.0.1 会被安全策略拒绝）。
const publicIP = "198.51.100.7"

// newSearchServer 起一个本地搜索页服务，返回响应体与请求计数指针。
func newSearchServer(t *testing.T, html string) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// dialerFor 把「公网 IP:端口」路由到本地测试服务，避免任何真实外网请求。
func dialerFor(t *testing.T, target string) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, target)
	}
}

// serverPort 取 httptest 的监听端口（int 与字符串两种形态都要用）。
func serverPort(t *testing.T, srv *httptest.Server) (string, int) {
	t.Helper()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port, atoiPort(t, port)
}

// profileFor 构造一份指向本地测试服务的在线书源。
func profileFor(id, port string) source.Profile {
	base := "http://" + publicIP + ":" + port
	return source.Profile{
		ID: id, Name: id, SchemaVersion: source.SchemaVersion, Enabled: true, Priority: 10,
		Kind: source.KindOnline,
		Online: &source.Online{
			BookSourceURL: base,
			SearchURL:     base + "/search?q={{key}}&page={{page}}",
			RuleSearch: map[string]string{
				"bookList": ".item",
				"name":     ".name@text",
				"author":   ".author@text",
				"intro":    ".intro@text",
				"kind":     ".kind@text",
				"bookUrl":  ".name@href | abs()",
			},
		},
	}
}

// engineFor 用本地服务与单个书源构造执行器。
func engineFor(t *testing.T, srv *httptest.Server, opt Options) *Engine {
	t.Helper()
	port, p := serverPort(t, srv)
	opt.AllowedPorts = []int{p}
	opt.DialContext = dialerFor(t, srv.Listener.Addr().String())
	e, err := NewEngine([]source.Profile{profileFor("src-a", port)}, opt)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// atoiPort 把端口串转成 int（测试助手，避免引入 strconv 只为一行）。
func atoiPort(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// sampleHTML 是两个条目的搜索结果：一条标题相似、一条完全不相似。
const sampleHTML = `<ul class="list">
  <li class="item"><a class="name" href="/book/1">示例书一</a><span class="author">作者甲</span>
    <span class="intro">这是一段足够长的示例简介，用来验证简介字段能被正确取出并参与投票。</span>
    <span class="kind">玄幻/东方玄幻</span></li>
  <li class="item"><a class="name" href="/book/2">完全无关的另一本</a><span class="author">作者乙</span></li>
</ul>`

// TestSearchBook 覆盖主路径：命中解析、字段抽取、不相似的书被丢弃。
func TestSearchBook(t *testing.T) {
	srv, _ := newSearchServer(t, sampleHTML)
	e := engineFor(t, srv, Options{QPS: -1})
	res, err := e.SearchBook(context.Background(), "示例书一")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("相似度过滤后应只剩 1 条命中，实际 %d", len(res.Hits))
	}
	h := res.Hits[0]
	if h.Author != "作者甲" || h.Kind != "玄幻/东方玄幻" {
		t.Errorf("字段抽取不完整: %+v", h)
	}
	if !strings.HasSuffix(h.BookURL, "/book/1") {
		t.Errorf("bookUrl 应解析成绝对地址，实际 %q", h.BookURL)
	}
	if res.Requests != 1 {
		t.Errorf("应只发 1 次请求，实际 %d", res.Requests)
	}
}

// TestSearchBookDegraded 覆盖 @js 规则：降级为空并点名，不影响其它字段。
func TestSearchBookDegraded(t *testing.T) {
	srv, _ := newSearchServer(t, sampleHTML)
	port, p := serverPort(t, srv)
	prof := profileFor("src-a", port)
	prof.Online.RuleSearch["kind"] = "@js:java.getString()"
	e, err := NewEngine([]source.Profile{prof}, Options{QPS: -1, AllowedPorts: []int{p},
		DialContext: dialerFor(t, srv.Listener.Addr().String())})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.SearchBook(context.Background(), "示例书一")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Degraded) == 0 || !strings.Contains(res.Degraded[0], "ruleSearch.kind") {
		t.Errorf("@js 字段应被点名，实际 %v", res.Degraded)
	}
	if len(res.Hits) != 1 || res.Hits[0].Author != "作者甲" {
		t.Errorf("非 @js 字段应照常取到: %+v", res.Hits)
	}
}

// TestSearchBookQPS 覆盖单源限速：QPS=5 时 3 次请求至少要跨过 2 个 200ms 窗口。
func TestSearchBookQPS(t *testing.T) {
	srv, _ := newSearchServer(t, sampleHTML)
	e := engineFor(t, srv, Options{QPS: 5, MaxSources: 1, StopAfterHits: 5})
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := e.SearchBook(context.Background(), "示例书一"); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Fatalf("3 次请求在 QPS=5 下不该这么快，实测 %v", elapsed)
	}
}

// TestSearchBookBudget 覆盖请求数预算：打满后不再发请求，也不报错。
func TestSearchBookBudget(t *testing.T) {
	srv, hits := newSearchServer(t, sampleHTML)
	e := engineFor(t, srv, Options{QPS: -1, MaxRequests: 1, MaxSources: 1})
	for i := 0; i < 3; i++ {
		if _, err := e.SearchBook(context.Background(), "示例书一"); err != nil {
			t.Fatalf("预算打满不该报错，实际 %v", err)
		}
	}
	if *hits > 1 {
		t.Fatalf("预算 1 次请求，实际发了 %d 次", *hits)
	}
	if stats := e.Stats(); stats.Requests < 1 {
		t.Errorf("Stats().Requests 应 ≥ 1，实际 %d", stats.Requests)
	}
}

// TestSearchBookBreaker 覆盖熔断：连续失败达到阈值后不再请求该源。
func TestSearchBookBreaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	e := engineFor(t, srv, Options{QPS: -1, MaxSources: 1})
	for i := 0; i < breakerThreshold+2; i++ {
		_, _ = e.SearchBook(context.Background(), "示例书一")
	}
	if tripped := e.Stats().Tripped; len(tripped) != 1 {
		t.Fatalf("连续失败应熔断该源，实际 %v", tripped)
	}
}

// TestNewEngineSkipsDisabled 覆盖书源过滤：停用或非 online 的规则包不参与检索。
func TestNewEngineSkipsDisabled(t *testing.T) {
	off := profileFor("src-off", "80")
	off.Enabled = false
	local := source.Profile{ID: "src-local", Enabled: true, Kind: source.KindLocal}
	e, err := NewEngine([]source.Profile{off, local}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Sources(); len(got) != 0 {
		t.Fatalf("停用/本地规则包不该参与检索，实际 %v", got)
	}
}

// TestSearchBookNoSource 覆盖「没有任何在线书源」。
func TestSearchBookNoSource(t *testing.T) {
	e, err := NewEngine(nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.SearchBook(context.Background(), "示例书一"); err == nil {
		t.Fatal("没有书源时应报 ErrNoSource")
	}
}

// TestExpandSearchURL 覆盖 URL 模板的两个变量。
func TestExpandSearchURL(t *testing.T) {
	got := expandSearchURL("http://x/s?q={{key}}&p={{page}}", "示例 书")
	if !strings.Contains(got, "p=1") || strings.Contains(got, "{{") {
		t.Fatalf("变量未展开: %q", got)
	}
	if !strings.Contains(got, "%E7%A4%BA%E4%BE%8B") {
		t.Errorf("关键词应 URL 转义，实际 %q", got)
	}
	if !strings.Contains(got, "http://x/s") {
		t.Errorf("主机与路径不该被改动: %q", got)
	}
}
