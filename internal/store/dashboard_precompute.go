package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// 本文件是驾驶舱三张预计算表的**写入侧**（0022）：章节长度直方图、导入趋势按日、
// 作者聚合。读侧在 dashboard_read.go，回填在 migrate_0022.go。
//
// 为什么要有这三张表：它们原本都是「请求路径上的全表聚合」。NAS 生产库
// （71,320 本 / 24,697,647 章）实测 import-trend 28.85 s、health/report 3.35 s
// （9 个驾驶舱请求并发时 25.5 s）、/api/stats 152 ms、/api/authors?sort=words 319 ms。
// 三张预计算表把成本从「每次请求」挪到「每次写入」：导入本来就要写这些行，
// 增量维护只是同一事务里多几条 UPSERT。
//
// 一致性口径（两条都靠测试守）：
//   - 增量维护的结果必须与全量重启（rebuildAll...）逐项相等；
//   - 任何一张表没建（老库没跑 0022）时，读接口降级为实时 SQL 并标 stale，绝不 500。

// precomputeVersion 是预计算口径版本，写在 store_metrics 的 precompute_version 键上。
//
// 读接口据此判断「表里的值是不是当前口径算的」：口径变了（比如长度分桶边界调整）
// 就 +1，老库的值立即被判过期并触发后台重建，而不是悄悄返回一组错数字。
//
//	v1：0022 首次引入（长度直方图 5 桶 / 导入趋势按 UTC 日 / 作者聚合）
const precomputeVersion = 1

// metricPrecomputeVersion 是 store_metrics 里记录口径版本的键。
const metricPrecomputeVersion = "precompute_version"

// metricPrecomputeZone 记录回填时本地时区相对 UTC 的偏移分钟数。
//
// 它存在的理由只有一个：import_daily 的 key 是**本地日**，而本地日的边界由时区决定。
// 部署方换了时区（或夏令时切换）之后，旧 key 落到哪一天已经不对了；把偏移一起存下来，
// 读接口一比就知道该降级重算，而不是静默返回错了一天的趋势图。
const metricPrecomputeZone = "precompute_zone_offset"

// 三张预计算表的表名（迁移 0022 建，回填与增量维护共用）。
const (
	tableChapterLengthHist = "chapter_length_hist"
	tableImportDaily       = "import_daily"
	tableAuthorAgg         = "author_agg"
)

// resetPrecomputeCache 让下一次读重新探测（迁移完成、回填完成、RefreshMetrics 后调用）。
//
// 只重置本实例：预计算的就绪状态是「这个库」的属性，不是进程的属性。
func (s *Store) resetPrecomputeCache() { s.precomputeReady.Store(false) }

// PrecomputeStatus 报告三张预计算表是否就绪，供读接口决定「读表」还是「降级实时算」。
//
// 就绪 = 三张表都存在 + store_metrics.precompute_version 等于当前口径版本
// + 记录时的**时区偏移**等于此刻的偏移（import_daily 的 key 是本地日，
// 偏移一变「哪一天」的定义就变了，旧 key 不能再直接用）。
//
// 返回 false 时附带一句原因（只进日志与 stale 标记，不进接口响应）。
func (s *Store) PrecomputeStatus(ctx context.Context) (bool, string) {
	if s.precomputeReady.Load() {
		return true, ""
	}
	for _, table := range []string{tableChapterLengthHist, tableImportDaily, tableAuthorAgg} {
		ok, err := s.tableExists(ctx, table)
		if err != nil {
			return false, fmt.Sprintf("探测 %s 失败：%v", table, err)
		}
		if !ok {
			return false, "预计算表 " + table + " 不存在（未应用迁移 0022）"
		}
	}
	version, zoneOffset, err := s.readPrecomputeMeta(ctx)
	if err != nil {
		return false, err.Error()
	}
	if version != precomputeVersion {
		return false, fmt.Sprintf("预计算口径版本 %d 落后于当前 %d", version, precomputeVersion)
	}
	if zoneOffset != currentZoneOffset() {
		return false, fmt.Sprintf("时区偏移从 %d 分钟变为 %d 分钟，导入趋势按日口径失效",
			zoneOffset, currentZoneOffset())
	}
	s.precomputeReady.Store(true)
	return true, ""
}

