package recstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// 兴趣加权：一条事件按维度累计衰减权重。
//
// 维度拆分的理由见 dim 常量注释；负反馈（dislike）按 -2 扣、下限 0。

// interestDim 是兴趣的一个维度（分类名或作者 id）。
type interestDim struct {
	dim string
	key string
}

// 兴趣维度：分类拆成 L1 / L2 两个维度，作者一个维度。
//
// 为什么不合成一个 category：同一本书的「武侠」与「江湖」分属两个层级，混在一个维度里
// 会让看板出现「武侠 69.4 / 江湖 69.4」成对重复，召回也分不清命中凭据是大类还是细分，
// 权重更没法按层级给（设计 §3：L1 3.0 / L2 1.5）。
//
// 旧键 dim=category **直接作废**：推荐库整库可重建（设计 §1），不做数据迁移；
// 库里残留的 category 行不会再被任何读路径消费，随 90 天保留窗口自然清掉，或删库重建。
const (
	dimCategoryL1 = "category_l1"
	dimCategoryL2 = "category_l2"
	dimAuthor     = "author"
	// tag / series 的 key 用名字本身（与分类一致）：id 会随改名/重建失去可读性，
	// 而兴趣榜最终要展示给用户看，名字比 id 更适合当 key。
	dimTag    = "tag"
	dimSeries = "series"
)

// interestDims 列出这条事件该加权的维度；空分类/0 作者直接跳过。
func interestDims(ev Event) []interestDim {
	out := make([]interestDim, 0, 3+len(ev.Tags))
	if ev.CategoryL1 != "" {
		out = append(out, interestDim{dimCategoryL1, ev.CategoryL1})
	}
	if ev.CategoryL2 != "" {
		out = append(out, interestDim{dimCategoryL2, ev.CategoryL2})
	}
	if ev.AuthorID > 0 {
		out = append(out, interestDim{dimAuthor, strconv.FormatInt(ev.AuthorID, 10)})
	}
	for _, tag := range ev.Tags {
		if tag != "" {
			out = append(out, interestDim{dimTag, tag})
		}
	}
	if ev.Series != "" {
		out = append(out, interestDim{dimSeries, ev.Series})
	}
	return out
}

// interestDelta 把事件折算成兴趣增量（设计 §2 的 delta 表）。
//
// read 的增量随停留时长增长但封顶 +3：读 5 分钟才算「真读过」，读 5 小时也不该
// 等于十几次收藏——否则把阅读器挂后台就能把单一分类拱到 top1。
// search 不进兴趣：搜索词落在 extra 里是自由文本，直接当分类名会污染召回。
func interestDelta(kind string, value int) float64 {
	switch kind {
	case KindClick:
		return 1
	case KindRead:
		bonus := float64(value) / 300
		if bonus > 3 {
			bonus = 3
		}
		return 1 + bonus
	case KindShelf:
		return 3
	case KindRating:
		return 5
	case KindView:
		return 0.5
	case KindDislike:
		// 负反馈按「一次 shelf 的强度」扣：够把一次误点压下去，
		// 又不至于一次手滑就抹掉长期偏好（累计权重靠多次互动堆起来）。
		return -2
	}
	return 0
}

// bumpInterest 给这条事件涉及的每个维度累加衰减后的兴趣增量。
func (s *Store) bumpInterest(ctx context.Context, tx *sql.Tx, userID int64,
	ev Event, now time.Time) error {
	delta := interestDelta(ev.Kind, ev.Value)
	if delta == 0 {
		return nil
	}
	for _, d := range interestDims(ev) {
		if err := addInterest(ctx, tx, userID, d, delta, now); err != nil {
			return err
		}
	}
	return nil
}

// addInterest 读旧权重 → 先衰减到 now → 加 delta → 写回。
//
// 衰减在 Go 里算而不是 SQL 的 exp()：SQLite 的数学函数是编译期开关，
// 换驱动或降级构建时 exp() 会静默消失，而这里的公式是推荐分的基础。
func addInterest(ctx context.Context, tx *sql.Tx, userID int64, d interestDim,
	delta float64, now time.Time) error {
	var weight float64
	var updatedAt string
	err := tx.QueryRowContext(ctx,
		"SELECT weight, updated_at FROM user_interest WHERE user_id = ? AND dim = ? AND key = ?",
		userID, d.dim, d.key).Scan(&weight, &updatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		weight = 0 // 首次互动：没有旧权重可衰减
	case err != nil:
		return fmt.Errorf("recstore: 读兴趣权重: %w", err)
	default:
		if prev, perr := parseTime(updatedAt); perr == nil {
			weight = decay(weight, prev, now)
		}
	}
	next := weight + delta
	if next < 0 {
		next = 0 // 下限 0：负权重会让「越讨厌越像」的排序无从解释
	}
	if _, err := tx.ExecContext(ctx, upsertInterestSQL, userID, d.dim, d.key,
		next, formatTime(now)); err != nil {
		return fmt.Errorf("recstore: 写兴趣权重: %w", err)
	}
	return nil
}
