-- 0017 正文块重组：单块目标 8 MB + 写入格式 0x03（码本 + zstd）。
--
-- 为什么存量还要再搬一次：0007 时期的块目标是 1 MB、写入格式是 0x02（纯 zstd）。
-- 码本上线后同一批语料（200 本抽样、同一口径）实测：
--   1 MB 块 纯 zstd         33.42%
--   1 MB 块 码本 + zstd     28.08%
--   8 MB 块 码本 + zstd     26.45%   ← 本次迁到的目标形态
-- 只换写入代码不会让**已有**块变小：老块仍是 1 MB / 0x02，必须逐块解开、
-- 按 8 MB 章节边界重切、用 0x03 重压。生产库口径：87,626 个块、块字节 27.5 GB，
-- 迁移后按抽样换算约 21 GB。
--
-- ---- DDL 分工 ----
-- 本文件只做能条件化的部分：建重建目标表 + 口径版本 +1。
-- 「逐块解压 → 重切 → 重压 → 换表」没法用 SQL 表达，放在**同一事务**的 Go 步骤里：
-- migrationStepsByVersion[17] → rebuildBlocksForCodebook。
--
-- ---- 为什么先写新表再换名，而不是原地改行 ----
-- 这一步同时改了块的字节数、块内偏移、块与章节的对应关系。原地改要动两处
-- （块行 + 章节行），任何一步失败都会留下「章节指向已被重压的块」的半迁移库——
-- 正文还在，偏移全错，读出来是乱码级故障。写新表 + 最后两条 DDL 换名，
-- 让切换成为事务里的原子动作。
--
-- ---- 磁盘峰值 ----
-- 迁移期间新旧两份块字节同时存在（生产约 27.5 GB + 21 GB），迁移后还要 VACUUM
-- 才会真正缩小文件。运维必须预留约 2 倍块存储，见 docs/deploy.md。
--
-- 目标表不建索引：重建期间每本书要插几百到几千行，索引只会拖慢插入；
-- 换名之后再按 0007 的同名索引重建（旧表的索引随 DROP TABLE 一起消失）。
CREATE TABLE IF NOT EXISTS chapter_blocks_new (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id    INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,
    first_idx  INTEGER NOT NULL,
    last_idx   INTEGER NOT NULL,
    raw_bytes  INTEGER NOT NULL,
    packed     BLOB    NOT NULL,
    UNIQUE(book_id, seq)
);

-- 口径版本 +1：text_bytes 的含义仍是「块 packed 字节之和」，但块边界与压缩格式都变了，
-- 缓存里的数值必然过期。不 +1 的话，「迁移后体积怎么没降」会被当成迁移没生效。
INSERT INTO store_metrics(key, value, updated_at)
VALUES ('metrics_version', 3, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ON CONFLICT(key) DO UPDATE SET value = 3, updated_at = excluded.updated_at;

-- 清掉旧口径的聚合值，由后台按新块重算（与 0007 同一条路径）。
DELETE FROM store_metrics WHERE key IN ('text_bytes', 'book_text_bytes', 'chapter_word_sum', 'index_bytes');
