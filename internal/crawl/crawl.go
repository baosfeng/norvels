// Package crawl 是「按在线书源检索一本书的元数据」的执行器。
//
// 它把三件已有能力接起来：internal/fetch 做受限抓取（SSRF/端口/重定向/QPS/并发，
// 见 docs/adr/0004-online-book-source.md §3）、internal/ruleexec 求值声明式规则、
// internal/enrich 做纯决策（书名相似度）。本包自己只负责编排：
// 多源并行、单源匀速、连续失败熔断、请求数与墙钟预算、@js 降级点名。
//
// 本包不发 PUT/POST、不写库、不解析正文，也不执行任何 JS 规则。
package crawl

import (
	"context"
	"errors"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"norvels/internal/enrich"
	"norvels/internal/fetch"
	"norvels/internal/source"
)

// 熔断阈值：单源连续这么多次网络类失败后，本会话不再请求它。
//
// 5 次是折中：书源站抽风通常是短时的，2 次就熔断会让一次抖动静默掉整个源；
// 而 10 次意味着 10 个失败请求的等待时间（按 10s 超时算是 100s）才止损。
const breakerThreshold = 5

// Options 是执行器参数；零值由 withDefaults 补齐。
type Options struct {
	Workers       int           // 同时在飞的检索任务数（默认 4）
	QPS           float64       // 单个书源每秒请求数（默认 1）；<0 表示不限速（仅测试）
	Timeout       time.Duration // 单次请求超时（默认 10s）
	MaxSources    int           // 一本书最多问几个源（默认 3）
	MaxRequests   int           // 全局请求上限（0 = 不限）
	Budget        time.Duration // 墙钟预算（0 = 不限）
	StopAfterHits int           // 凑够几条相似命中即停（默认 2）
	MinTitleSim   float64       // 命中书名相似度下限（默认 0.86）
	UserAgent     string        // 请求 UA（默认 norvels-enrich）
	// DialContext 覆盖默认拨号器，只给测试把公网地址路由到 httptest 用；生产不要设置。
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	// AllowedPorts 覆盖允许出网的端口（默认 80/443）；只给测试放行 httptest 的随机端口用。
	// 生产不要设置：书源包内不能放宽端口是 ADR-0004 §3 的硬约束。
	AllowedPorts []int
}

// Result 是一本书的检索结果。
type Result struct {
	Hits     []enrich.Hit
	Tried    []string // 试过的 sourceId（按尝试顺序）
	Failed   []string // 失败/熔断的 sourceId（稳定排序）
	Degraded []string // 因 @js 降级的规则，形如 "sourceId:ruleSearch.bookList"
	Requests int
}

// Engine 是并发安全的检索执行器，进程内复用一个（限速器与熔断状态都在里面）。
type Engine struct {
	opt      Options
	sources  []*sourceState
	mu       sync.Mutex
	requests int
	deadline time.Time
}

// sourceState 是一个书源在本进程内的运行态（限速客户端 + 熔断计数）。
type sourceState struct {
	profile  source.Profile
	client   *fetch.Client
	mu       sync.Mutex
	failures int
	tripped  bool
	reqs     int
}

// NewEngine 只用「kind=online 且 enabled」的规则包构造执行器：
// 按 priority 降序、同优先级按 id 升序，保证多源顺序稳定。
func NewEngine(profiles []source.Profile, opt Options) (*Engine, error) {
	o, err := opt.withDefaults()
	if err != nil {
		return nil, err
	}
	e := &Engine{opt: o}
	if o.Budget > 0 {
		e.deadline = time.Now().Add(o.Budget)
	}
	for _, p := range profiles {
		if p.Kind != source.KindOnline || !p.Enabled || p.Online == nil {
			continue
		}
		// 书源加载时转换一次：库里可能还存着导入适配器上线前的 Legado 简写原文。
		// 复制 online 段再改，避免转换结果写回调用方传进来的规则包。
		online := *p.Online
		convertOnlineRules(&online)
		p.Online = &online
		st, err := newSourceState(p, o)
		if err != nil {
			return nil, err
		}
		e.sources = append(e.sources, st)
	}
	sort.SliceStable(e.sources, func(i, j int) bool {
		if e.sources[i].profile.Priority != e.sources[j].profile.Priority {
			return e.sources[i].profile.Priority > e.sources[j].profile.Priority
		}
		return e.sources[i].profile.ID < e.sources[j].profile.ID
	})
	return e, nil
}

// convertOnlineRules 把五个规则块里的 Legado 简写翻译成标准 CSS（加载时转换一次）。
//
// 转换失败保留原文：导入时已在 degradedFields 里报过，运行期不重复刷屏；
// 规则本身抽不出数据时按空结果处理，不影响其它字段与整个源。
func convertOnlineRules(o *source.Online) {
	o.RuleSearch, _ = source.ConvertLegadoRules(o.RuleSearch)
	o.RuleExplore, _ = source.ConvertLegadoRules(o.RuleExplore)
	o.RuleBookInfo, _ = source.ConvertLegadoRules(o.RuleBookInfo)
	o.RuleToc, _ = source.ConvertLegadoRules(o.RuleToc)
	o.RuleContent, _ = source.ConvertLegadoRules(o.RuleContent)
}

// newSourceState 为一个书源建受限客户端：限速与并发都在 fetch.Client 内部。
func newSourceState(p source.Profile, o Options) (*sourceState, error) {
	opt := fetch.DefaultOptions()
	opt.QPS = o.QPS
	opt.TotalTimeout = o.Timeout
	opt.UserAgent = o.UserAgent
	opt.DialContext = o.DialContext
	if len(o.AllowedPorts) > 0 {
		opt.AllowedPorts = o.AllowedPorts
	}
	if o.MaxSources > 0 {
		opt.MaxConcurrent = o.Workers
	}
	return &sourceState{profile: p, client: fetch.New(opt)}, nil
}

// Sources 返回本次生效的书源 id 列表（稳定顺序），供报告展示。
func (e *Engine) Sources() []string {
	out := make([]string, 0, len(e.sources))
	for _, s := range e.sources {
		out = append(out, s.profile.ID)
	}
	return out
}

// Stats 是执行器累计统计，供 CLI 汇总打印。
type Stats struct {
	Requests int
	Tripped  []string // 被熔断的 sourceId（稳定排序）
}

// Stats 返回累计统计（并发安全）。
func (e *Engine) Stats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := Stats{Requests: e.requests}
	for _, s := range e.sources {
		s.mu.Lock()
		if s.tripped {
			st.Tripped = append(st.Tripped, s.profile.ID)
		}
		s.mu.Unlock()
	}
	sort.Strings(st.Tripped)
	return st
}

// 预算与超时错误：调用方用 errors.Is 区分「正常收尾」与「真错误」。
var (
	// ErrBudget 表示请求数或墙钟预算已打满，本次检索提前收尾（不算失败）。
	ErrBudget = errors.New("crawl: 预算已用尽")
	// ErrNoSource 表示没有可用的在线书源（规则包为空或全部熔断）。
	ErrNoSource = errors.New("crawl: 没有可用的在线书源")
)

// urlEscape 把搜索关键词转成查询串片段（空格用 + 而不是 %20，多数书源站这样认）。
func urlEscape(key string) string {
	return strings.ReplaceAll(url.QueryEscape(key), "%20", "+")
}
