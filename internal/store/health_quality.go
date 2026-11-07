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
//
// 0026 起两个数都读 book_totals（4 行小表，亚毫秒）：这条 SQL 曾是 health/report 上
// 唯一的全表扫——生产 182,000 本实测 p95 5500 ms / max 6760 ms，把整个接口拖到秒级。
// 预计算不可用时退回实时单次扫描（数字同一口径），接口照常 200。
func (s *Store) qualityHealth(ctx context.Context, r *HealthReport, pre healthPrecompute) *QualityHealth {
	cov, err := s.qualityCoverageOf(ctx, pre)
	if err != nil {
		r.warn("quality", err)
		return nil
	}
	return qualityHealthOf(cov)
}

// qualityCoverageOf 取「已算 / 未算」两档书数：优先预计算，不可用时实时算。
func (s *Store) qualityCoverageOf(ctx context.Context, pre healthPrecompute) (QualityCoverage, error) {
	if pre.totalsReady {
		return QualityCoverage{
			Books:    pre.totals.BooksAll,
			Computed: pre.totals.QualityComputed,
			Missing:  pre.totals.BooksAll - pre.totals.QualityComputed,
		}, nil
	}
	return s.qualityCoverage(ctx)
}

// qualityHealthOf 把覆盖率翻成健康报告里的那一项（缺口 > 0 时补处置建议）。
func qualityHealthOf(cov QualityCoverage) *QualityHealth {
	h := &QualityHealth{Books: cov.Books, Computed: cov.Computed, Missing: cov.Missing}
	if cov.Missing > 0 {
		h.Hint = fmt.Sprintf(
			"有 %d 本书还没有质量分（版本择优比不到它们）：跑一次 norvels backfill-quality 补齐", cov.Missing)
	}
	return h
}