// readPrecomputeMeta 读预计算的口径版本与时区偏移；任一缺失返回 (0, 0, 原因)。
func (s *Store) readPrecomputeMeta(ctx context.Context) (int, int, error) {
	var version, zoneOffset int
	err := s.db.QueryRowContext(ctx,
		"SELECT "+
			"COALESCE(MAX(CASE WHEN key = ? THEN value END), -1), "+
			"COALESCE(MAX(CASE WHEN key = ? THEN value END), 0) FROM store_metrics",
		metricPrecomputeVersion, metricPrecomputeZone).Scan(&version, &zoneOffset)
	if err != nil {
		return 0, 0, fmt.Errorf("读取预计算口径失败：%v", err)
	}
	if version < 0 {
		return 0, 0, fmt.Errorf("预计算尚未回填（store_metrics 里没有 %s）", metricPrecomputeVersion)
	}
	return version, zoneOffset, nil
}

// currentZoneOffset 返回此刻本地时区相对 UTC 的偏移分钟数。
//
// 东八区是 480；它随夏令时变化，所以结果不能写死，只能每次比较。
func currentZoneOffset() int {
	_, secs := time.Now().Zone()
	return secs / 60
}

// bumpLengthHist 在章节行变化的**同一事务**内，把变化落进长度直方图。
//
// added   = 现在存在、之前不存在的章字数（新增 / 拆分出的段 / 改写后的新字数）
// removed = 之前存在、现在不存在的章字数（删除 / 被合并掉 / 改写前的旧字数）
//
// 为什么用两组切片而不是「带符号的 delta」：桶号由字数决定，负数会被 bucketIndex
// 判成 0 号桶（-5000 落进 `<1千` 就是错的）。分成「加」「减」两组，每组的字数都是
// 正数，桶号才准。同一章的字数变化 = 先减旧值、再加新值。
//
// 一次性聚合再按桶累加：几千章的导入只产生 5 条 UPSERT，而不是几千条。
func bumpLengthHist(ctx context.Context, tx *sql.Tx, added, removed []int) error {
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	buckets := make([]int, len(lengthBucketEdges))
	for _, wc := range added {
		buckets[bucketIndex(lengthBucketEdges, wc)]++
	}
	for _, wc := range removed {
		buckets[bucketIndex(lengthBucketEdges, wc)]--
	}
	return addLengthHist(ctx, tx, buckets)
}

// addLengthHist 把各桶增量（可为负）累加进 chapter_length_hist。
func addLengthHist(ctx context.Context, tx *sql.Tx, deltas []int) error {
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO "+tableChapterLengthHist+"(bucket, count) VALUES (?, ?) "+
			"ON CONFLICT(bucket) DO UPDATE SET count = count + excluded.count")
	if err != nil {
		return fmt.Errorf("store: prepare length hist: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄
	for bucket, delta := range deltas {
		if delta == 0 {
			continue
		}
		if _, err := stmt.ExecContext(ctx, bucket, delta); err != nil {
			return fmt.Errorf("store: bump length hist bucket %d: %w", bucket, err)
		}
	}
	return nil
}

// bumpImportDaily 把「某天新增了 n 本书、c 章」累加进导入趋势。
//
// importedAt 是这本书的入库时间文本（store 统一写成 UTC），按**本地日**归并——
// 与原先 countByDay 的口径一致：那边也是 parseTime 后 t.Local().Format(dateLayout)，
// 否则北京时间凌晨 3 点导入的书会被算到前一天。
//
// 章节数用 delta 传入：覆盖写入时先按负数减掉旧值（见 bumpImportDailyDelta）。
func bumpImportDaily(ctx context.Context, tx *sql.Tx, importedAt string, bookDelta, chapterDelta int) error {
	day, ok := localDayKey(importedAt)
	if !ok {
		// 时间文本解析不了（历史脏数据）就不进趋势表：趋势图少一天比整体 500 好，
		// 原实现 countByDay 对同一行也是 continue（口径保持一致）。
		return nil
	}
	if bookDelta == 0 && chapterDelta == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+tableImportDaily+"(day, book_count, chapter_count) VALUES (?, ?, ?) "+
			"ON CONFLICT(day) DO UPDATE SET book_count = book_count + excluded.book_count, "+
			"chapter_count = chapter_count + excluded.chapter_count",
		day, bookDelta, chapterDelta); err != nil {
		return fmt.Errorf("store: bump import_daily %s: %w", day, err)
	}
	return nil
}

