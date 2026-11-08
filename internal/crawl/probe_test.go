package crawl

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestProbeMatchesEnrichPipeline Probe 必须与 enrich（SearchBook）走同一条链路：
// 同一个 Engine、同一套 header / URL 选项块 / 解码 / 规则求值；候选只差相似度过滤。
// 这样「自检说能抽出书名」才等价于「enrich 实际能抽出来」。
func TestProbeMatchesEnrichPipeline(t *testing.T) {
	srv, seen := newGBKSearchServer(t)
	e := newRequestLayerEngine(t, srv)

	probe, err := e.Probe(context.Background(), "src-a", "示例书一")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	requireSameRequest(t, seen)
	if len(probe.Candidates) != 1 {
		t.Fatalf("Probe 候选 = %d，期望 1：%+v", len(probe.Candidates), probe.Candidates)
	}
	c := probe.Candidates[0]
	if c.Title != "示例书一" || c.Author != "作者甲" || c.Similarity < 0.99 {
		t.Errorf("Probe 候选字段不对: %+v", c)
	}
	if !strings.HasSuffix(c.BookURL, "/book/1") {
		t.Errorf("Probe bookUrl 不对: %q", c.BookURL)
	}

	res, err := e.SearchBook(context.Background(), "示例书一")
	if err != nil {
		t.Fatalf("SearchBook: %v", err)
	}
	requireSameRequest(t, seen)
	if len(res.Hits) != 1 {
		t.Fatalf("enrich 命中 = %d，期望 1：%+v", len(res.Hits), res.Hits)
	}
	h := res.Hits[0]
	if h.Title != c.Title || h.Author != c.Author || h.BookURL != c.BookURL {
		t.Errorf("Probe 与 enrich 抽出的候选不一致：probe=%+v hit=%+v", c, h)
	}
	if got, want := strings.Join(probe.Degraded, "；"), strings.Join(res.Degraded, "；"); got != want {
		t.Errorf("Probe 与 enrich 的降级信息不一致：%q vs %q", got, want)
	}
}

// requireSameRequest 断言两次请求都带上了 header、且 URL 选项块已被剥离。
func requireSameRequest(t *testing.T, seen *requestSeen) {
	t.Helper()
	if seen.referer != "http://ref.example/" {
		t.Errorf("请求未带 header：Referer=%q", seen.referer)
	}
	if strings.Contains(seen.query, "charset") || strings.Contains(seen.query, "%2C") {
		t.Errorf("URL 选项块未剥离：%q", seen.query)
	}
}

// TestProbeUnknownSource 不存在的书源 id 返回 ErrSourceNotFound。
func TestProbeUnknownSource(t *testing.T) {
	e, err := NewEngine(nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Probe(context.Background(), "src-none", "示例书一"); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("err = %v，期望 ErrSourceNotFound", err)
	}
}
