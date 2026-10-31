package fetch

import (
	"context"
	"sync"
	"time"
)

// limiter 是单源匀速限速器：按 QPS 控制两次请求之间的最小间隔。
// nil 表示不限速（Options.QPS < 0，仅测试用）。
type limiter struct {
	mu   sync.Mutex
	rate float64
	next time.Time // 下一次允许发包的时间
}

func newLimiter(qps float64) *limiter {
	if qps < 0 {
		return nil
	}
	return &limiter{rate: qps}
}

// wait 等到下次可发；ctx 取消则立即返回。
func (l *limiter) wait(ctx context.Context) error {
	if l == nil {
		return nil
	}
	delay := l.take()
	if delay <= 0 {
		return nil
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// take 预约一个发送名额，返回需要等待的时长。
func (l *limiter) take() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.Before(l.next) {
		delay := l.next.Sub(now)
		l.next = l.next.Add(time.Duration(float64(time.Second) / l.rate))
		return delay
	}
	l.next = now.Add(time.Duration(float64(time.Second) / l.rate))
	return 0
}

// acquire 依次拿限速名额与并发名额，返回释放函数。
func (c *Client) acquire(ctx context.Context) (func(), error) {
	if err := c.limiter.wait(ctx); err != nil {
		return nil, err
	}
	select {
	case c.sem <- struct{}{}:
		return func() { <-c.sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
