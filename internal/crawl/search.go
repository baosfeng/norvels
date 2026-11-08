package crawl

import (
	"context"
	"strings"
	"time"

	"norvels/internal/enrich"
	"norvels/internal/source"
)

// withDefaults 补齐零值并按抓取器的口径校验 QPS（负值表示不限速，只给测试用）。
func (o Options) withDefaults() (Options, error) {
	if o.Workers <= 0 {
		o.Workers = 4
	}
	if o.QPS == 0 {
		o.QPS = 1
	}
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	if o.MaxSources <= 0 {
		o.MaxSources = 3
	}
	if o.StopAfterHits <= 0 {
		o.StopAfterHits = 2
	}
	if o.MinTitleSim <= 0 {
		o.MinTitleSim = enrich.DefaultOptions().MinTitleSim
	}
	if o.UserAgent == "" {
		o.UserAgent = "norvels-enrich"
	}
	return o, nil
}

// acquire 在发请求前占一个全局请求名额并检查墙钟预算。
func (e *Engine) acquire() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.deadline.IsZero() == false && time.Now().After(e.deadline) {
		return ErrBudget
	}
	if e.opt.MaxRequests > 0 && e.requests >= e.opt.MaxRequests {
		return ErrBudget
	}
	e.requests++
	return nil
}

// SearchBook 按优先级依次问源，直到凑够 StopAfterHits 条相似命中或源用尽。
//
// 返回的 error 只在「完全没有可用源」或 ctx 取消时非 nil；
// 预算打满属于正常收尾（已得结果照常返回，Requests 表明用了多少请求）。
func (e *Engine) SearchBook(ctx context.Context, title string) (Result, error) {
	res := Result{Tried: []string{}, Failed: []string{}, Degraded: []string{}}
	if len(e.sources) == 0 {
		return res, ErrNoSource
	}
	tried := 0
	for _, st := range e.sources {
		if tried >= e.opt.MaxSources {
			break
		}
		if st.isTripped() {
			res.Failed = append(res.Failed, st.profile.ID)
			continue
		}
		hits, degraded, err := e.searchOne(ctx, st, title)
		tried++
		res.Tried = append(res.Tried, st.profile.ID)
		res.Requests = e.Stats().Requests
		res.Degraded = append(res.Degraded, degraded...)
		if err != nil {
			if errorsIsBudget(err) {
				return res, nil
			}
			res.Failed = append(res.Failed, st.profile.ID)
			continue
		}
		res.Hits = append(res.Hits, hits...)
		if len(res.Hits) >= e.opt.StopAfterHits {
			break
		}
	}
	return res, nil
}

// searchOne 向单个源发一次搜索请求，按源配置的编码解码后再解析列表。
func (e *Engine) searchOne(ctx context.Context, st *sourceState, title string) ([]enrich.Hit, []string, error) {
	raw, opts, degraded, err := e.fetchList(ctx, st, title)
	if err != nil {
		return nil, degraded, err
	}
	text, _, warning := decodeBody(raw, opts.Charset, st.profile.Decode.ForceEncoding)
	if warning != "" {
		degraded = append(degraded, st.profile.ID+":decode:"+warning)
	}
	hits, ruleDegraded, err := parseHits(st, []byte(text), title, e.opt.MinTitleSim)
	return hits, append(degraded, ruleDegraded...), err
}

// fetchList 请求搜索页：模板替换后先拆 Legado 选项块（,{"charset":"gbk"}），
// 纯 URL 才发请求；不认识的选项按降级回显，不影响本次请求。
func (e *Engine) fetchList(ctx context.Context, st *sourceState, title string) ([]byte, source.URLOptions, []string, error) {
	rawURL, opts, degraded := prepareSearchURL(st.profile, title)
	if err := e.acquire(); err != nil {
		return nil, opts, degraded, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, e.opt.Timeout)
	defer cancel()
	resp, err := st.client.Get(reqCtx, rawURL)
	st.record(err)
	if err != nil {
		return nil, opts, degraded, err
	}
	return resp.Body, opts, degraded, nil
}

// urlOptionWarnings 把无法解析的选项块与未实现的选项键转成降级条目。
func urlOptionWarnings(sourceID string, opts source.URLOptions) []string {
	out := make([]string, 0, len(opts.Ignored)+1)
	if opts.Warning != "" {
		out = append(out, sourceID+":urlOptions "+opts.Warning)
	}
	for _, key := range opts.Ignored {
		out = append(out, sourceID+":urlOptions."+key+" 未实现，已忽略")
	}
	return out
}

// expandSearchURL 展开书源搜索 URL 的两个变量。
func expandSearchURL(tpl, key string) string {
	out := strings.ReplaceAll(tpl, "{{key}}", urlEscape(key))
	out = strings.ReplaceAll(out, "{{page}}", "1")
	// 兼容 Legado 的 {{page}} 与常见别名。
	out = strings.ReplaceAll(out, "{{searchKey}}", urlEscape(key))
	return out
}

// record 记录一次请求结果，维护连续失败计数与熔断位。
func (s *sourceState) record(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs++
	if err == nil {
		s.failures = 0
		return
	}
	s.failures++
	if s.failures >= breakerThreshold {
		s.tripped = true
	}
}

// isTripped 报告该源是否已被熔断。
func (s *sourceState) isTripped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tripped
}

// errorsIsBudget 判断是否为预算用尽（不引入 errors 包别名，保持调用点可读）。
func errorsIsBudget(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrBudget.Error())
}