// localDayKey 把库内 UTC 时间文本换算成本地日（YYYY-MM-DD）。
//
// 与 countByDay（降级路径）逐字同口径：parseTime → t.Local().Format(dateLayout)。
// 两条路径必须给出同一张表，这条由 TestImportTrendCacheMatchesLive 守着。
func localDayKey(raw string) (string, bool) {
	t, err := parseTime(raw)
	if err != nil {
		return "", false
	}
	if t.IsZero() {
		return "", false
	}
	return t.Local().Format(dateLayout), true
}

// refreshAuthorAgg 重算**单个作者**的聚合行（作品数与字数）。
//
// 与 refreshAuthorStats（authors 表的 book_count / word_count 列）同口径、同调用点，
// 但写的是另一张表：author_agg 是 /api/authors 的读路径，authors 的两列是既有
// 接口契约的一部分（列表里要能直接看到），两者都要维护。
//
// 单作者重算是两张索引查找（books.author_id / books_authors.author_id），
// 与 books 行数无关——所以「导入一本书」的成本不会随库规模变差。
func refreshAuthorAgg(ctx context.Context, tx *sql.Tx, authorID int64) error {
	if authorID == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+tableAuthorAgg+"(author_id, book_count, word_count) "+
			"SELECT a.id, COUNT(b.id), COALESCE(SUM(b.word_count), 0) "+
			"FROM authors a LEFT JOIN "+authorWorks+" x ON x.aid = a.id "+
			"LEFT JOIN books b ON b.id = x.book_id WHERE a.id = ? GROUP BY a.id "+
			"ON CONFLICT(author_id) DO UPDATE SET book_count = excluded.book_count, "+
			"word_count = excluded.word_count", authorID); err != nil {
		return fmt.Errorf("store: refresh author agg %d: %w", authorID, err)
	}
	return nil
}

// bookChapterWordCounts 读一本书现有各章的字数（覆盖写入前要把它们从直方图里减掉）。
//
// 走 idx_chapters_book_idx，代价与书的章数成正比（不是全库）：替换一本书的导入
// 本来就要重写它的全部章节，这次读在同一个量级里。
func bookChapterWordCounts(ctx context.Context, tx *sql.Tx, bookID int64) ([]int, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT word_count FROM chapters WHERE book_id = ?", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: read book chapter words %d: %w", bookID, err)
	}
	defer rows.Close()

	out := make([]int, 0, 64)
	for rows.Next() {
		var wc int
		if err := rows.Scan(&wc); err != nil {
			return nil, fmt.Errorf("store: scan book chapter word %d: %w", bookID, err)
		}
		out = append(out, wc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read book chapter words rows %d: %w", bookID, err)
	}
	return out, nil
}

// bookImportedAt 读一本书的入库时间文本（删书时要把它的那一行从趋势表里减掉）。
func bookImportedAt(ctx context.Context, tx *sql.Tx, bookID int64) (string, error) {
	var raw string
	err := tx.QueryRowContext(ctx, "SELECT imported_at FROM books WHERE id = ?", bookID).Scan(&raw)
	if err != nil {
		return "", fmt.Errorf("store: read imported_at of book %d: %w", bookID, err)
	}
	return raw, nil
}
