-- 0025 书级健康计数预计算（health/report 请求路径上最后三条全表/全索引扫描）。
--
-- 背景（生产 169,230 本实测，/api/metrics 的 sql 数组）：
--   SELECT COUNT(*) FROM books WHERE TRIM(intro) = ''                                 4847 ms
--   SELECT COUNT(*) FROM books WHERE author_id = 0 OR author_id NOT IN (SELECT …)        851 ms
--   SELECT COUNT(*) … books.chapter_count 六段区间                                       324 ms
-- 前两条要走全表扫（books 含简介，GB 级页读），第三条是六次全索引扫——都不是 CPU 问题，
-- 而是「每请求都要把同一批页从 NAS 读一遍」。
--
-- 本迁移只建结构，**不做回填**（生产升级期间不能停监听）：
--   · book_health_counts   三个标量（缺简介 / 缺作者 / 超薄书）；
--   · book_chapter_buckets 章节数六段区间；
--   · book_health_state    回填状态（complete=0 时读侧回退到实时 SQL，绝不返回错数字）。
-- 维护走 SQL 触发器（migrate_0025.go 生成），因此所有写路径——导入、追更、合并/拆分、
-- 删书、改作者、建作者、删作者——都自动维持，不需要在 Go 的每条写路径上补一行，
-- 也就不会出现「某个新写路径忘了维护」的静默漂移。
--
-- 回填由启动后的后台任务做（books 一遍扫描，169k 行）：完成后读取侧切到预计算值。
-- 幂等：全部 IF NOT EXISTS / INSERT OR IGNORE / ON CONFLICT DO NOTHING。

CREATE TABLE IF NOT EXISTS book_health_counts (
    kind TEXT PRIMARY KEY CHECK (kind IN ('missing_intro', 'missing_author', 'thin_books')),
    n    INTEGER NOT NULL DEFAULT 0
) WITHOUT ROWID;

INSERT OR IGNORE INTO book_health_counts(kind, n) VALUES
    ('missing_intro', 0), ('missing_author', 0), ('thin_books', 0);

CREATE TABLE IF NOT EXISTS book_chapter_buckets (
    bucket INTEGER PRIMARY KEY,
    n      INTEGER NOT NULL DEFAULT 0
) WITHOUT ROWID;

INSERT OR IGNORE INTO book_chapter_buckets(bucket, n) VALUES
    (0, 0), (1, 0), (2, 0), (3, 0), (4, 0), (5, 0);

CREATE TABLE IF NOT EXISTS book_health_state (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    complete   INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL DEFAULT ''
);
