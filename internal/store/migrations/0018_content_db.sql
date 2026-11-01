-- 0018 正文块表搬进独立文件 norvels-content.db。
--
-- 为什么拆：正文占库体积 95%（ADR-0007 口径：27.5 GB / 28.9 GB）。拆开之后
--   · 元数据侧的备份/VACUUM/损坏修复不再被几十 GB 正文牵连；
--   · 正文库可以单独换盘、单独从备份恢复；
--   · chapters.block_id 退化成**跨库弱引用**（没有外键，见下方 Go 步骤说明）。
--
-- ---- DDL 分工 ----
-- 建表 / 搬块 / DROP TABLE 都发生在**另一个数据库文件**上，没法放进本迁移的主库
-- 事务里（SQLite 的单库提交语义）。因此：
--   · 正文库的建表与逐书搬运在 Go 步骤里：migrationStepsByVersion[18] →
--     splitContentBlocksStep（闭包绑定 Store，才能访问正文库连接）；
--   · 本文件的 SQL 只做能落在主库上的部分：把聚合口径标记为过期。
--
-- 幂等：步骤以「主库还有没有 chapter_blocks 表」判定是否已完成（DROP 之后重跑
-- 直接返回）；正文库一侧用 CREATE TABLE IF NOT EXISTS + INSERT OR REPLACE，
-- 中断在半路可以原样重跑。
--
-- 顺序：即便正文库先搬完而主库事务回滚，主库的 chapter_blocks 与版本号都还在，
-- 重跑只是把同样的块再写一遍（id 相同）。反过来不成立——所以 DROP 是主库事务里的
-- 最后一步，与版本号一起原子提交。

-- 口径版本 +1：text_bytes 的含义不变（仍是块 packed 字节），但「索引类」改成
-- 只按主库文件算，缓存里的旧值必然过期；不 +1 的话 /api/stats 会拿旧口径的
-- 数字去和新的两文件合计对比。
INSERT INTO store_metrics(key, value, updated_at)
VALUES ('metrics_version', 4, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ON CONFLICT(key) DO UPDATE SET value = 4, updated_at = excluded.updated_at;

-- 清掉旧口径的聚合值，由后台按新口径重算（与 0007 / 0017 同一条路径）。
DELETE FROM store_metrics WHERE key IN ('text_bytes', 'book_text_bytes', 'chapter_word_sum', 'index_bytes');
