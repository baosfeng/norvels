package crawl

// 单源自检：给一个书源 + 一个书名，发一次搜索请求，把「能不能抽出候选」说清楚。
//
// 与 enrich 的批量检索共用同一条链路（同一份 Engine、同一套 header/选项块/解码/
// 规则求值），因此自检结果能代表 enrich 的真实行为；区别只在于这里不做相似度过滤，
// 所有解析出的候选连相似度一起返回——「源坏了」与「只是搜到同人」一眼可分。

import (
	"context"
	"errors"
	"time"

	"norvels/internal/enrich"
)

// ErrSourceNotFound 表示指定的书源 id 不在执行器里。
var ErrSourceNotFound = errors.New("crawl: 没有这个书源")

// Probe 是一次单源自检的结果。
type Probe struct {
	SourceID   string           `json:"sourceId"`
	SourceName string           `json:"sourceName"`
	URL        string           `json:"url"`
	Status     int              `json:"status"`
	ElapsedMS  int64            `json:"elapsedMs"`
	Candidates []ProbeCandidate `json:"candidates"`
	Degraded   []string         `json:"degraded"`
}

// ProbeCandidate 是一条解析出的候选；Similarity 是它相对查询书名的相似度。
type ProbeCandidate struct {
	Title      string  `json:"title"`
	Author     string  `json:"author"`
	BookURL    string  `json:"bookUrl"`
	Similarity float64 `json:"similarity"`
}

// Probe 对指定书源发一次搜索请求并返回全部候选（不做相似度过滤）。
// 请求失败时仍尽量返回已拿到的 URL/状态码/耗时，便于排障。
func (e *Engine) Probe(ctx context.Context, sourceID, title string) (*Probe, error) {
	st, ok := e.findSource(sourceID)
	if !ok {
		return nil, ErrSourceNotFound
	}
	p := &Probe{SourceID: st.profile.ID, SourceName: st.profile.Name, Degraded: []string{}}
	rawURL, opts, degraded := prepareSearchURL(st.profile, title)
	p.Degraded = append(p.Degraded, degraded...)
	if err := e.acquire(); err != nil {
		return p, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, e.opt.Timeout)
	defer cancel()
	start := time.Now()
	resp, err := st.client.Get(reqCtx, rawURL)
	p.ElapsedMS = time.Since(start).Milliseconds()
	st.record(err)
	p.URL = rawURL
	if resp != nil {
		p.Status = resp.Status
		if resp.FinalURL != "" {
			p.URL = resp.FinalURL
		}
	}
	if err != nil {
		return p, err
	}
	text, _, warning := decodeBody(resp.Body, opts.Charset, st.profile.Decode.ForceEncoding)
	if warning != "" {
		p.Degraded = append(p.Degraded, sourceID+":decode:"+warning)
	}
	// minSim 传 -1：自检要看全部候选，相似度由调用方展示，不在这一层过滤。
	hits, degraded, perr := parseHits(st, []byte(text), title, -1)
	p.Degraded = append(p.Degraded, degraded...)
	p.Candidates = probeCandidates(title, hits)
	return p, perr
}

// findSource 按 id 取本进程内的书源运行态。
func (e *Engine) findSource(sourceID string) (*sourceState, bool) {
	for _, st := range e.sources {
		if st.profile.ID == sourceID {
			return st, true
		}
	}
	return nil, false
}

// probeCandidates 把命中列表转成带相似度的自检候选。
func probeCandidates(query string, hits []enrich.Hit) []ProbeCandidate {
	out := make([]ProbeCandidate, 0, len(hits))
	for _, h := range hits {
		out = append(out, ProbeCandidate{
			Title: h.Title, Author: h.Author, BookURL: h.BookURL,
			Similarity: enrich.TitleSim(query, h.Title),
		})
	}
	return out
}
