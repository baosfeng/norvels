package recstore

import (
	"context"
	"fmt"
	"time"
)

// 本文件是推荐库的读侧：兴趣、共读、热度、曝光与已读集合。
//
// 全部只读、不开事务：推荐请求是无副作用的 GET（设计 §4），读侧起事务只会让
// WAL 快照多活一会儿，反过来挡住后续埋点写入。

// cooccurQuery 查「读过这本书的人也读过」的邻居，排除自己已读过的书。
//
// 已读的书在重排阶段也会被过滤（已读书不推），这里提前排掉是为了不让 limit
// 被已知答案吃掉——limit=50 时用户读过的 30 本会把真正的邻居挤出结果。
const cooccurQuery = "SELECT c.book_id, c.other_id, c.users FROM cooccur c " +
	"WHERE c.book_id IN (SELECT book_id FROM events WHERE user_id = ? AND kind = 'read' AND book_id > 0) " +
	"AND c.other_id NOT IN (SELECT book_id FROM events WHERE user_id = ? AND kind = 'read' AND book_id > 0) " +
	"ORDER BY c.users DESC, c.book_id, c.other_id LIMIT ?"

// hotBooksQuery 按近 N 天的 book_daily 汇总热度。
//
// 只有 read/click 参与排序，纯曝光不算热度：曝光是「被排到过」，不是用户选择，
// 把它算进热度会让冷启动的热门榜变成「上一次推荐了什么」。
const hotBooksQuery = "SELECT book_id, SUM(reads), SUM(clicks), SUM(seconds) FROM book_daily " +
	"WHERE day >= ? GROUP BY book_id HAVING SUM(reads) > 0 OR SUM(clicks) > 0 " +
	"ORDER BY SUM(reads) DESC, SUM(clicks) DESC, book_id LIMIT ?"

// statsQuery 一次取回全部诊断计数：分六次查询会把同一个快照读成六个时刻。
const statsQuery = "SELECT (SELECT COUNT(*) FROM events), (SELECT COUNT(*) FROM book_stats), " +
	"(SELECT COUNT(DISTINCT user_id) FROM events), (SELECT COUNT(*) FROM user_interest), " +
	"(SELECT COUNT(*) FROM cooccur), (SELECT COUNT(*) FROM recent_items)"

// Interests 返回该账号按 14 天指数衰减后的兴趣权重。
//
// key 形如 "category_l1:武侠" / "category_l2:江湖" / "author:12"，与 recommend.Signals.Interest 同一口径；
// 衰减在这里再做一次而不是只靠写入时衰减：用户几天不互动，读侧的旧权重必须继续变轻。
func (s *Store) Interests(ctx context.Context, userID int64) (map[string]float64, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("%w: user_id %d", ErrInvalidArgument, userID)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT dim, key, weight, updated_at FROM user_interest WHERE user_id = ?", userID)
	if err != nil {
		return nil, fmt.Errorf("recstore: 查兴趣: %w", err)
	}
	defer rows.Close()

	out := make(map[string]float64, 16)
	now := nowUTC()
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

// Cooccur 返回该用户「读过 A 的人也在读 B」的邻居对（按共读人数降序）。
//
// 返回的是**对**而不是按邻居聚合的结果：同一本邻居书可能由多本已读书同时贡献，
// 聚合口径（取最大还是求和）属于召回策略，交给 api 层决定，数据层不预设。
func (s *Store) Cooccur(ctx context.Context, userID int64, limit int) ([]Cooccur, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("%w: user_id %d", ErrInvalidArgument, userID)
	}
	rows, err := s.db.QueryContext(ctx, cooccurQuery, userID, userID,
		clampLimit(limit, defaultCooccurLimit))
	if err != nil {
		return nil, fmt.Errorf("recstore: 查共读: %w", err)
	}
	defer rows.Close()

	out := make([]Cooccur, 0, 32)
	for rows.Next() {
		var c Cooccur
		if err := rows.Scan(&c.BookID, &c.OtherID, &c.Users); err != nil {
			return nil, fmt.Errorf("recstore: 扫描共读: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recstore: 遍历共读: %w", err)
	}
	return out, nil
}

// HotBooks 返回近 N 天热度最高的书（reads 优先、clicks 次之）。
func (s *Store) HotBooks(ctx context.Context, days, limit int) ([]HotBook, error) {
	days = clampDays(days)
	from := nowUTC().AddDate(0, 0, -(days - 1)) // 含今天：近 7 天 = 今天 + 前 6 天
	rows, err := s.db.QueryContext(ctx, hotBooksQuery, dayOf(formatTime(from)),
		clampLimit(limit, defaultCooccurLimit))
	if err != nil {
		return nil, fmt.Errorf("recstore: 查热度: %w", err)
	}
	defer rows.Close()

	out := make([]HotBook, 0, 32)
	for rows.Next() {
		var h HotBook
		if err := rows.Scan(&h.BookID, &h.Reads, &h.Clicks, &h.Seconds); err != nil {
			return nil, fmt.Errorf("recstore: 扫描热度: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recstore: 遍历热度: %w", err)
	}
	return out, nil
}

// RecentImpressions 返回 since 之后曝光过的书集合（去重后的「近期已曝光」）。
//
// since 为零值时 formatTime 得到 "0001-01-01T…"，小于任何真实事件时间，
// 于是同一句 SQL 天然表示「不限时间」，不必为它再写一个查询分支。
func (s *Store) RecentImpressions(ctx context.Context, userID int64,
	since time.Time) (map[int64]bool, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("%w: user_id %d", ErrInvalidArgument, userID)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT book_id FROM recent_items WHERE user_id = ? AND ts >= ?",
		userID, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("recstore: 查近期曝光: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]bool, 64)
	for rows.Next() {
		var bookID int64
		if err := rows.Scan(&bookID); err != nil {
			return nil, fmt.Errorf("recstore: 扫描近期曝光: %w", err)
		}
		out[bookID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recstore: 遍历近期曝光: %w", err)
	}
	return out, nil
}

// ReadBooks 返回该账号读过的书集合（read 事件口径，与共读计数同源）。
//
// 用 events 明细而不是阅读会话表：推荐库不看主库，读没读过只认「本库有 read 事件」，
// 两库解耦是设计前提（§7），跨库查一次就破功了。
func (s *Store) ReadBooks(ctx context.Context, userID int64) (map[int64]bool, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("%w: user_id %d", ErrInvalidArgument, userID)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT DISTINCT book_id FROM events WHERE user_id = ? AND kind = 'read' AND book_id > 0",
		userID)
	if err != nil {
		return nil, fmt.Errorf("recstore: 查已读: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]bool, 32)
	for rows.Next() {
		var bookID int64
		if err := rows.Scan(&bookID); err != nil {
			return nil, fmt.Errorf("recstore: 扫描已读: %w", err)
		}
		out[bookID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recstore: 遍历已读: %w", err)
	}
	return out, nil
}

// Stats 返回推荐库的诊断计数。
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx, statsQuery).Scan(&st.Events, &st.Books, &st.Users,
		&st.Interests, &st.Cooccur, &st.RecentItems)
	if err != nil {
		return Stats{}, fmt.Errorf("recstore: 读统计: %w", err)
	}
	return st, nil
}
