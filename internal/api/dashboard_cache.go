package api

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"
)

// 本文件是驾驶舱聚合端点（GET /api/dashboard）的短 TTL 缓存。
//
// 为什么要缓存：一次聚合要在服务端并发跑 7 组查询（健康报告自己还要 7 个 goroutine）。
// 驾驶舱的刷新按钮连点、PC 与平板同时开着驾驶舱、前端按 revision 触发的重取，
// 都会在几百毫秒内重复发同一份聚合——而这些数字以「秒」为粒度变化，没有必要求新的。
//
// 语义（与 /api/health/report 的 stale 是两件事，别混）：
//   - TTL 内复用同一次结果，响应里 cached=true + ageSeconds 让调用方知道这是几秒前的数；
//   - TTL 外的第一个请求**真正计算**，同一时刻的其它请求等它算完再复用
//     （single-flight：慢查询不会因为并发而被放大成 N 倍）；
//   - 只缓存**完整**的成功结果：算失败（如库被删）、或某块为空却与 stats 打架
//     （见 dashboard_ready.go）都不进缓存，下一次请求会立刻重算。

const (
	// envDashboardTTL 覆盖驾驶舱聚合的缓存时长（Go duration 文本，如 3s / 500ms）。
	envDashboardTTL = "NORVELS_DASHBOARD_TTL"
	// defaultDashboardTTL 是默认缓存时长：5 秒足够吸收刷新连点，又短到用户看不出延迟。
	defaultDashboardTTL = 5 * time.Second
	// dashboardCacheKeys 是缓存条目上限（键 = 参数组合，正常只有个位数；
	// 超限时丢掉最旧的一条，避免被构造出来的参数把内存撑开）。
	dashboardCacheKeys = 16
)

// dashboardCacheTTL 决定缓存时长：Config 显式值优先，其次环境变量，最后默认值。
//
// 约定与其它 0 值配置一致：0 = 用默认，负数 = 关闭缓存（每次都真算，供排障用）。
func dashboardCacheTTL(cfg Config) time.Duration {
	if cfg.DashboardTTL > 0 {
		return cfg.DashboardTTL
	}
	if cfg.DashboardTTL < 0 {
		return 0
	}
	raw := strings.TrimSpace(os.Getenv(envDashboardTTL))
	if raw == "" {
		return defaultDashboardTTL
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultDashboardTTL // 写错环境变量不该把驾驶舱变成无缓存
	}
	return d
}

// dashboardCache 是按参数键缓存聚合结果的短 TTL 缓存。
type dashboardCache struct {
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]*dashboardEntry
}

// dashboardEntry 是一个键的缓存槽位；loading 非 nil 表示「有人正在算」。
type dashboardEntry struct {
	at      time.Time // 结果生成时间（零值 = 还没算完）
	payload dashboardPayload
	loading chan struct{}
}

// newDashboardCache 构造缓存；ttl<=0 表示每次真算（等价于关闭）。
func newDashboardCache(ttl time.Duration) *dashboardCache {
	return &dashboardCache{ttl: ttl, entries: make(map[string]*dashboardEntry)}
}

// do 返回 key 对应的聚合结果：命中则直接返回（cached=true），未命中则调用 load 真算一次。
//
// 「查缓存 / 占位」在 beginOrClaim 里同一把锁内完成，因此同一时刻只有一个计算者，
// 其余并发请求等它算完再复用结果（single-flight）。
func (c *dashboardCache) do(ctx context.Context, key string,
	load func(context.Context) (dashboardPayload, error)) (dashboardPayload, bool, error) {
	for {
		payload, slot, hit, wait := c.beginOrClaim(key)
		if hit {
			return payload, true, nil
		}
		if wait != nil {
			select {
			case <-wait:
				continue // 别人算完了：回到开头读结果
			case <-ctx.Done():
				return dashboardPayload{}, false, ctx.Err()
			}
		}
		payload, err := load(ctx)
		c.finish(key, slot, payload, err)
		if err != nil {
			return dashboardPayload{}, false, err
		}
		c.trim()
		return payload, false, nil
	}
}

// beginOrClaim 做一次原子动作：命中返回结果；有人在算返回它的 wait channel；
// 否则登记自己是计算者并返回占位 slot。
func (c *dashboardCache) beginOrClaim(key string) (payload dashboardPayload,
	slot chan struct{}, hit bool, wait chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.entries[key]; e != nil {
		if e.loading != nil {
			return dashboardPayload{}, nil, false, e.loading
		}
		if c.ttl > 0 && time.Since(e.at) < c.ttl {
			return e.payload, nil, true, nil
		}
	}
	slot = make(chan struct{})
	c.entries[key] = &dashboardEntry{loading: slot}
	return dashboardPayload{}, slot, false, nil
}

// cacheable 报告一份聚合结果能不能进 TTL 缓存：只有「七块都齐、且自洽」的结果才收。
//
// 带 warning 的结果（某块失败、或一致性校验发现空块与 stats 打架）一律不缓存：
// 否则一次启动期 / 瞬时空快照会被 TTL 缓存分发给之后所有并发请求，驾驶舱整页空态
// 要挂满一个 TTL 才恢复，而且后来的人拿到的响应里连 warning 都可能是旧的。
// 「不缓存」同时就是「下次重试」——下一个请求会重新真算，直到拿到完整结果。
func cacheable(payload dashboardPayload) bool { return len(payload.Warnings) == 0 }

// finish 落结果并唤醒等待者；失败与不完整都不缓存（走同一条「下次重试」路径）。
//
// 成功时必须把 loading 置 nil：它同时是「有人正在算」的标记与等待者监听的 channel，
// 忘了清掉会让之后的每个请求都在「等一个已经关闭的 channel」里空转（实测整请求挂到超时）。
// 等待者手里已经有 channel 值，唤醒后重进 beginOrClaim 会读到 loading == nil 的新鲜结果。
func (c *dashboardCache) finish(key string, slot chan struct{}, payload dashboardPayload, err error) {
	c.mu.Lock()
	if e := c.entries[key]; e != nil && e.loading == slot {
		if err != nil || !cacheable(payload) {
			delete(c.entries, key) // 不完整 / 失败：丢掉槽位，下次请求重新真算
		} else {
			e.payload, e.at, e.loading = payload, time.Now(), nil
		}
	}
	c.mu.Unlock()
	close(slot)
}

// trim 在条目超限时丢掉最旧的一条（缓存只是加速，丢光也不会算错）。
func (c *dashboardCache) trim() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) <= dashboardCacheKeys {
		return
	}
	oldestKey := ""
	var oldest time.Time
	for k, v := range c.entries {
		if v.loading != nil {
			continue
		}
		if oldestKey == "" || v.at.Before(oldest) {
			oldestKey, oldest = k, v.at
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
