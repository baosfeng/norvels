package recstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 本文件是推荐库的**唯一写入口**：所有事件都从 RecordEvents 进库。
//
// 为什么集中在一处：写放大护栏（设计 §2）依赖「一个请求一个事务」和「impression
// 只进聚合不落明细」这两个前提。散落的写路径迟早会有人漏掉其中一条，
// 而漏掉的代价是推荐库无上限膨胀。

const insertEventSQL = "INSERT INTO events(user_id, kind, book_id, author_id, " +
	"category_l1, category_l2, value, extra, ts) VALUES(?,?,?,?,?,?,?,?,?)"

// last_at 用 CASE 取较大值：事件是客户端补发的，乱序到达很常见，
// 直接覆盖会让「最近一次互动」回退到更早的时间。
const upsertBookStatsSQL = "INSERT INTO book_stats(book_id, impressions, clicks, reads, " +
	"read_seconds, shelf_adds, last_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(book_id) DO UPDATE SET " +
	"impressions = book_stats.impressions + excluded.impressions, " +
	"clicks = book_stats.clicks + excluded.clicks, " +
	"reads = book_stats.reads + excluded.reads, " +
	"read_seconds = book_stats.read_seconds + excluded.read_seconds, " +
	"shelf_adds = book_stats.shelf_adds + excluded.shelf_adds, " +
	"last_at = CASE WHEN excluded.last_at > book_stats.last_at " +
	"THEN excluded.last_at ELSE book_stats.last_at END"

const upsertBookDailySQL = "INSERT INTO book_daily(book_id, day, impressions, clicks, reads, seconds) " +
	"VALUES(?,?,?,?,?,?) ON CONFLICT(book_id, day) DO UPDATE SET " +
	"impressions = book_daily.impressions + excluded.impressions, " +
	"clicks = book_daily.clicks + excluded.clicks, " +
	"reads = book_daily.reads + excluded.reads, " +
	"seconds = book_daily.seconds + excluded.seconds"

const upsertRecentSQL = "INSERT INTO recent_items(user_id, book_id, ts) VALUES(?,?,?) " +
	"ON CONFLICT(user_id, book_id) DO UPDATE SET ts = excluded.ts"

const upsertInterestSQL = "INSERT INTO user_interest(user_id, dim, key, weight, updated_at) " +
	"VALUES(?,?,?,?,?) ON CONFLICT(user_id, dim, key) DO UPDATE SET " +
	"weight = excluded.weight, updated_at = excluded.updated_at"

// 共读双向各写一行：查询时不必写 OR，单次主键前缀扫描即可（代价是两倍行数）。
const upsertCooccurSQL = "INSERT INTO cooccur(book_id, other_id, users) VALUES(?,?,1) " +
	"ON CONFLICT(book_id, other_id) DO UPDATE SET users = cooccur.users + 1"

// RecordEvents 在一个事务里批量写入事件并更新聚合，返回已接受的条数。
//
// impression 也计入返回值（对调用方同样是已处理的一条），但只有非 impression
// 事件落 events 明细——曝光量级最大，明细对推荐排序没有增量价值。
func (s *Store) RecordEvents(ctx context.Context, userID int64, events []Event) (int, error) {
	if userID <= 0 {
		return 0, fmt.Errorf("%w: user_id %d", ErrInvalidArgument, userID)
	}
	if len(events) == 0 {
		return 0, nil
	}
	if len(events) > MaxEventBatch {
		return 0, fmt.Errorf("%w: 单次 %d 条超过上限 %d",
			ErrInvalidArgument, len(events), MaxEventBatch)
	}
	now := nowUTC()
	prepared, err := normalizeAll(events, now)
	if err != nil {
		return 0, err
	}
	// 已读集合在事务外一次读出：共读计数要在同一事务内看到本批次新读的书，
	// 所以往下传的是可写 map，applyEvent 每写入一条 read 就把它加进去。
	readSet, err := s.ReadBooks(ctx, userID)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("recstore: 开事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, ev := range prepared {
		if err := s.applyEvent(ctx, tx, userID, ev, readSet, now); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("recstore: 提交事件: %w", err)
	}
	return len(prepared), nil
}

