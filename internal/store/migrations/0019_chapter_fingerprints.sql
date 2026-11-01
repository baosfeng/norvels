-- 0019 章级内容指纹（设计 §4）。
--
-- 表在**正文库** norvels-content.db 里：建表与索引由 Go 步骤 createFingerprintTableStep
-- 完成（跨库 DDL 进不了主库事务，与 0018 同一个理由；结构口径只在
-- internal/store/content.go 的 contentSchemaDDL 一处）。
--
-- 本文件的 SQL 只做主库侧的一件事：预置去重阈值设置项，让「阈值可配」在库里可见、
-- 可被运维直接改（缺省 0.95；命中率 ≥ 阈值即视为同一本书，见 docs/architecture.md §2.4）。
--
-- 幂等：ON CONFLICT DO NOTHING —— 用户改过阈值时重跑不会把值打回 0.95。
INSERT INTO settings(key, value, updated_at)
VALUES ('dedupe.threshold', '0.95', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ON CONFLICT(key) DO NOTHING;
