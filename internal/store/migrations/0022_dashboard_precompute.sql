-- 0022 驾驶舱聚合预计算：把请求路径上的全表扫描挪到写入路径。
--
-- 背景（NAS 生产库 71,320 本 / 24,697,647 章 / 主库 2.8 GB / 正文库 59 GB 实测）：
--
--   GET /api/stats/import-trend?days=14   28.85 s
--   GET /api/health/report                3.35 s（9 个驾驶舱请求并发时 25.5 s）
--   GET /api/stats                          152 ms
--   GET /api/authors?sort=words             319 ms
--
-- 四处根因都是「请求路径上做全表聚合」：
--
-- 1. import-trend 的章节按日：原先是
--      SELECT b.imported_at FROM chapters c JOIN books b ON b.id = c.book_id
--      WHERE b.imported_at >= ?
--    24,697,647 行被拉进 Go，每行一次 parseTime。书只有 7 万行，
--    章节数在 books.chapter_count 上本来就有，按书归日即可：
--      SELECT imported_at, chapter_count FROM books WHERE imported_at >= ?
-- 2. 章节长度直方图（health/report）：五段区间 COUNT，每段都要扫
--    idx_chapters_wc 的全部 2,469 万个索引项。
-- 3. /api/stats 的 SELECT COUNT(*) FROM chapters：每次请求都数 2,469 万行。
-- 4. /api/authors 的 authors LEFT JOIN (UNION 子查询) LEFT JOIN books GROUP BY：
--    UNION 两个分支各扫一遍 books / books_authors 再全量聚合。
--
-- 本迁移只建**空表**，数据由 Go 侧在同一事务后段回填（migrationStepsByVersion[22]）：
-- 回填要在事务里先查再写，SQL 文件里做不到，而回填与建表必须同事务
-- （否则中途失败会留下「表在、值是 0」的库，读接口只能一直报 stale）。
--
-- 三张表都可从业务表重算，删掉无数据损失（运维入口：norvels refresh-metrics）。

-- 章节长度直方图：一行一个桶，桶号与 store.lengthBucketEdges 的下标一一对应。
-- 读接口一次 SELECT 拿全 5 桶，不再碰 chapters 表。
CREATE TABLE IF NOT EXISTS chapter_length_hist (
    bucket INTEGER PRIMARY KEY,
    count  INTEGER NOT NULL DEFAULT 0
);

-- 导入趋势按日聚合：一天一行，book_count / chapter_count 是当天入库的书与章。
-- 读接口按日期范围取，不再 JOIN chapters。
CREATE TABLE IF NOT EXISTS import_daily (
    day           TEXT PRIMARY KEY,  -- YYYY-MM-DD（UTC 日；读取侧按本地时区映射）
    book_count    INTEGER NOT NULL DEFAULT 0,
    chapter_count INTEGER NOT NULL DEFAULT 0
);

-- 作者聚合物化：一行一位作者，覆盖 /api/authors 的三种排序。
-- 为什么物化而不是补覆盖索引：排序键是「该作者名下所有书的字数之和」，
-- 跨越 books_authors 与 books 两张表，任何单表索引都覆盖不了这个聚合。
CREATE TABLE IF NOT EXISTS author_agg (
    author_id  INTEGER PRIMARY KEY,
    book_count INTEGER NOT NULL DEFAULT 0,
    word_count INTEGER NOT NULL DEFAULT 0
);

-- 按字数倒序取 Top N（驾驶舱「字数 Top 10」）：没有它就要全表聚合后排序。
CREATE INDEX IF NOT EXISTS idx_author_agg_words ON author_agg(word_count DESC, author_id);
-- 按作品数倒序（作者页默认排序）。
CREATE INDEX IF NOT EXISTS idx_author_agg_books ON author_agg(book_count DESC, author_id);
-- 口径版本 +1（v5 → v6）：store_metrics 多了 chapter_count 键，三张预计算表是本次新增。
--
-- 与 0007 / 0017 / 0018 / 0020 同一条路径：版本号落后即判定过期，启动后由
-- ensureMetrics 在后台按新口径重算一次。区别在于本次的**预计算表已经在本步骤里
-- 回填完**（migrate_0022.go），所以最近一次重算只补 text_bytes / chapter_word_sum
-- 这类需要扫正文块的项，读接口在它算完前返回 stale 而不是 500。
INSERT INTO store_metrics(key, value, updated_at)
VALUES ('metrics_version', 6, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ON CONFLICT(key) DO UPDATE SET value = 6, updated_at = excluded.updated_at;

-- 清掉旧口径的聚合值，由后台按新口径重算（与 0007 / 0017 / 0018 / 0020 同一路径）。
DELETE FROM store_metrics WHERE key IN ('text_bytes', 'text_bytes_raw', 'book_text_bytes', 'chapter_word_sum', 'index_bytes');
