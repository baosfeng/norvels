-- 0010 书籍连载状态 + 全局设置。
--
-- 动机（P0 书籍管理 spec §2.1）：详情页要能标「连载中 / 已完结 / 未知」并显示
-- 「最后更新 · 最新章节」；自定义章节识别规则需要一个全局键值表来存。
--
-- serial_status 与 books.status 是两个互不覆写的语义：
--   books.status        解析状态（parsed / …），由导入链路硬编码写入；
--   books.serial_status 作品连载状态（''=未知 / ongoing / completed），只由用户 PATCH 写入。
--
-- 存量书回填为空串（未知），不猜：用户没标过就不该假装知道。
--
-- ---- DDL 分工 ----
-- settings 是建表，条件 DDL 够用，放本文件；
-- books.serial_status 是加列，SQLite 没有 ADD COLUMN IF NOT EXISTS，版本回退重跑
-- 会撞 "duplicate column name"，因此由 Go 步骤
-- migrationStepsByVersion[10] → addBookSerialStatusStep 在同一事务内幂等执行。
--
-- 只加列/加表，不删数据、不搬迁，因此不需要 VACUUM。

-- 全局设置：键值 + 更新时间。自定义章节规则（chapter_rules）存这里的 JSON。
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
