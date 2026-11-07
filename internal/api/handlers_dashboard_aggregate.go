package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"norvels/internal/store"
)

// 驾驶舱聚合端点：GET /api/dashboard（契约见 docs/api/api-dashboard.md）。
//
// 为什么要有它：驾驶舱一次刷新原本要发 10 个后端请求（stats / health / import-trend /
// reading-stats / authors / progress ×2 / jobs / metrics / recommend-stats），
// 每个请求各自打库、各自超时。生产 18.2 万本时 health/report 单独就 p95 5500 ms，
// 10 个请求叠在一起把首屏拖成十几秒。
//
// 这里把「数字类」的 7 块收成一个响应：服务端并发（而不是浏览器并发）取数，
// 加短 TTL 缓存吸收刷新连点。剩下两项刻意留在外面：
//   - /api/metrics：进程内快照，是排障面不是业务数据，且必须反映**当下**；
//   - /api/recommend/stats：依赖独立的推荐库，缺库时是 503，不该拖累整块驾驶舱。
// 于是驾驶舱一次刷新 = 1（本端点）+ 1（recommend/stats）+ 1（metrics）= 3 个请求。

const (
	// dashboardTimeout 是整次聚合的上限：任一子块卡住也不能把请求挂成 60 s。
	// 与前端 STATS_TIMEOUT_MS = 20 s 对齐（留 5 s 给网络与序列化）。
	dashboardTimeout = 15 * time.Second
)

// dashboardParams 是聚合端点的参数（同时也是缓存键的构成）。
type dashboardParams struct {
	Days        int // 导入趋势天数
	ReadingDays int // 阅读统计窗口
	TZ          int // 阅读统计的时区偏移（分钟）
	TopAuthors  int // 作者榜条数
	Progress    int // 最近在读条数
	Jobs        int // 导入任务条数
}

// parseDashboardParams 解析聚合端点参数；任一越界返回 400（不静默夹紧）。
func parseDashboardParams(r *http.Request) (dashboardParams, error) {
	var p dashboardParams
	var err error
	if p.Days, err = intQuery(r, "days", defaultTrendDays, 1, maxTrendDays); err != nil {
		return p, err
	}
	if p.ReadingDays, err = intQuery(r, "readingDays", defaultStatsDays, minStatsDays, maxStatsDays); err != nil {
		return p, err
	}
	if p.TZ, err = intQuery(r, "tz", 0, minTZMinutes, maxTZMinutes); err != nil {
		return p, err
	}
	if p.TopAuthors, err = intQuery(r, "topAuthors", defaultTopAuthors, 1, maxTopAuthors); err != nil {
		return p, err
	}
	if p.Progress, err = intQuery(r, "progress", maxProgressLimit, 1, maxProgressLimit); err != nil {
		return p, err
	}
	if p.Jobs, err = intQuery(r, "jobs", defaultJobLimit, 1, maxJobLimit); err != nil {
		return p, err
	}
	return p, nil
}

// key 是缓存键：同一组参数命中同一份聚合结果。
func (p dashboardParams) key() string {
	return fmt.Sprintf("days=%d;rdays=%d;tz=%d;authors=%d;progress=%d;jobs=%d",
		p.Days, p.ReadingDays, p.TZ, p.TopAuthors, p.Progress, p.Jobs)
}

// dashboardPayload 是 GET /api/dashboard 的响应体。
//
// 每个数据块的形状与它自己的端点逐字一致（stats 同 /api/stats、health 同
// /api/health/report…），前端切换到这个端点时不需要改任何渲染逻辑；
// 块内的 stale / estimated 标记原样保留，含义不变。
type dashboardPayload struct {
	// GeneratedAt 是这份数据的生成时刻（缓存命中时是上一次真算的时刻）。
	GeneratedAt time.Time `json:"generatedAt"`
	// Cached 为 true 表示这份数据来自 TTL 缓存（ageSeconds 是它的年龄）。
	Cached     bool    `json:"cached"`
	AgeSeconds float64 `json:"ageSeconds"`
	TTLSeconds float64 `json:"ttlSeconds"`
	// Warnings 收集「某一块没取到」的原因；整体仍是 200，对应块为 null。
	Warnings []string `json:"warnings"`

	Stats           *store.Stats                     `json:"stats"`
	Health          *store.HealthReport              `json:"health"`
	ImportTrend     []store.ImportTrendPoint         `json:"importTrend"`
	ReadingStats    *store.ReadingStats              `json:"readingStats"`
	TopAuthors      *store.Page[store.AuthorSummary] `json:"topAuthors"`
	ReadingProgress []store.ReadingProgress          `json:"readingProgress"`
	ImportJobs      []store.ImportJob                `json:"importJobs"`
}

