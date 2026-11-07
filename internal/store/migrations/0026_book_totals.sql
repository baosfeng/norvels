-- 0026 库级总量预计算（/api/stats 的 books/words 与 health 的质量分覆盖率）。
--
-- 背景（生产 182,000 本 / 54,579,000 章实测，GET /api/metrics 的 sql 数组）：
--
--   GET /api/health/report  p95 5500 ms（SQL 占 100%），最慢一条
--     SELECT COUNT(*), COALESCE(SUM(CASE WHEN quality_chapters > 0 AND quality_at <> '' …), 0) FROM books
--     单次 p95 5500 ms / max 6760 ms —— 每次请求把 18 万本整行（含简介，GB 级页）扫一遍；
--   GET /api/stats           p95 261 ms，其中
--     SELECT (SELECT COUNT(*) … superseded_by = 0), (SELECT COALESCE(SUM(word_count), 0) FROM books) …
--     同样是一次 books 全表扫（SUM 要读完整行记录，不是 CPU 问题，是 IO）。
--
-- 0025 已经把「缺简介 / 缺作者 / 超薄书 / 章节数分桶」挪进 book_health_counts；
-- 本迁移接着把剩下的 4 个值挪出请求路径：书数（全量 / 当前版本）、全库书目字数、
-- 已算质量分的书数。它们都是「books 一遍扫描」的产物，与 0025 同一套维护方式。
--
-- 本迁移只建结构，**不做回填**（生产升级期间不能停监听）：
--   · book_totals       4 个标量，一行一个 kind；
--   · book_totals_state 回填状态（complete=0 时读侧回退到实时 SQL，绝不返回错数字）。
-- 维护走 SQL 触发器（migrate_0026.go 生成）：books 的 INSERT / DELETE / 改四列
-- （superseded_by、word_count、quality_chapters、quality_at）自动维持，
-- 所有写路径（导入、追更、替换旧版、合并/拆书、删书、回填质量分）都覆盖，
-- 不需要在 Go 的每条写路径上补一行，也就不会出现「新写路径忘了维护」的静默漂移。
--
-- 回填由启动后的后台任务做（books 一遍扫描，18 万行）：完成后读取侧切到预计算值。
-- 幂等：全部 IF NOT EXISTS / INSERT OR IGNORE。

CREATE TABLE IF NOT EXISTS book_totals (
    kind TEXT PRIMARY KEY CHECK (kind IN
        ('books_all', 'books_current', 'word_sum', 'quality_computed')),
    n    INTEGER NOT NULL DEFAULT 0
) WITHOUT ROWID;

INSERT OR IGNORE INTO book_totals(kind, n) VALUES
    ('books_all', 0), ('books_current', 0), ('word_sum', 0), ('quality_computed', 0);

CREATE TABLE IF NOT EXISTS book_totals_state (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    complete   INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL DEFAULT ''
);
