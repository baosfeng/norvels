package recstore

import (
	"context"
	"fmt"
	"sort"
)

// 推荐看板（驾驶舱）的聚合查询：按日埋点曲线、窗口汇总、当前账号的兴趣榜。
//
// 口径说明（后来者最容易搞混的一点）：
//   - daily / summary 读的是 book_daily，**全局口径**：埋点聚合表不分账号
//     （事件表里有 user_id，但按书的热度是所有人共同贡献的，拆账号反而失去意义）；
//   - interests 读的是 user_interest，**按账号**：兴趣只对本人有意义。
// 两个口径混用会得到「这台机器上所有人的兴趣平均值」这种没有消费方的数字。

// DailyStat 是某一天的埋点汇总（看板折线的一格）。
type DailyStat struct {
	Date        string `json:"date"` // YYYY-MM-DD（UTC）
	Impressions int    `json:"impressions"`
	Clicks      int    `json:"clicks"`
	Reads       int    `json:"reads"`
	Seconds     int    `json:"seconds"`
}

// Summary 是近 N 天的汇总（看板顶部的数字卡）。
type Summary struct {
	Days        int `json:"days"`
	Impressions int `json:"impressions"`
	Clicks      int `json:"clicks"`
	Reads       int `json:"reads"`
	ReadSeconds int `json:"readSeconds"`
	// ShelfAdds 是**累计**收藏数，不是窗口值：book_stats 只有当前值没有日维度，
	// 拿它当窗口值会给出一个「最近 7 天收藏了 42 次」的假数字。
	ShelfAdds int `json:"shelfAdds"`
}

// InterestRow 是兴趣榜的一行。
type InterestRow struct {
	Dim    string  `json:"dim"`    // category / author
	Key    string  `json:"key"`    // 分类名 / 作者 id 的十进制文本
	Weight float64 `json:"weight"` // 已按 14 天衰减
}

const (
	maxStatsDays     = 90 // 看板窗口上限：再长就是「考古」，30/90 天足够看趋势
	maxInterestLimit = 50 // 兴趣榜上限：分类 + 作者合计也就几十个，再多没人看
)

const (
	dailyStatsQuery = "SELECT day, COALESCE(SUM(impressions), 0), COALESCE(SUM(clicks), 0), " +
		"COALESCE(SUM(reads), 0), COALESCE(SUM(seconds), 0) " +
		"FROM book_daily WHERE day >= ? GROUP BY day ORDER BY day"
	summaryQuery = "SELECT COALESCE(SUM(impressions), 0), COALESCE(SUM(clicks), 0), " +
		"COALESCE(SUM(reads), 0), COALESCE(SUM(seconds), 0) FROM book_daily WHERE day >= ?"
	shelfTotalQuery = "SELECT COALESCE(SUM(shelf_adds), 0) FROM book_stats"
)

// DailyStats 返回最近 days 天（含今天）的按日埋点汇总，缺日补 0、升序返回。
//
// 缺日补 0 而不是省略：前端折线按数组下标画，省略一天会让曲线整体左移一格，
// 看起来像「那天没人用」和「那天数据丢了」是一样的——补 0 至少和下钻数字对得上。
func (s *Store) DailyStats(ctx context.Context, days int) ([]DailyStat, error) {
	days = clampStatsDays(days)
	today := nowUTC()
	from := dayOf(formatTime(today.AddDate(0, 0, -(days - 1))))
	rows, err := s.db.QueryContext(ctx, dailyStatsQuery, from)
	if err != nil {
		return nil, fmt.Errorf("recstore: 查按日统计: %w", err)
	}
	defer rows.Close()

	byDay := make(map[string]DailyStat, days)
	for rows.Next() {
		var d DailyStat
		if err := rows.Scan(&d.Date, &d.Impressions, &d.Clicks, &d.Reads, &d.Seconds); err != nil {
			return nil, fmt.Errorf("recstore: 扫描按日统计: %w", err)
		}
		byDay[d.Date] = d
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recstore: 遍历按日统计: %w", err)
	}

	out := make([]DailyStat, 0, days)
	for i := days - 1; i >= 0; i-- {
		day := dayOf(formatTime(today.AddDate(0, 0, -i)))
		stat := byDay[day]
		stat.Date = day
		out = append(out, stat)
	}
	return out, nil
}

