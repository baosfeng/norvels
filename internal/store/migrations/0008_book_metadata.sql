-- 0008 书籍元数据扩展 + 封面存储。
--
-- 动机：导入 EPUB 时能拿到书名/作者之外的「身份」——语言、出版社、书号、
-- 出版日期、系列、分类（dc:subject），以及封面。txt 路径这些列留空，
-- 前端按零值降级，不改变现有行为。
--
-- 封面存 BLOB 而不是文件：本项目的数据契约是「一个 SQLite 文件即全部」，
-- 备份/迁移只复制一个文件。封面是全库唯一的二进制资源，插图/字体/CSS 不入库。
--
-- ---- DDL 分工 ----
-- books 的 8 个新列无法用条件 DDL 表达（SQLite 没有 ADD COLUMN IF NOT EXISTS，
-- 而版本回退重跑、开发期手工加过列的库都必须能安全通过），由 Go 步骤
-- migrationStepsByVersion[8] → addBookMetadataColumnsStep 在**同一事务**内幂等执行。
-- 本文件只负责建表。
--
-- 本迁移只加列/加表，不删数据、不搬迁，因此不需要 VACUUM。
CREATE TABLE IF NOT EXISTS book_covers (
    book_id    INTEGER PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
    mime       TEXT    NOT NULL,
    bytes      BLOB    NOT NULL,
    sha256     TEXT    NOT NULL DEFAULT '',
    updated_at TEXT    NOT NULL
);