// handleDashboard 处理 GET /api/dashboard：一次返回驾驶舱需要的全部数字。
//
// stats 是首屏的核心块，它失败则整体失败（与 /api/stats 同口径）；
// 其余块失败只记 Warnings 并把该块置 null——一个面板取不到不该让整页空白。
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	p, err := parseDashboardParams(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	payload, cached, err := s.dashboard.do(r.Context(), p.key(),
		func(ctx context.Context) (dashboardPayload, error) { return s.loadDashboard(ctx, p) })
	if err != nil {
		s.fail(w, r, err, "驾驶舱聚合不可用")
		return
	}
	payload.Cached = cached
	payload.TTLSeconds = s.dashboard.ttl.Seconds()
	payload.AgeSeconds = time.Since(payload.GeneratedAt).Seconds()
	writeJSON(w, http.StatusOK, payload)
}

// loadDashboard 并发取齐所有块；stats 同步先取（失败即整体失败）。
func (s *Server) loadDashboard(ctx context.Context, p dashboardParams) (dashboardPayload, error) {
	ctx, cancel := context.WithTimeout(ctx, dashboardTimeout)
	defer cancel()
	stats, err := s.st.Stats(ctx)
	if err != nil {
		return dashboardPayload{}, err
	}
	out := dashboardPayload{GeneratedAt: time.Now().UTC(), Stats: &stats, Warnings: []string{}}
	s.loadDashboardBlocks(ctx, p, &out)
	return out, nil
}

// dashboardRunner 是「并发取一块 + 失败记 warnings」的样板：
// 每个块一个 goroutine，写结果与追加告警都在同一把锁里（Warnings 是 slice，不能并发 append）。
type dashboardRunner struct {
	wg  sync.WaitGroup
	mu  sync.Mutex
	out *dashboardPayload
}

// run 启动一块的取数；fn 返回错误即记一条 warning（该块字段留 null / 空数组）。
func (r *dashboardRunner) run(name string, fn func() error) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := fn(); err != nil {
			r.mu.Lock()
			r.out.Warnings = append(r.out.Warnings, name+" 不可用："+err.Error())
			r.mu.Unlock()
		}
	}()
}

// set 在锁内写一个块的结果（闭包形式，省掉每块各写一遍 Lock/Unlock）。
func (r *dashboardRunner) set(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn()
}

// loadDashboardBlocks 并发跑其余六块，失败的块进 Warnings（对应字段留 null / 空数组）。
func (s *Server) loadDashboardBlocks(ctx context.Context, p dashboardParams, out *dashboardPayload) {
	r := &dashboardRunner{out: out}
	r.run("health", func() error {
		rep := s.st.HealthReport(ctx) // 单项失败只进它自己的 Warnings，整体恒成功
		r.set(func() { out.Health = &rep })
		return nil
	})
	r.run("importTrend", func() error {
		points, err := s.st.ImportTrend(ctx, p.Days)
		if err != nil {
			return err
		}
		r.set(func() { out.ImportTrend = nonNilPoints(points) })
		return nil
	})
	r.run("readingStats", func() error {
		stats, err := s.st.ReadingStats(ctx, p.ReadingDays, p.TZ)
		if err != nil {
			return err
		}
		if stats.Days == nil {
			stats.Days = []store.DaySeconds{}
		}
		r.set(func() { out.ReadingStats = &stats })
		return nil
	})
	r.run("topAuthors", func() error {
		page, err := s.st.ListAuthors(ctx, store.AuthorQuery{Sort: "words", Page: 1, Size: p.TopAuthors})
		if err != nil {
			return err
		}
		r.set(func() { out.TopAuthors = &page })
		return nil
	})
	r.run("readingProgress", func() error {
		items, err := s.st.ListReadingProgress(ctx, p.Progress)
		if err != nil {
			return err
		}
		r.set(func() { out.ReadingProgress = nonNilProgress(items) })
		return nil
	})
	r.run("importJobs", func() error {
		jobs, err := s.st.ListImportJobs(ctx, p.Jobs)
		if err != nil {
			return err
		}
		r.set(func() { out.ImportJobs = nonNilJobs(jobs) })
		return nil
	})
	r.wg.Wait()
	sort.Strings(out.Warnings) // 顺序稳定：同一份数据两次响应的 Warnings 不会乱序
}

// nonNilPoints 把 nil 切片换成空切片：契约里这三个字段恒为数组，不是 null。
func nonNilPoints(points []store.ImportTrendPoint) []store.ImportTrendPoint {
	if points == nil {
		return []store.ImportTrendPoint{}
	}
	return points
}

// nonNilProgress 把 nil 切片换成空数组（同上）。
func nonNilProgress(items []store.ReadingProgress) []store.ReadingProgress {
	if items == nil {
		return []store.ReadingProgress{}
	}
	return items
}

// nonNilJobs 把 nil 切片换成空数组（同上）。
func nonNilJobs(jobs []store.ImportJob) []store.ImportJob {
	if jobs == nil {
		return []store.ImportJob{}
	}
	return jobs
}
