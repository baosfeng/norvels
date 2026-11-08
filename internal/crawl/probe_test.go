package crawl

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/enrich"
)

// TestProbeMatchesEnrichPipeline Probe 必须与 enrich（SearchBook）走同一条链路：
// 同一个 Engine、同一套 header / URL 选项块 / 解码 / 规则求值；候选只差相似度过滤。
// 这样「自检说能抽出书名」才等价于「enrich 实际能抽出来」。
func TestProbeMatchesEnrichPipeline(t *testing.T) {
	srv, seen := newGBKSearchServer(t)
	e := newRequestLayerEngine(t, srv)
	probe := mustProbeOnce(t, e, seen)
	res := mustSearchOnce(t, e, seen)
	requireSamePipeline(t, probe, res)
}

// mustProbeOnce 发一次自检并断言请求确实带上了 header、选项块被剥离。
func mustProbeOnce(t *testing.T, e *Engine, seen *requestSeen) *Probe {
	t.Helper()
	probe, err := e.Probe(context.Background(), "src-a", "示例书一")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	requireSameRequest(t, seen)
	return probe
}

// mustSearchOnce 发一次批量检索（enrich 的路径）并断言同样带 header、选项块被剥离。
func mustSearchOnce(t *testing.T, e *Engine, seen *requestSeen) Result {
	t.Helper()
	res, err := e.SearchBook(context.Background(), "示例书一")
	if err != nil {
		t.Fatalf("SearchBook: %v", err)
	}
	requireSameRequest(t, seen)
	return res
}

// requireSamePipeline 断言两条路径抽出的候选与降级信息完全一致。
func requireSamePipeline(t *testing.T, probe *Probe, res Result) {
	t.Helper()
	if len(probe.Candidates) != 1 || len(res.Hits) != 1 {
		t.Fatalf("probe/hits = %d/%d，均期望 1", len(probe.Candidates), len(res.Hits))
	}
	requireSameCandidate(t, probe.Candidates[0], res.Hits[0])
	if got, want := strings.Join(probe.Degraded, "；"), strings.Join(res.Degraded, "；"); got != want {
		t.Errorf("Probe 与 enrich 的降级信息不一致：%q vs %q", got, want)
	}
}

// requireSameCandidate 断言 Probe 候选字段正确、且与 enrich 抽出的命中一致。
func requireSameCandidate(t *testing.T, c ProbeCandidate, h enrich.Hit) {
	t.Helper()
	if c.Title != "示例书一" || c.Author != "作者甲" || c.Similarity < 0.99 {
		t.Errorf("Probe 候选字段不对: %+v", c)
	}
	if !strings.HasSuffix(c.BookURL, "/book/1") {
		t.Errorf("Probe bookUrl 不对: %q", c.BookURL)
	}
	if h.Title != c.Title || h.Author != c.Author || h.BookURL != c.BookURL {
		t.Errorf("Probe 与 enrich 抽出的候选不一致：probe=%+v hit=%+v", c, h)
	}
}

// requireSameRequest 断言请求都带上了 header、且 URL 选项块已被剥离。
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
