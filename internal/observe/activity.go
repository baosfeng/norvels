package observe

import (
	"sync/atomic"
	"time"
)

// lastActivity 记录「最近一次真的有 SQL 跑」的时刻（unix 纳秒，0 = 从未）。
//
// 存在的理由只有一个：让「闲置多久了」可判断，从而把 SQLite 页缓存的归还
// 放在确实没人用的时候。每次 Query/Exec 只存一个 int64——不取锁、不分配。
var lastActivity atomic.Int64

// noteActivity 由驱动包装层在每次 Query/Exec 时调用。
//
// 刻意放在 disabled 判断**之前**：内存归还不该因为关掉观测就不工作，
// 而「有没有人在用库」与「要不要记指标」是两件事。
func noteActivity() { lastActivity.Store(time.Now().UnixNano()) }

// LastActivity 返回最近一次 SQL 活动的时刻（零值 = 本进程还没跑过 SQL）。
func LastActivity() time.Time {
	ns := lastActivity.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// IdleFor 返回距最近一次 SQL 活动过了多久。
//
// 从未跑过 SQL 时返回一个远大于任何闲置阈值的值：进程刚起、还没服务过任何
// 请求，此时本来就该被判为「闲」，不需要特判。
func IdleFor() time.Duration {
	t := LastActivity()
	if t.IsZero() {
		return time.Duration(1) << 62
	}
	return time.Since(t)
}
