package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"norvels/internal/crawl"
	"norvels/internal/source"
)

// probePublicIP 是 IANA 保留的示例地址：SSRF 校验看到公网，实际连接走注入 dialer。
const probePublicIP = "198.51.100.7"

// probeHTML 是单条搜索结果的固定样例页（虚构站点）。
const probeHTML = "<ul class=\"list\"><li class=\"item\">" +
	"<a class=\"name\" href=\"/book/1\">示例书一</a>" +
	"<span class=\"author\">作者甲</span></li></ul>"

// probeFixture 起一个只返回固定页的假站，并返回「书源 + 抓取参数」。
func probeFixture(t *testing.T, status int) (source.Profile, crawl.Options) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(probeHTML))
	}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("取端口失败: %v", err)
	}
	base := "http://" + probePublicIP + ":" + port
	prof := source.Profile{
		ID: "src-probe", Name: "示例书源", SchemaVersion: source.SchemaVersion,
		Enabled: true, Kind: source.KindOnline,
		Online: &source.Online{
			BookSourceURL: base,
			SearchURL:     base + "/search?q={{key}}",
			RuleSearch: map[string]string{
				"bookList": ".item", "name": ".name@text", "author": ".author@text",
			},
		},
	}
	opt := crawl.Options{QPS: -1, AllowedPorts: []int{portInt(t, port)},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, srv.Listener.Addr().String())
		}}
	return prof, opt
}

// portInt 把端口串转 int（避免只为一行引入 strconv）。
func portInt(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			t.Fatalf("端口 %q 不是数字", s)
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// localProfile 造一个不参与检索的本地规则包。
func localProfile(id string) source.Profile {
	return source.Profile{ID: id, Name: "本地规则", Kind: source.KindLocal}
}

// TestCmdSourceUsage 覆盖入口参数校验：缺子命令或未知子命令都给出用法。
func TestCmdSourceUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"bogus"}} {
		err := cmdSource(args)
		if err == nil || !strings.Contains(err.Error(), "用法") {
			t.Errorf("cmdSource(%v) = %v，期望用法错误", args, err)
		}
	}
}

// TestSourceTestRejectsBadInput 覆盖参数校验、找不到 id 与无书源。
func TestSourceTestRejectsBadInput(t *testing.T) {
	online := []source.Profile{probeProfileStub()}
	cases := []struct {
		name     string
		profiles []source.Profile
		opts     sourceTestOpts
		want     string
	}{
		{"无在线书源", []source.Profile{localProfile("local-1")}, sourceTestOpts{out: &bytes.Buffer{}}, "没有在线书源"},
		{"有 id 缺 title", online, sourceTestOpts{out: &bytes.Buffer{}, id: "src-probe"}, "--title"},
		{"找不到 id", online, sourceTestOpts{out: &bytes.Buffer{}, id: "src-nope", title: "示例书一"}, "找不到书源"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := runSourceTest(context.Background(), c.profiles, c.opts)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v，期望包含 %q", err, c.want)
			}
		})
	}
}

// probeProfileStub 是只用于参数校验的在线书源（不发请求）。
func probeProfileStub() source.Profile {
	return source.Profile{ID: "src-probe", Name: "示例书源", Kind: source.KindOnline,
		Online: &source.Online{BookSourceURL: "https://example.invalid/", SearchURL: "https://example.invalid/s?q={{key}}"}}
}

