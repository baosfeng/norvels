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

// bumpInterest 给这条事件涉及的每个维度累加衰减后的兴趣增量，并按事件时段再记一份。
func (s *Store) bumpInterest(ctx context.Context, tx *sql.Tx, userID int64,
	ev Event, now time.Time) error {
	delta := interestDelta(ev.Kind, ev.Value)
	if delta == 0 {
		return nil
	}
	slot := slotOfTimestamp(ev.TS)
	for _, d := range interestDims(ev) {
		if err := addInterest(ctx, tx, userID, d, delta, slot, now); err != nil {
			return err
		}
	}
	return nil
}

// 时段按 6 小时一格切 4 槽：0-5 深夜 / 6-11 上午 / 12-17 下午 / 18-23 晚上。
//
// 用进程本地时区而不是 UTC：时段是「人的作息」，容器里配了 TZ 就按当地切；
// 写入与读取用同一个时区，所以即使容器跑 UTC，也至少能正确表达「相对当前时段的偏好」。
const timeSlotHours = 6

// timeSlotBoost 是「当前时段兴趣」并入全局兴趣时的放大倍数。
//
// 1.5 而不是更多：时段偏好是「此刻想看什么」的提示，不该压过长期兴趣；
// 也不能是 1，否则它在排序上就与普通兴趣没有区别了。
const timeSlotBoost = 1.5

const upsertTimeInterestSQL = "INSERT INTO time_interest(user_id, slot, dim, key, weight, updated_at) " +
	"VALUES(?,?,?,?,?,?) ON CONFLICT(user_id, slot, dim, key) DO UPDATE SET " +
	"weight = excluded.weight, updated_at = excluded.updated_at"

// slotOf 把时刻归到 4 个时段（本地时区）。
func slotOf(t time.Time) int { return t.In(time.Local).Hour() / timeSlotHours }

// slotOfTimestamp 从库内定长 UTC 文本取时段；解析失败归到「现在」的时段，不阻断写入。
func slotOfTimestamp(ts string) int {
	t, err := parseTime(ts)
	if err != nil || t.IsZero() {
		return slotOf(nowUTC())
	}
	return slotOf(t)
}

// addInterest 同时累加全局兴趣与「事件所在时段」的兴趣（同一份 delta）。
//
// 两张表不是两套算法：时段表只是把同一份信号按时间切片再存一遍，
// 衰减与下限口径必须完全一致，否则同一个事件会在两张表里得到不同的权重。
func addInterest(ctx context.Context, tx *sql.Tx, userID int64, d interestDim,
	delta float64, slot int, now time.Time) error {
	weight, err := decayedWeight(ctx, tx, now,
		"SELECT weight, updated_at FROM user_interest WHERE user_id = ? AND dim = ? AND key = ?",
		userID, d.dim, d.key)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, upsertInterestSQL, userID, d.dim, d.key,
		clampWeight(weight+delta), formatTime(now)); err != nil {
		return fmt.Errorf("recstore: 写兴趣权重: %w", err)
	}
	slotWeight, err := decayedWeight(ctx, tx, now,
		"SELECT weight, updated_at FROM time_interest WHERE user_id = ? AND slot = ? AND dim = ? AND key = ?",
		userID, slot, d.dim, d.key)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, upsertTimeInterestSQL, userID, slot, d.dim, d.key,
		clampWeight(slotWeight+delta), formatTime(now)); err != nil {
		return fmt.Errorf("recstore: 写时段兴趣: %w", err)
	}
	return nil
}

// decayedWeight 读一条兴趣记录的权重并衰减到 now；没有记录返回 0。
//
// 衰减在 Go 里算而不是 SQL 的 exp()：SQLite 的数学函数是编译期开关，
// 换驱动或降级构建时 exp() 会静默消失，而这里的公式是推荐分的基础。
func decayedWeight(ctx context.Context, tx *sql.Tx, now time.Time,
	query string, args ...any) (float64, error) {
	var (
		weight    float64
		updatedAt string
	)
	err := tx.QueryRowContext(ctx, query, args...).Scan(&weight, &updatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, nil // 首次互动：没有旧权重可衰减
	case err != nil:
		return 0, fmt.Errorf("recstore: 读兴趣权重: %w", err)
	default:
		if prev, perr := parseTime(updatedAt); perr == nil {
			weight = decay(weight, prev, now)
		}
		return weight, nil
	}
}

// clampWeight 把兴趣权重压到非负：负权重会让「越讨厌越像」的排序无从解释。
func clampWeight(w float64) float64 {
	if w < 0 {
		return 0
	}
	return w
}

// InterestsAt 返回该账号的兴趣权重：全局兴趣 + 当前时段兴趣 ×1.5。
//
// now 显式传入是为了可测：时段取决于「现在几点」，测试要能钉住某个时刻。
// 合并后仍按 minDecayedWeight 截断已由 readInterestRows 完成，这里只做叠加。
func (s *Store) InterestsAt(ctx context.Context, userID int64, now time.Time) (map[string]float64, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("%w: user_id %d", ErrInvalidArgument, userID)
	}
	out, err := s.readInterestRows(ctx, userID, now, -1)
	if err != nil {
		return nil, err
	}
	slotRows, err := s.readInterestRows(ctx, userID, now, slotOf(now))
	if err != nil {
		return nil, err
	}
	for key, weight := range slotRows {
		out[key] += weight * timeSlotBoost
	}
	return out, nil
}

// readInterestRows 读一张兴趣表（slot<0 = 全局表），衰减到 now 并丢弃过小分量。
func (s *Store) readInterestRows(ctx context.Context, userID int64, now time.Time,
	slot int) (map[string]float64, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if slot < 0 {
		rows, err = s.db.QueryContext(ctx,
			"SELECT dim, key, weight, updated_at FROM user_interest WHERE user_id = ?", userID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			"SELECT dim, key, weight, updated_at FROM time_interest WHERE user_id = ? AND slot = ?",
			userID, slot)
	}
	if err != nil {
		return nil, fmt.Errorf("recstore: 查兴趣: %w", err)
	}
	defer rows.Close()

	out := make(map[string]float64, 16)
	for rows.Next() {
		var (
			dim, key, updatedAt string
			weight              float64
		)
		if err := rows.Scan(&dim, &key, &weight, &updatedAt); err != nil {
			return nil, fmt.Errorf("recstore: 扫描兴趣: %w", err)
		}
		if prev, perr := parseTime(updatedAt); perr == nil {
			weight = decay(weight, prev, now)
		}
		if weight < minDecayedWeight {
			continue
		}
		out[dim+":"+key] = weight
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recstore: 遍历兴趣: %w", err)
	}
	return out, nil
}