// normalizeAll 逐条归一化，任意一条非法则整批拒绝。
//
// 整批拒绝而不是「跳过坏的写好的」：批量上报是前端的重试单位，混合接受会让
// 客户端无法判断该重发哪一条，最终选择整批重发并造成翻倍写入。
func normalizeAll(events []Event, now time.Time) ([]Event, error) {
	out := make([]Event, len(events))
	for i, ev := range events {
		n, err := normalizeEvent(ev, now)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// normalizeEvent 校验并归一化一条事件：kind 白名单、必带 book、字段长度、时间格式。
func normalizeEvent(ev Event, now time.Time) (Event, error) {
	if !validKind(ev.Kind) {
		return Event{}, fmt.Errorf("%w: 未知 kind %q", ErrInvalidArgument, ev.Kind)
	}
	if ev.Kind != KindSearch && ev.BookID <= 0 {
		return Event{}, fmt.Errorf("%w: %s 缺少 book_id", ErrInvalidArgument, ev.Kind)
	}
	if len(ev.CategoryL1) > maxFieldLen || len(ev.CategoryL2) > maxFieldLen {
		return Event{}, fmt.Errorf("%w: 分类字段超过 %d 字节", ErrInvalidArgument, maxFieldLen)
	}
	if len(ev.Extra) > maxExtraLen {
		return Event{}, fmt.Errorf("%w: extra 超过 %d 字节", ErrInvalidArgument, maxExtraLen)
	}
	ts, err := eventTime(ev.TS, now)
	if err != nil {
		return Event{}, err
	}
	ev.TS = ts
	if ev.Value < 0 {
		ev.Value = 0 // 负值只可能来自坏客户端，夹成 0 而不是让它把聚合值拉低
	}
	return ev, nil
}

// validKind 是事件白名单。白名单而不是黑名单：新 kind 必须先在这里显式登记，
// 否则它进不了聚合口径，落库只会制造没人消费的垃圾行。
func validKind(kind string) bool {
	switch kind {
	case KindImpression, KindClick, KindRead, KindView, KindSearch, KindShelf, KindRating:
		return true
	}
	return false
}

// eventTime 归一化事件时间：空串取服务端当前时间，非法格式报错（不静默变成现在）。
func eventTime(raw string, now time.Time) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return formatTime(now), nil
	}
	t, err := parseTime(raw)
	if err != nil {
		return "", fmt.Errorf("%w: ts %q", ErrInvalidArgument, raw)
	}
	if t.IsZero() {
		return formatTime(now), nil
	}
	return formatTime(t), nil
}

// applyEvent 写一条事件：明细（非 impression）+ 聚合 + 兴趣 + 共读。
func (s *Store) applyEvent(ctx context.Context, tx *sql.Tx, userID int64,
	ev Event, readSet map[int64]bool, now time.Time) error {
	if ev.Kind != KindImpression {
		if _, err := tx.ExecContext(ctx, insertEventSQL, userID, ev.Kind, ev.BookID,
			ev.AuthorID, ev.CategoryL1, ev.CategoryL2, ev.Value, ev.Extra, ev.TS); err != nil {
			return fmt.Errorf("recstore: 写事件明细: %w", err)
		}
		if err := s.bumpInterest(ctx, tx, userID, ev, now); err != nil {
			return err
		}
	}
	if err := s.bumpAggregates(ctx, tx, ev); err != nil {
		return err
	}
	if ev.Kind == KindImpression {
		return upsertRecentItem(ctx, tx, userID, ev.BookID, ev.TS)
	}
	return s.bumpCooccur(ctx, tx, ev, readSet)
}

// bookDelta 是一条事件折算成聚合列的增量（book_stats 与 book_daily 共用）。
type bookDelta struct {
	impressions int
	clicks      int
	reads       int
	seconds     int
	shelf       int
}

// deltaOf 按 kind 折算增量；view / search / rating 没有对应聚合列，只更新 last_at。
func deltaOf(kind string, value int) bookDelta {
	switch kind {
	case KindImpression:
		return bookDelta{impressions: 1}
	case KindClick:
		return bookDelta{clicks: 1}
	case KindRead:
		return bookDelta{reads: 1, seconds: value}
	case KindShelf:
		return bookDelta{shelf: 1}
	}
	return bookDelta{}
}

// hasDaily 判断是否需要动按日聚合：只有曝光/点击/阅读有日维度信号。
func (d bookDelta) hasDaily() bool {
	return d.impressions != 0 || d.clicks != 0 || d.reads != 0 || d.seconds != 0
}

