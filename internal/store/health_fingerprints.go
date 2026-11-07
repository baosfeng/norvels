package store

import (
	"context"
	"fmt"
)

// FingerprintHealth 是章级指纹的覆盖率（迁移 0019 之后才有意义）。
//
// 为什么放进健康报告：0019 只建表、不回填历史书，而「判重比不到老书」在用户侧的表现是
// 「同一本书又入了一本」——这种沉默的退化必须出现在运维看得见的地方，并直接给出处置命令。
type FingerprintHealth struct {
	Books         int    `json:"books"`          // 主库书数
	Fingerprinted int    `json:"fingerprinted"`  // 有指纹的书数（覆盖表行数）
	Missing       int    `json:"missing"`        // 缺口 = Books − Fingerprinted
	Hint          string `json:"hint,omitempty"` // 缺口 > 0 时的处置建议（指向回填命令）
	// Stale 表示 Fingerprinted 不能当作最终覆盖数看：
	//   - 正文库还没有覆盖表（0023 未应用）：退回全表 COUNT(DISTINCT book_id)，这次请求付了 20 秒级扫描；
	//   - 覆盖表的历史回填还没跑完（后台任务在补）：数字只是「已补到的那部分」。
	// 两种都必须让运维看见，否则会把「还没补」误读成「这些书都没有指纹」。
	Stale bool `json:"stale,omitempty"`
	// Backfilling 表示覆盖表的历史回填正在后台跑（扫 5125 万行指纹，分钟级）。
	// 增量字段：前端可忽略；运维用它区分「还没开始」与「正在补」。
	Backfilling bool `json:"backfilling,omitempty"`
}

// fingerprintHealth 统计指纹覆盖率；正文库还没有指纹表时返回 nil（迁移未应用，不喊缺口）。
//
// 两个数是**跨库**数出来的：书数在主库，指纹按 book_id 去重数在正文库。
// 章节全空的书写不出指纹，会长期留在缺口里——这类书本来就判不出重复，留缺口是对的。
func (s *Store) fingerprintHealth(ctx context.Context, r *HealthReport, pre healthPrecompute) *FingerprintHealth {
	ready, err := s.fingerprintTableExists(ctx)
	if err != nil || !ready {
		if err != nil {
			r.warn("fingerprints", err)
		}
		return nil
	}
	books, err := s.bookTotalCount(ctx, pre)
	if err != nil {
		r.warn("fingerprints", err)
		return nil
	}
	fingerprinted, stale, err := s.fingerprintedBookCount(ctx)
	if err != nil {
		r.warn("fingerprints", err)
		return nil
	}
	h := &FingerprintHealth{Books: books, Fingerprinted: fingerprinted, Stale: stale}
	if missing := books - fingerprinted; missing > 0 {
		h.Missing = missing
		h.Hint = fmt.Sprintf(
			"有 %d 本书还没有章级指纹（判重比不到它们）：跑一次 norvels backfill-fingerprints 补齐", missing)
	}
	return h
}

// fingerprintHealthWithBackfill 在统计覆盖率的同时，把「后台回填是否在跑」也报出来。
//
// 覆盖表在升级后的头几分钟是空的（历史 5125 万行指纹由后台分批扫进来），
// 这期间 Missing 会看起来很大。Hint 说清「这是后台在补、无需操作」，
// 否则运维会以为要手工跑一遍几十小时的回填。
func (s *Store) fingerprintHealthWithBackfill(ctx context.Context, r *HealthReport,
	pre healthPrecompute) *FingerprintHealth {
	h := s.fingerprintHealth(ctx, r, pre)
	if h == nil {
		return nil
	}
	complete, _, err := s.coverageState(ctx)
	if err != nil {
		r.warn("fingerprints", err)
		return h
	}
	if !complete {
		h.Stale = true
		h.Backfilling = s.CoverageBackfillRunning()
		if h.Backfilling {
			h.Hint = fmt.Sprintf(
				"指纹覆盖表正在后台回填（已覆盖 %d 本，总数以扫描为准）：无需操作，完成后自动消失", h.Fingerprinted)
		}
		s.EnsureFingerprintCoverage(ctx)
	}
	return h
}

// bookTotalCount 是「books 表行数」（指纹覆盖率的分母）：优先 0026 预计算，降级实时 COUNT。
//
// 这条 COUNT(*) 也曾是 health/report 上的一次 books 全表扫（18.2 万本，NAS 上数十毫秒）。
// 降级路径与 0025 / 0026 同一约定：数字同一口径，慢一点而已，绝不报错。
func (s *Store) bookTotalCount(ctx context.Context, pre healthPrecompute) (int, error) {
	if pre.totalsReady {
		return pre.totals.BooksAll, nil
	}
	return s.bookCount(ctx)
}

// fingerprintedBookCount 数「有指纹的书」。
//
// 优先读覆盖表（O(书数)，毫秒级）；覆盖表不存在时退回 0023 之前那条
// COUNT(DISTINCT book_id)（正文库 51,254,165 行实测约 22 s）并标 stale——
// 旧库没跑 0023 时必须降级出正确数字，不能 500，也不能悄悄慢。
func (s *Store) fingerprintedBookCount(ctx context.Context) (count int, stale bool, err error) {
	ready, err := s.fingerprintBooksTableExists(ctx)
	if err != nil {
		return 0, false, err
	}
	if ready {
		n, err := s.fingerprintCoverageCount(ctx)
		return n, false, err
	}
	n, err := s.fingerprintedBookCountLegacy(ctx)
	return n, true, err
}

// fingerprintedBookCountLegacy 是 0023 之前的口径（全表去重计数），只在降级路径上跑。
func (s *Store) fingerprintedBookCountLegacy(ctx context.Context) (int, error) {
	var n int
	if err := s.cdb.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT book_id) FROM chapter_fingerprints").Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count fingerprinted books: %w", err)
	}
	return n, nil
}
