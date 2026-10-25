-- 0007 正文改为「按块压缩」存储。
--
-- 背景：用户只需要读章节内容，不需要按正文检索。0006 已把为检索服务的
-- 分词镜像列与 FTS 索引整块移除；本条迁移解决剩下的大头——正文本身。
-- 全量库实测：正文 content 4.87 GB，占 14.84 GB 的 33%。
--
-- 存储形态：从「每章一行存原文」改成「若干章打成一包、zstd 压缩后存进 chapter_blocks」。
-- 章节行只保留「我属于哪一块、块内从第几字节起、多长」三个整数。
--
-- 为什么按块而不是按章：章节平均 13 KB，对压缩器样本太小。真实语料实测
--   按章独立 flate-6   49.0%
--   按章独立 zstd-best 48%
--   1 MB 块 zstd-best 34.1%   ← 体积几乎减半
-- 因为压缩窗口跨章节才能吃到「同一本书反复出现的词、人名、地名、句式」。
--
-- 为什么不是整本：整本 31.9%，只差 2.2 个百分点，但读一章要解压整本，
-- 翻页从毫秒级掉到十几毫秒，内存峰值随书本大小线性涨。1 MB 是拐点。
--
-- 块的边界按**章节边界**切（不按字节切），因此一块永远覆盖整数个章节，
-- 块内偏移直接指向完整正文，不会把某一章切碎。
--
-- ---- DDL 分工 ----
-- 本文件负责能条件化的部分（建表、建索引）；「重建 chapters 表以替换 content 列」
-- 无法条件化（SQLite 没有条件 DDL，老库可能有/没有分词列），由 Go 在**同一事务**内
-- 幂等执行：见 migrate.go 的 migrationStepsByVersion[7] → rebuildChaptersForBlocks。
--
-- 迁移顺序很重要：先把新表建好，Go 步骤再来搬数据。搬完若任何一步失败，
-- 整个事务回滚，库停在 0006 状态（正文仍是原文、一个字节都没少）。
CREATE TABLE IF NOT EXISTS chapter_blocks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id    INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,
    first_idx  INTEGER NOT NULL,
    last_idx   INTEGER NOT NULL,
    raw_bytes  INTEGER NOT NULL,
    packed     BLOB    NOT NULL,
    UNIQUE(book_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_chapter_blocks_range ON chapter_blocks(book_id, first_idx, last_idx);
CREATE INDEX IF NOT EXISTS idx_chapter_blocks_book  ON chapter_blocks(book_id, seq);

-- 口径版本 +1：0007 之后 text_bytes 的含义变了（packed 字节之和，不再是原文逻辑字节），
-- 老缓存必须作废，否则 /api/stats 会拿着旧口径的数去和新的库体积对比。
INSERT INTO store_metrics(key, value, updated_at)
VALUES ('metrics_version', 2, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ON CONFLICT(key) DO UPDATE SET value = 2, updated_at = excluded.updated_at;

-- 清掉旧口径的聚合值：text_bytes 的含义已经变了，留着会被误读成「压缩后体积」。
DELETE FROM store_metrics WHERE key IN ('text_bytes', 'book_text_bytes', 'chapter_word_sum', 'index_bytes');