// bumpAggregates 更新 book_stats（总是，用于 last_at）与 book_daily（有日信号时）。
//
// 两条 upsert 分开写而不是合成一条：book_daily 是 WITHOUT ROWID 的聚簇表，
// 曝光事件量大，能省掉一次写就该省。
func (s *Store) bumpAggregates(ctx context.Context, tx *sql.Tx, ev Event) error {
	if ev.BookID <= 0 {
		return nil // search 没有书，热度与曝光都无从归属
	}
	d := deltaOf(ev.Kind, ev.Value)
	if _, err := tx.ExecContext(ctx, upsertBookStatsSQL, ev.BookID, d.impressions,
		d.clicks, d.reads, d.seconds, d.shelf, ev.TS); err != nil {
		return fmt.Errorf("recstore: 更新 book_stats: %w", err)
	}
	if !d.hasDaily() {
		return nil
	}
	if _, err := tx.ExecContext(ctx, upsertBookDailySQL, ev.BookID, dayOf(ev.TS),
		d.impressions, d.clicks, d.reads, d.seconds); err != nil {
		return fmt.Errorf("recstore: 更新 book_daily: %w", err)
	}
	return nil
}

// interestDim 是兴趣的一个维度（分类名或作者 id）。
type interestDim struct {
	dim string
	key string
}

const (
	dimCategory = "category"
	dimAuthor   = "author"
)

// interestDims 列出这条事件该加权的维度；空分类/0 作者直接跳过。
func interestDims(ev Event) []interestDim {
	out := make([]interestDim, 0, 3)
	if ev.CategoryL1 != "" {
		out = append(out, interestDim{dimCategory, ev.CategoryL1})
	}
	if ev.CategoryL2 != "" {
		out = append(out, interestDim{dimCategory, ev.CategoryL2})
	}
	if ev.AuthorID > 0 {
		out = append(out, interestDim{dimAuthor, strconv.FormatInt(ev.AuthorID, 10)})
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
	}
	return 0
}

// bumpInterest 给这条事件涉及的每个维度累加衰减后的兴趣增量。
func (s *Store) bumpInterest(ctx context.Context, tx *sql.Tx, userID int64,
	ev Event, now time.Time) error {
	delta := interestDelta(ev.Kind, ev.Value)
	if delta <= 0 {
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
	if _, err := tx.ExecContext(ctx, upsertInterestSQL, userID, d.dim, d.key,
		weight+delta, formatTime(now)); err != nil {
		return fmt.Errorf("recstore: 写兴趣权重: %w", err)
	}
	return nil
}

// bumpCooccur 为这条 read 事件与「此前读过的书」双向累计共读。
//
// 只对 read 计数：点击/曝光代表「有兴趣」，读（=AddReadingSession 成功）才代表
// 「真读过」，把点击算进共读会让推荐流被标题党主导。
func (s *Store) bumpCooccur(ctx context.Context, tx *sql.Tx, ev Event,
	readSet map[int64]bool) error {
	if ev.Kind != KindRead || ev.BookID <= 0 {
		return nil
	}
	for other := range readSet {
		if other == ev.BookID {
			continue
		}
		if err := bumpPair(ctx, tx, ev.BookID, other); err != nil {
			return err
		}
		if err := bumpPair(ctx, tx, other, ev.BookID); err != nil {
			return err
		}
	}
	readSet[ev.BookID] = true
	return nil
}

// bumpPair 单方向累计一次共读。
func bumpPair(ctx context.Context, tx *sql.Tx, bookID, otherID int64) error {
	if _, err := tx.ExecContext(ctx, upsertCooccurSQL, bookID, otherID); err != nil {
		return fmt.Errorf("recstore: 更新共读: %w", err)
	}
	return nil
}

// upsertRecentItem 记录一条曝光（同一本书只留最近一次时间）。
func upsertRecentItem(ctx context.Context, tx *sql.Tx, userID, bookID int64, ts string) error {
	if _, err := tx.ExecContext(ctx, upsertRecentSQL, userID, bookID, ts); err != nil {
		return fmt.Errorf("recstore: 更新曝光记录: %w", err)
	}
	return nil
}

// RecordReading 是阅读事件便捷写入，供 AddReadingSession 成功后 best-effort 调用。
//
// 秒数在这里夹到 [0, maxReadSeconds] 而不报错：阅读会话本身已经校验过时长，
// 走到这一步再失败只会让一次成功的阅读上报连带失败，而且客户端也无从补救。
func (s *Store) RecordReading(ctx context.Context, userID, bookID, authorID int64,
	l1, l2 string, seconds int, ts string) error {
	if seconds < 0 {
		seconds = 0
	}
	if seconds > maxReadSeconds {
		seconds = maxReadSeconds
	}
	_, err := s.RecordEvents(ctx, userID, []Event{{
		Kind:       KindRead,
		BookID:     bookID,
		AuthorID:   authorID,
		CategoryL1: l1,
		CategoryL2: l2,
		Value:      seconds,
		TS:         ts,
	}})
	return err
}