// TestSourceTestListsSources 覆盖「不带 -id 列出书源」的文本与 JSON 输出。
func TestSourceTestListsSources(t *testing.T) {
	profiles := []source.Profile{probeProfileStub(), localProfile("local-1")}
	var text bytes.Buffer
	if err := runSourceTest(context.Background(), profiles, sourceTestOpts{out: &text}); err != nil {
		t.Fatalf("runSourceTest: %v", err)
	}
	if !strings.Contains(text.String(), "src-probe") || strings.Contains(text.String(), "local-1") {
		t.Errorf("文本输出应只列在线书源，实际 %q", text.String())
	}

	var js bytes.Buffer
	if err := runSourceTest(context.Background(), profiles, sourceTestOpts{out: &js, jsonOut: true}); err != nil {
		t.Fatalf("runSourceTest(json): %v", err)
	}
	var got struct {
		Sources []struct {
			ID string `json:"id"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(js.Bytes(), &got); err != nil {
		t.Fatalf("JSON 解析失败: %v（%q）", err, js.String())
	}
	if len(got.Sources) != 1 || got.Sources[0].ID != "src-probe" {
		t.Fatalf("JSON sources = %+v，期望只有 src-probe", got.Sources)
	}
}

// TestSourceTestProbeSuccess 覆盖成功路径：报告含 URL/状态/耗时/候选（书名/作者/相似度）。
func TestSourceTestProbeSuccess(t *testing.T) {
	prof, opt := probeFixture(t, http.StatusOK)
	var text bytes.Buffer
	o := sourceTestOpts{out: &text, id: prof.ID, title: "示例书一", options: opt}
	if err := runSourceTest(context.Background(), []source.Profile{prof}, o); err != nil {
		t.Fatalf("runSourceTest: %v", err)
	}
	requireProbeReport(t, text.String(), prof.ID, "示例书一", "作者甲")

	o.jsonOut = true
	var js bytes.Buffer
	o.out = &js
	if err := runSourceTest(context.Background(), []source.Profile{prof}, o); err != nil {
		t.Fatalf("runSourceTest(json): %v", err)
	}
	requireProbeJSON(t, js.Bytes(), prof.ID)
}

// requireProbeReport 断言人读报告的关键行。
func requireProbeReport(t *testing.T, out, id, title, author string) {
	t.Helper()
	for _, part := range []string{id, "HTTP 200", "候选  1 条", title + " / " + author, "/search?q="} {
		if !strings.Contains(out, part) {
			t.Errorf("报告缺少 %q：\n%s", part, out)
		}
	}
	if strings.Contains(out, "%7B") {
		t.Errorf("URL 选项块应被剥离：\n%s", out)
	}
}

// requireProbeJSON 断言 JSON 结果可解析且字段齐全。
func requireProbeJSON(t *testing.T, raw []byte, id string) {
	t.Helper()
	var got struct {
		SourceID   string `json:"sourceId"`
		URL        string `json:"url"`
		Status     int    `json:"status"`
		ElapsedMS  int64  `json:"elapsedMs"`
		Candidates []struct {
			Title      string  `json:"title"`
			Author     string  `json:"author"`
			BookURL    string  `json:"bookUrl"`
			Similarity float64 `json:"similarity"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("JSON 解析失败: %v（%q）", err, raw)
	}
	if got.SourceID != id || got.Status != 200 || len(got.Candidates) != 1 {
		t.Fatalf("JSON 结果不完整: %+v", got)
	}
	c := got.Candidates[0]
	if c.Title != "示例书一" || c.Author != "作者甲" || c.Similarity < 0.99 {
		t.Errorf("候选字段不对: %+v", c)
	}
	if !strings.Contains(got.URL, "/search?q=") || got.ElapsedMS < 0 {
		t.Errorf("URL/耗时不对: %+v", got)
	}
}

// TestSourceTestProbeHTTPFailure 单源失败要报出状态码并返回非零错误。
func TestSourceTestProbeHTTPFailure(t *testing.T) {
	prof, opt := probeFixture(t, http.StatusForbidden)
	var text bytes.Buffer
	err := runSourceTest(context.Background(), []source.Profile{prof},
		sourceTestOpts{out: &text, id: prof.ID, title: "示例书一", options: opt})
	if err == nil {
		t.Fatal("403 应返回错误（CLI 以非零码退出）")
	}
	if !strings.Contains(text.String(), "HTTP 403") || !strings.Contains(text.String(), "失败") {
		t.Errorf("失败报告应含状态码与原因：\n%s", text.String())
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("返回错误应带状态码，实际 %v", err)
	}
}
