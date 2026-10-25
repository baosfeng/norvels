-- 0004 驾驶舱性能修复：全库聚合值预计算表。
--
-- 背景（真实全量库 838 本 / 422,715 章 / 13.8 GiB 实测）：
-- 库体积构成原先在 /api/stats 请求路径上对 chapters 做
--   SUM(length(content) + length(content_seg))
-- 要扫 4.2 GB 正文 + 4.2 GB 镜像列，实测 99 s；对 chapters_fts_data 做
-- SUM(length(block)) 再花 20.9 s。合计让 /api/stats 达到 19~90 s。
--
-- 对照实测：SUM(word_count)（整数列）42 万行只要 0.05 s，
-- 而 SUM(length(content))（文本列）要 54 s——文本列必须读完整行记录，慢约 1000 倍。
-- dbstat 虚表全库扫描 74.9 s，同样不可用。
--
-- 因此把「必须扫文本列」的聚合值在导入时顺手算好存这里，读接口只查表（毫秒级）。
-- 其余整型聚合（书名数、章数、字数）实时 SQL 就是毫秒级，不重复缓存。
--
-- ⚠️ 这里必须先 DROP 再 CREATE，不能只写 CREATE TABLE IF NOT EXISTS：
-- 开发期存在过一版旧结构（id, text_bytes, index_bytes, chapters_at, computed_at），
-- 而 IF NOT EXISTS 遇到同名旧表会直接跳过，导致新结构永远建不上，
-- 后续所有写 metrics 的语句都会以 "no column named key" 失败。
-- store_metrics 存的全是可从业务表重算的派生值，删掉无数据损失，下次启动后台重算即可。
DROP TABLE IF EXISTS store_metrics;
CREATE TABLE store_metrics (
    key        TEXT PRIMARY KEY,
    value      INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);

-- 明确写入口径版本：口径变了就 +1，启动时发现表内版本落后即触发重算。
INSERT INTO store_metrics(key, value, updated_at)
VALUES ('metrics_version', 1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ON CONFLICT(key) DO NOTHING;

-- 章节字数覆盖索引不写在这里：它依赖 chapters 表已存在，而老库升级路径上可能只有
-- 部分业务表（SQLite 没有条件 DDL，迁移文件会直接失败），故由应用层在迁移完成后
-- 幂等创建（见 store.ensureCompositionIndex）。
-- 该索引让「章节长度分布」从 73.6 s 降到 0.07 s：没有它 GROUP BY 要读完整行记录
-- （含数 GB 正文），有它则走覆盖索引只读该列。索引体积约 4.6 MB。
