package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// metrics_version 的当前口径版本；口径变化时 +1，启动时旧库会自动重算。
const metricsVersion = 1

// store_metrics 的键位。全部是整数值，读接口只查表，毫秒级。
const (
	metricTextBytes       = "text_bytes"        // 正文：chapters 正文与字级镜像列（导入时增量累加）
	metricBookTextBytes   = "book_text_bytes"   // 正文：books 文本列（全量重算时算一次）
	metricChapterWordSum  = "chapter_word_sum"  // 全库章节字数总和
	metricIndexBytes      = "index_bytes"       // FTS 索引字节（= 库文件大小 − 正文字节，近似）
	metricMetricsVersion  = "metrics_version"   // 计算口径版本
	metricRefreshedAtUnix = "refreshed_at_unix" // 上次全量重算时间（Unix 秒）
)

// metricsSnapshot 是从 store_metrics 读出的缓存值。
type metricsSnapshot struct {
	TextBytes      int64
	ChapterWordSum int64
	IndexBytes     int64
	RefreshedAt    time.Time
	Stale          bool // true 表示缓存缺失、口径落后或与当前库规模对不上
}

// ChapterWordSumSQL 是实时求章节字数总和（整数列，42 万行实测 0.05 s）。
const chapterWordSumSQL = "SELECT COALESCE(SUM(word_count), 0) FROM chapters"

// bookTextBytesSQL 是 books 侧文本字节（838 行，全表扫也是毫秒级）。
const bookTextBytesSQL = "SELECT COALESCE(SUM(length(title) + length(intro) + length(source_path) + " +
	"length(source_name) + length(encoding) + length(status)), 0) FROM books"

// chapterTextBytesSQL 是 chapters 侧文本字节（慢查询，只允许在导入与全量重算里调用）。
const chapterTextBytesSQL = "SELECT COALESCE(SUM(length(title) + length(content) + " +
	"length(title_seg) + length(content_seg)), 0) FROM chapters"

// textBytesForBookSQL 求单本书的文本字节与字数（用于覆盖写入前先减旧值）。
const textBytesForBookSQL = "SELECT COALESCE(SUM(length(title) + length(content) + " +
	"length(title_seg) + length(content_seg)), 0), COALESCE(SUM(word_count), 0) " +
	"FROM chapters WHERE book_id = ?"

// readMetrics 读出当前缓存；值缺失或口径落后时 Stale=true。
func (s *Store) readMetrics(ctx context.Context) (metricsSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM store_metrics")
	if err != nil {
		// 未迁移的老库没有该表：按「全部过期」处理，由后台补算。
		return metricsSnapshot{Stale: true}, nil //nolint:nilerr // 缓存不可用是降级场景
	}
	defer rows.Close()

	m := make(map[string]int64, 8)
	for rows.Next() {
		var k string
		var v int64
		if err := rows.Scan(&k, &v); err != nil {
			return metricsSnapshot{Stale: true}, nil //nolint:nilerr
		}
		m[k] = v
	}
	if err := rows.Err(); err != nil {
		return metricsSnapshot{Stale: true}, nil //nolint:nilerr
	}

	snap := metricsSnapshot{
		// TextBytes 对调用方是「正文总量」，含 books 侧的少量文本；两者分开存储是为了让
		// 导入时的增量累加（只碰 chapters）与全量重算（都算）落在同一个口径上。
		TextBytes:      m[metricTextBytes] + m[metricBookTextBytes],
		ChapterWordSum: m[metricChapterWordSum],
		IndexBytes:     m[metricIndexBytes],
	}
	if ts := m[metricRefreshedAtUnix]; ts > 0 {
		snap.RefreshedAt = time.Unix(ts, 0).UTC()
	}
	// 缓存视为有效需同时满足：口径版本一致、正文值已算过。
	// 空库（0 章）也合法，此时以「版本号存在」作为已算过的判据。
	snap.Stale = m[metricMetricsVersion] != metricsVersion
	return snap, nil
}

// bumpMetrics 在给定事务内把累加量加到缓存上（可为负，用于覆盖/删除旧值）。
//
// 导入路径调用它时与章节写入处于同一事务：要么都成功，要么都回滚，
// 不会出现「章节进了库但 metrics 没记账」的漂移。
func bumpMetrics(ctx context.Context, tx *sql.Tx, textDelta, wordDelta int64) error {
	if textDelta == 0 && wordDelta == 0 {
		return nil
	}
	now := formatTime(nowUTC())
	for _, kv := range []struct {
		key   string
		delta int64
	}{
		{metricTextBytes, textDelta},
		{metricChapterWordSum, wordDelta},
	} {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO store_metrics(key, value, updated_at) VALUES (?, ?, ?) "+
				"ON CONFLICT(key) DO UPDATE SET value = value + excluded.value, updated_at = excluded.updated_at",
			kv.key, kv.delta, now); err != nil {
			return fmt.Errorf("store: bump metric %s: %w", kv.key, err)
		}
	}
	return nil
}

