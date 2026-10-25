package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// metricsVersion 是当前口径版本；口径变化时 +1，启动时旧库会自动重算。
//
//	v1：text_bytes = chapters 的 TITLE+CONTENT 字符数（含字级分词镜像列）
//	v2：text_bytes = chapter_blocks.packed 压缩字节之和；原文逻辑字节另记 text_bytes_raw
const metricsVersion = 2

// store_metrics 的键位。全部是整数值，读接口只查表，毫秒级。
const (
	metricTextBytes       = "text_bytes"        // 正文：压缩后 packed 字节之和（导入时增量累加）
	metricTextBytesRaw    = "text_bytes_raw"    // 正文：压缩前的原文逻辑字节（供文档说明压缩比）
	metricBookTextBytes   = "book_text_bytes"   // 正文：books 文本列（全量重算时算一次）
	metricChapterWordSum  = "chapter_word_sum"  // 全库章节字数总和
	metricIndexBytes      = "index_bytes"       // 索引类字节（= 库文件大小 − 正文字节，近似）
	metricMetricsVersion  = "metrics_version"   // 计算口径版本
	metricRefreshedAtUnix = "refreshed_at_unix" // 上次全量重算时间（Unix 秒）
)

// metricsSnapshot 是从 store_metrics 读出的缓存值。
type metricsSnapshot struct {
	TextBytes      int64 // 压缩后 packed 字节 + books 侧文本
	TextBytesRaw   int64 // 压缩前的原文逻辑字节（books 侧同样计入）
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

// chapterTextBytesSQL 是 chapters 侧**压缩后**的正文字节（只允许在导入与全量重算里调用）。
//
// 走 SUM(length(packed)) 是扫 BLOB 列，比扫 TEXT 原文快得多（压缩后只有原文的 1/3），
// 但仍只适合全量重算，不能放在请求路径上。
const chapterTextBytesSQL = "SELECT COALESCE(SUM(length(packed)), 0) FROM chapter_blocks"

// chapterRawBytesSQL 是 chapters 侧**压缩前**的原文逻辑字节。
const chapterRawBytesSQL = "SELECT COALESCE(SUM(raw_bytes), 0) FROM chapter_blocks"

// textBytesForBookSQL 求单本书的压缩后字节、原文逻辑字节与字数（覆盖写入前先减旧值）。
const textBytesForBookSQL = "SELECT " +
	"COALESCE((SELECT SUM(length(packed)) FROM chapter_blocks WHERE book_id = ?), 0), " +
	"COALESCE((SELECT SUM(raw_bytes) FROM chapter_blocks WHERE book_id = ?), 0), " +
	"COALESCE((SELECT SUM(word_count) FROM chapters WHERE book_id = ?), 0)"

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
		//
		// v2 起口径是**压缩后**的 packed 字节；TextBytesRaw 是原文逻辑字节，
		// 供文档与前端说明「压缩比」，不参与体积构成的计算。
		TextBytes:      m[metricTextBytes] + m[metricBookTextBytes],
		TextBytesRaw:   m[metricTextBytesRaw] + m[metricBookTextBytes],
		ChapterWordSum: m[metricChapterWordSum],
		IndexBytes:     m[metricIndexBytes],
	}
	if ts := m[metricRefreshedAtUnix]; ts > 0 {
		snap.RefreshedAt = time.Unix(ts, 0).UTC()
	}
	// 缓存有效的判据：口径版本一致 + 聚合值确实算过。
	//
	// 「算过」不能只看 metrics_version 存在：迁移重建表时会写入 metrics_version，
	// 但此时 text_bytes 还是 0。若只比值版本号，重建后的库会被永远判为「有效」，
	// 后台重算再也不会触发（真实踩过）。因此额外要求 text_bytes 落过值，
	// 或者库本来就没有章节（空库的 0 是合法结果）。
	_, hasText := m[metricTextBytes]
	_, hasWordSum := m[metricChapterWordSum]
	computed := hasText && hasWordSum
	if snap.TextBytes == 0 && snap.ChapterWordSum == 0 {
		// 空库场景：章节数为 0 时 0 就是正确答案，视为已算过。
		computed = computed && s.chapterCountIsZero(ctx)
	}
	snap.Stale = m[metricMetricsVersion] != metricsVersion || !computed
	return snap, nil
}

// chapterCountIsZero 判断库里是否确实没有章节（用于区分「空库」与「尚未计算」）。
func (s *Store) chapterCountIsZero(ctx context.Context) bool {
	n, err := s.scalarInt64(ctx, chapterCountSQL)
	return err == nil && n == 0
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

// bookStats 是单本书的正文口径快照，供覆盖写入前先减旧值。
type bookStats struct {
	packedBytes int64 // 压缩后 packed 字节之和
	rawBytes    int64 // 原文逻辑字节之和
	words       int64 // 章节字数之和
}

// bookTextStats 求单本书当前的正文口径快照（覆盖写入前要先从 metrics 里减掉）。
func bookTextStats(ctx context.Context, tx *sql.Tx, bookID int64) (bookStats, error) {
	var st bookStats
	err := tx.QueryRowContext(ctx, textBytesForBookSQL, bookID, bookID, bookID).
		Scan(&st.packedBytes, &st.rawBytes, &st.words)
	return st, err
}

// refreshMetrics 全表精确重算并写库（慢，运维与后台补算用）。
//
// 计算口径：
//   - text_bytes = chapters 标题与正文（唯一必须扫文本列的项）
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
	chapterRaw, err := s.scalarInt64(ctx, chapterRawBytesSQL)
	if err != nil {
		return fmt.Errorf("store: metrics chapter raw bytes: %w", err)
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
		{metricTextBytesRaw, chapterRaw},
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
