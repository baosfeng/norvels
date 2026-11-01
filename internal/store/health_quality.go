package store

import (
	"context"
	"fmt"
)

// QualityHealth 是 books 质量分的覆盖率（迁移 0020 之后才有意义）。
//
// 为什么放进健康报告：0020 只加列、不回填存量书，而「版本择优比不到旧版本」在用户侧
// 的表现是「明明有更好的版本，却永远提示重复」——这种沉默的退化必须出现在运维看得见的
// 地方，并直接给出处置命令。
type QualityHealth struct {
	Books    int    `json:"books"`          // 主库书数
	Computed int    `json:"computed"`       // 已算质量分的书数
	Missing  int    `json:"missing"`        // 缺口 = Books − Computed
	Hint     string `json:"hint,omitempty"` // 缺口 > 0 时的处置建议（指向回填命令）
}

// qualityHealth 统计质量分覆盖率；统计失败记一条 WARN 并返回 nil（不影响其它项）。
func (s *Store) qualityHealth(ctx context.Context, r *HealthReport) *QualityHealth {
	cov, err := s.qualityCoverage(ctx)
	if err != nil {
		r.warn("quality", err)
		return nil
	}
	h := &QualityHealth{Books: cov.Books, Computed: cov.Computed, Missing: cov.Missing}
	if cov.Missing > 0 {
		h.Hint = fmt.Sprintf(
			"有 %d 本书还没有质量分（版本择优比不到它们）：跑一次 norvels backfill-quality 补齐", cov.Missing)
	}
	return h
}
