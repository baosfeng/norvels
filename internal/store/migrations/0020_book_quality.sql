-- 0020 书籍质量分（设计 §5 版本择优的落库部分）。
--
-- 动机：判重把「同一本书」识别出来之后，还要能回答「哪一版更好」。质量分是这条判断的
-- 唯一输入，必须随书落库——每次导入都重算旧版本的质量分是不可行的（旧版本正文几十 MB，
-- 而且要解压；判重路径的预算是 200 ms）。
--
-- 七列（全部 NOT NULL DEFAULT 0/''，存量行天然是「未计算」）：
--   garbage_ratio     乱码率 0~1（U+FFFD / 私用区 / 非法字节占比），越低越好
--   noise_ratio       噪声率 0~1（清洗器丢弃的广告/群号/网址行占比），越低越好
--   structure_score   结构分 0~1（章数合理性 × (1-空章率) × (1-离群章率)），越高越好
--   duplicate_ratio   书内重复段落占比 0~1，越低越好
--   quality_chapters  算质量分时的章数（= 0 表示未计算）
--   quality_chars     算质量分时的正文字符数
--   quality_at        算质量分的时间（'' = 未计算）
--
-- 「未计算」的两种载体必须一起看：quality_chapters = 0 **且** quality_at = ''。
-- 零值质量分不是 0 分而是「没算过」——EPUB 与合集拆出的子书没有解析期质量分，
-- 把它们写成 0 分会让版本择优把一本没算过分的书当成「零乱码零噪声的完美版本」。
--
-- ---- DDL 分工 ----
-- 加列无法用条件 DDL 表达（SQLite 没有 ADD COLUMN IF NOT EXISTS，而版本回退重跑、
-- 开发期手工加过列的库都必须能安全通过），由 Go 步骤
-- migrationStepsByVersion[20] → addBookQualityColumnsStep 在**同一事务**内幂等执行。
-- 本文件的 SQL 只做能落在主库上的部分：把聚合口径标记为过期。
--
-- 只加列，不删数据、不搬迁，因此不需要 VACUUM。

-- 口径版本 +1：0020 之后 books 多出七列质量分，健康报告新增「质量分覆盖率」聚合，
-- 缓存里的旧快照无法证明与新口径一致。按「宁可重算一次」处理（全表扫描，GB 级库几分钟），
-- 与 0018 同一条路径。
INSERT INTO store_metrics(key, value, updated_at)
VALUES ('metrics_version', 5, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ON CONFLICT(key) DO UPDATE SET value = 5, updated_at = excluded.updated_at;

-- 清掉旧口径的聚合值，由后台按新口径重算（与 0007 / 0017 / 0018 同一条路径）。
DELETE FROM store_metrics WHERE key IN ('text_bytes', 'book_text_bytes', 'chapter_word_sum', 'index_bytes');