// Summary 返回近 days 天的汇总；ShelfAdds 例外，取累计值（口径见字段注释）。
func (s *Store) Summary(ctx context.Context, days int) (Summary, error) {
	days = clampStatsDays(days)
	out := Summary{Days: days}
	from := dayOf(formatTime(nowUTC().AddDate(0, 0, -(days - 1))))
	if err := s.db.QueryRowContext(ctx, summaryQuery, from).Scan(
		&out.Impressions, &out.Clicks, &out.Reads, &out.ReadSeconds); err != nil {
		return Summary{}, fmt.Errorf("recstore: 查汇总: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, shelfTotalQuery).Scan(&out.ShelfAdds); err != nil {
		return Summary{}, fmt.Errorf("recstore: 查收藏总数: %w", err)
	}
	return out, nil
}

// InterestTop 返回该账号权重最高的 limit 个兴趣（已按 14 天衰减）。
//
// 排序完全确定（权重降序 → dim → key）：看板会把这张表直接渲染成榜，
// 每次刷新顺序抖动会让人以为数据在变，实际上只是 map/平手顺序不同。
func (s *Store) InterestTop(ctx context.Context, userID int64, limit int) ([]InterestRow, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("%w: user_id %d", ErrInvalidArgument, userID)
	}
	limit = clampInterestLimit(limit)
	rows, err := s.db.QueryContext(ctx,
		"SELECT dim, key, weight, updated_at FROM user_interest WHERE user_id = ?", userID)
	if err != nil {
		return nil, fmt.Errorf("recstore: 查兴趣榜: %w", err)
	}
	defer rows.Close()

	now := nowUTC()
	list := make([]InterestRow, 0, 32)
	for rows.Next() {
		var (
			row       InterestRow
			updatedAt string
		)
		if err := rows.Scan(&row.Dim, &row.Key, &row.Weight, &updatedAt); err != nil {
			return nil, fmt.Errorf("recstore: 扫描兴趣榜: %w", err)
		}
		if prev, perr := parseTime(updatedAt); perr == nil {
			row.Weight = decay(row.Weight, prev, now)
		}
		if row.Weight < minDecayedWeight {
			continue // 衰减到无意义的分量不上榜，否则榜单尾部全是 0.000001
		}
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recstore: 遍历兴趣榜: %w", err)
	}
	return topInterestRows(list, limit), nil
}

// topInterestRows 排序并截断兴趣榜（纯函数，便于单测钉住平手顺序）。
func topInterestRows(list []InterestRow, limit int) []InterestRow {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Weight != list[j].Weight {
			return list[i].Weight > list[j].Weight
		}
		if list[i].Dim != list[j].Dim {
			return list[i].Dim < list[j].Dim
		}
		return list[i].Key < list[j].Key
	})
	if len(list) > limit {
		return list[:limit]
	}
	return list
}

// clampStatsDays 把看板窗口夹进 1–90。
func clampStatsDays(days int) int {
	switch {
	case days < 1:
		return 1
	case days > maxStatsDays:
		return maxStatsDays
	default:
		return days
	}
}

// clampInterestLimit 把兴趣榜条数夹进 1–50。
func clampInterestLimit(n int) int {
	switch {
	case n < 1:
		return 1
	case n > maxInterestLimit:
		return maxInterestLimit
	default:
		return n
	}
}

// ReasonClick 是「某个推荐理由被点了多少次」（看板的理由分布）。
type ReasonClick struct {
	Reason string `json:"reason"`
	Clicks int    `json:"clicks"`
}

// CategoryClick 是「某个一级分类被点了多少次」。
type CategoryClick struct {
	CategoryL1 string `json:"categoryL1"`
	Clicks     int    `json:"clicks"`
}

// topMixLimit 是理由 / 分类分布榜的条数（与热门榜同一个「一屏看得完」标准）。
const topMixLimit = 10

const (
	reasonMixQuery = "SELECT extra, COUNT(*) FROM events WHERE kind = 'click' AND extra <> '' " +
		"AND ts >= ? GROUP BY extra ORDER BY COUNT(*) DESC, extra LIMIT ?"
	categoryClicksQuery = "SELECT category_l1, COUNT(*) FROM events WHERE kind = 'click' " +
		"AND category_l1 <> '' AND ts >= ? GROUP BY category_l1 ORDER BY COUNT(*) DESC, category_l1 LIMIT ?"
)

// ReasonMix 返回近 days 天点击最多的推荐理由（top10）。
//
// 只从 click 明细聚合：impression 不落明细（写放大护栏），拿不到理由分布；
// 想统计「曝光都在推什么」需要另开聚合列，不在本次范围。
func (s *Store) ReasonMix(ctx context.Context, days int) ([]ReasonClick, error) {
	rows, err := s.db.QueryContext(ctx, reasonMixQuery, statsWindowStart(days), topMixLimit)
	if err != nil {
		return nil, fmt.Errorf("recstore: 查理由分布: %w", err)
	}
	defer rows.Close()

	out := make([]ReasonClick, 0, topMixLimit)
	for rows.Next() {
		var r ReasonClick
		if err := rows.Scan(&r.Reason, &r.Clicks); err != nil {
			return nil, fmt.Errorf("recstore: 扫描理由分布: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recstore: 遍历理由分布: %w", err)
	}
	return out, nil
}

// CategoryClicks 返回近 days 天点击最多的一级分类（top10）。
func (s *Store) CategoryClicks(ctx context.Context, days int) ([]CategoryClick, error) {
	rows, err := s.db.QueryContext(ctx, categoryClicksQuery, statsWindowStart(days), topMixLimit)
	if err != nil {
		return nil, fmt.Errorf("recstore: 查分类点击: %w", err)
	}
	defer rows.Close()

	out := make([]CategoryClick, 0, topMixLimit)
	for rows.Next() {
		var c CategoryClick
		if err := rows.Scan(&c.CategoryL1, &c.Clicks); err != nil {
			return nil, fmt.Errorf("recstore: 扫描分类点击: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recstore: 遍历分类点击: %w", err)
	}
	return out, nil
}

// statsWindowStart 是「近 days 天」的起始日期（含今天，与 daily 同一口径）。
func statsWindowStart(days int) string {
	days = clampStatsDays(days)
	return dayOf(formatTime(nowUTC().AddDate(0, 0, -(days - 1))))
}
