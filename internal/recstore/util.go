package recstore

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// 库内时间一律 UTC 定长文本，与 internal/store 的 timeLayout 同口径。
//
// 为什么必须定长：最新曝光的判定、衰减的 Δt 都靠 ts 字符串比较/解析，
// 同一个库混进 "2026-11-01 12:00:00" 这种格式后，字符串比较会把 11 月排在 1 月前面。
const timeLayout = "2006-01-02T15:04:05.000Z"

func nowUTC() time.Time { return time.Now().UTC() }

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

// parseTime 解析客户端可能发来的几种时间写法（含 store 侧历史的空格写法）。
//
// 空串返回零值而不是错误：调用方按「空 = 服务端当前时间」处理。
func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	layouts := []string{timeLayout, time.RFC3339Nano, time.RFC3339,
		"2006-01-02 15:04:05", "2006-01-02T15:04:05"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("recstore: 时间格式非法 %q", s)
}

// dayOf 取归一化 ts 的 UTC 日期（前 10 字节）。
//
// 归一化后的 ts 是定长 UTC 文本，切前 10 字节就等于 SQLite 的 date()——按日聚合
// 与「近 N 天热度」因此不必依赖部署机时区。
func dayOf(ts string) string {
	if len(ts) >= len("2006-01-02") {
		return ts[:len("2006-01-02")]
	}
	return ts
}

// decay 把权重从他次写入的时刻衰减到 now：w·exp(-Δt/14d)。
//
// Δt ≤ 0 直接返回原值：客户端补发的旧事件、并发写入的同一秒都会让 Δt 变成 0 或负数，
// 指数函数此时算出 >1 的系数，会把兴趣分放大。
func decay(weight float64, from, to time.Time) float64 {
	if weight == 0 {
		return 0
	}
	elapsed := to.Sub(from)
	if elapsed <= 0 {
		return weight
	}
	return weight * math.Exp(-elapsed.Hours()/(24*decayDays))
}

// clampLimit 归一化查询条数：≤0 取默认值，过大夹到 500。
//
// 上限存在是为了防止 api 层把 limit 原样透传——一次 SELECT 十万行共读记录
// 会把这些辅助查询变成主库级的长事务。
func clampLimit(n, def int) int {
	switch {
	case n <= 0:
		return def
	case n > maxQueryLimit:
		return maxQueryLimit
	default:
		return n
	}
}

// clampDays 归一化热度窗口天数：≤0 取 7 天，>365 夹到 365（book_daily 按日聚簇）。
func clampDays(n int) int {
	switch {
	case n <= 0:
		return defaultHotDays
	case n > maxHotDays:
		return maxHotDays
	default:
		return n
	}
}
