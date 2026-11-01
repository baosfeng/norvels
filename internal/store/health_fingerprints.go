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
	Fingerprinted int    `json:"fingerprinted"`  // 有指纹的书数（正文库里出现过的 book_id）
	Missing       int    `json:"missing"`        // 缺口 = Books − Fingerprinted
	Hint          string `json:"hint,omitempty"` // 缺口 > 0 时的处置建议（指向回填命令）
}

// fingerprintHealth 统计指纹覆盖率；正文库还没有指纹表时返回 nil（迁移未应用，不喊缺口）。
//
// 两个数是**跨库**数出来的：书数在主库，指纹按 book_id 去重数在正文库。
// 章节全空的书写不出指纹，会长期留在缺口里——这类书本来就判不出重复，留缺口是对的。
func (s *Store) fingerprintHealth(ctx context.Context, r *HealthReport) *FingerprintHealth {
	ready, err := s.fingerprintTableExists(ctx)
	if err != nil || !ready {
		if err != nil {
			r.warn("fingerprints", err)
		}
		return nil
	}
	books, err := s.bookCount(ctx)
	if err != nil {
		r.warn("fingerprints", err)
		return nil
	}
	var fingerprinted int
	if err := s.cdb.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT book_id) FROM chapter_fingerprints").Scan(&fingerprinted); err != nil {
		r.warn("fingerprints", err)
		return nil
	}
	h := &FingerprintHealth{Books: books, Fingerprinted: fingerprinted}
	if missing := books - fingerprinted; missing > 0 {
		h.Missing = missing
		h.Hint = fmt.Sprintf(
			"有 %d 本书还没有章级指纹（判重比不到它们）：跑一次 norvels backfill-fingerprints 补齐", missing)
	}
	return h
}