// bumpMetricKey 在事务内把某个键加上 delta（可为负）。
func bumpMetricKey(ctx context.Context, tx *sql.Tx, key string, delta int64) error {
	if delta == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		"INSERT INTO store_metrics(key, value, updated_at) VALUES (?, ?, ?) "+
			"ON CONFLICT(key) DO UPDATE SET value = value + excluded.value, updated_at = excluded.updated_at",
		key, delta, formatTime(nowUTC()))
	if err != nil {
		return fmt.Errorf("store: bump metric %s: %w", key, err)
	}
	return nil
}

// setMetric 在事务内把某个键写成指定值。
func setMetric(ctx context.Context, tx *sql.Tx, key string, value int64) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO store_metrics(key, value, updated_at) VALUES (?, ?, ?) "+
			"ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at",
		key, value, formatTime(nowUTC()))
	if err != nil {
		return fmt.Errorf("store: set metric %s: %w", key, err)
	}
	return nil
}

// bookTextStats 求单本书当前的（文本字节, 字数），供覆盖写入前先减旧值。
func bookTextStats(ctx context.Context, tx *sql.Tx, bookID int64) (textBytes, words int64, err error) {
	err = tx.QueryRowContext(ctx, textBytesForBookSQL, bookID).Scan(&textBytes, &words)
	return textBytes, words, err
}

// refreshMetrics 全表精确重算并写库（慢，运维与后台补算用）。
//
// 计算口径：
//   - text_bytes = books 文本列 + chapters 正文与字级镜像列（唯一必须扫文本列的项）
//   - chapter_word_sum = SUM(chapters.word_count)（整数列，本身就快，顺手缓存）
//   - index_bytes = 库文件大小 − text_bytes（近似：其余页填充/表头/空闲页也算进索引侧）
func (s *Store) refreshMetrics(ctx context.Context) error {
	bookText, err := s.scalarInt64(ctx, bookTextBytesSQL)
	if err != nil {
		return fmt.Errorf("store: metrics book text: %w", err)
	}
	chapterText, err := s.scalarInt64(ctx, chapterTextBytesSQL)
	if err != nil {
		return fmt.Errorf("store: metrics chapter text: %w", err)
	}
	wordSum, err := s.scalarInt64(ctx, chapterWordSumSQL)
	if err != nil {
		return fmt.Errorf("store: metrics chapter word sum: %w", err)
	}

	// 口径：text_bytes 只统计 chapters 侧，与导入时的增量累加完全一致——
	// 这样「增量累加」与「全量重算」必然得到同一个数（有测试守着这条）。
	// books 侧文本量很小（838 行实测 174 KB），单独记 book_text_bytes，读接口相加。
	textBytes := chapterText
	indexBytes := s.diskUsage() - textBytes - bookText
	if indexBytes < 0 {
		indexBytes = 0
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: metrics begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作

	for _, kv := range []struct {
		key   string
		value int64
	}{
		{metricTextBytes, textBytes},
		{metricBookTextBytes, bookText},
		{metricChapterWordSum, wordSum},
		{metricIndexBytes, indexBytes},
		{metricMetricsVersion, metricsVersion},
		{metricRefreshedAtUnix, nowUTC().Unix()},
	} {
		if err := setMetric(ctx, tx, kv.key, kv.value); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: metrics commit: %w", err)
	}
	return nil
}

// RefreshMetrics 全表精确重算库级聚合值（运维入口，13.8 GiB 库约需 2 分钟）。
func (s *Store) RefreshMetrics(ctx context.Context) error { return s.refreshMetrics(ctx) }

// ensureMetrics 在缓存缺失或口径落后时后台补算一次，本调用立即返回。
//
// 启动路径调用它：不阻塞启动，算完前读接口返回 stale 标记。并发多次调用只会有一个
// 真正开工（用 atomic 标志去重）。
func (s *Store) ensureMetrics(ctx context.Context) {
	snap, err := s.readMetrics(ctx)
	if err != nil || !snap.Stale {
		return
	}
	if !s.metricsRefreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.metricsRefreshing.Store(false)
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
		defer cancel()
		err := s.refreshMetrics(rctx)
		if s.staleCallback != nil {
			s.staleCallback(err)
		}
	}()
}

// EnsureMetricsAsync 是 ensureMetrics 的导出别名：启动路径调用它触发后台补算。
func (s *Store) EnsureMetricsAsync(ctx context.Context) { s.ensureMetrics(ctx) }

// OnMetricsRefreshed 注册后台补算完成后的回调（nil 表示不关心）。启动前调用。
func (s *Store) OnMetricsRefreshed(fn func(err error)) { s.staleCallback = fn }

// Metrics 返回库级聚合值（毫秒级，只查预计算表）。
func (s *Store) Metrics(ctx context.Context) (metricsSnapshot, error) {
	snap, err := s.readMetrics(ctx)
	if err != nil {
		return metricsSnapshot{}, err
	}
	snap.Stale = snap.Stale || errors.Is(err, sql.ErrNoRows)
	return snap, nil
}
