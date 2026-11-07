package store

import (
	"context"
	"sync"
)

// healthPrecompute 是一次健康报告请求里可用的预计算快照。
//
// 两个来源分开带 ready：0025（书级健康计数）与 0026（库级总量）是两条独立迁移，
// 老库上可能只应用了一条。任何一项不可用时对应指标回退实时 SQL，
// 数字仍正确——降级绝不返回错数字，也不 500。
type healthPrecompute struct {
	counts      bookHealthCounts
	countsReady bool
	totals      bookTotals
	totalsReady bool
}

// healthPrecompute 取本次请求可用的预计算快照，并在缺失时拉起后台回填。
//
// 非阻塞：后台回填失败只记日志，本次请求继续走实时 SQL。
func (s *Store) healthPrecompute(ctx context.Context) healthPrecompute {
	var pre healthPrecompute
	pre.counts, pre.countsReady = s.bookHealthCounts(ctx)
	if !pre.countsReady {
		s.EnsureBookHealthCounts()
	}
	pre.totals, pre.totalsReady = s.bookTotals(ctx)
	if !pre.totalsReady {
		s.EnsureBookTotals()
	}
	return pre
}

// runBookMetrics 并发跑健康报告里七项互不依赖的统计，返回「长度直方图这次是实时算的」。
//
// 为什么并发：七项相互独立、连接池有 8 个连接，串行会白等 IO（真实库实测串行 138 ms、
// 并发约 40 ms）。为什么把返回值单独带出来：lengthLive 由并发 goroutine 写，
// 只有 wg.Wait() 之后才允许读，塞进 HealthReport 会让「谁写的、什么时候能读」变模糊。
func (s *Store) runBookMetrics(ctx context.Context, r *HealthReport,
	pre healthPrecompute, introWhere string) (lengthLive bool) {
	var wg sync.WaitGroup
	wg.Add(7)
	go func() {
		defer wg.Done()
		// 只数当前版本：健康报告的书数与书架同口径，否则替换一次旧版就多出「一本书」。
		// 0026 起读预计算（books_current），降级时退回实时 COUNT。
		r.TotalBooks = s.precomputedInt(ctx, r, "totalBooks", pre.totalsReady, pre.totals.BooksCurrent,
			"SELECT COUNT(*) FROM books WHERE superseded_by = 0")
	}()
	// 四项判据都来自 health_issues.go 的同一份 WHERE：报告的数字与「查看明细」
	// 接口列出的条数由同一条 SQL 条件保证一致，不会两处各写一套而漂移。
	go func() {
		defer wg.Done()
		r.ThinBooks = s.precomputedInt(ctx, r, "thinBooks", pre.countsReady, pre.counts.ThinBooks,
			"SELECT COUNT(*) FROM books WHERE "+thinBookWhere("chapter_count"))
	}()
	go func() {
		defer wg.Done()
		r.MissingAuthor = s.precomputedInt(ctx, r, "missingAuthor", pre.countsReady, pre.counts.MissingAuthor,
			"SELECT COUNT(*) FROM books WHERE "+whereMissingAuthor)
	}()
	go func() {
		defer wg.Done()
		r.MissingIntro = s.precomputedInt(ctx, r, "missingIntro", pre.countsReady, pre.counts.MissingIntro,
			"SELECT COUNT(*) FROM books WHERE "+introWhere)
	}()
	go func() {
		defer wg.Done()
		r.HugeChapters = s.intMetric(ctx, r, "hugeChapters",
			"SELECT COUNT(*) FROM chapters WHERE "+hugeChapterWhere("word_count"))
	}()
	go func() {
		defer wg.Done()
		r.ChapterBuckets = s.chapterBuckets(ctx, r, pre.counts, pre.countsReady)
	}()
	go func() {
		defer wg.Done()
		// 0022 起走预计算直方图：chapters 有 2,469 万行时，实时跑那 5 段区间 COUNT
		// 要扫 5 遍索引（NAS 实测 3.35 s，9 个驾驶舱请求并发时 25.5 s）。
		// 缓存缺失时降到实时算并置 Stale，接口照常 200。
		r.ChapterLengthBuckets, lengthLive = s.lengthBuckets(ctx, r)
	}()
	wg.Wait()
	return lengthLive
}
