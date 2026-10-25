-- 0011 阅读状态 + 书签（P1 阅读体验 spec §2.1）。
--
-- read_status 与 books.status（解析状态）、books.serial_status（连载状态）是三套语义：
--   read_status 是读者阅读状态：''=未标记 / want / reading / done，
--   只由用户 PATCH 或「首次写进度」写入；存量行保持 ''，不回填、不猜测。
--
-- bookmarks 不声明外键：删除书籍/章节时由 DELETE /api/books/{id} 的级联语句显式清理
-- （与其它 7 张子表一致，见 internal/store/delete.go 的 bookCascadeTables）。
-- 同一 (book_id, chapter_id, para_index) 唯一：重复添加走幂等 upsert，不产生重复条目。
--
-- ---- DDL 分工 ----
-- bookmarks 是建表，条件 DDL 够用，放本文件；
-- books.read_status 是加列，SQLite 没有 ADD COLUMN IF NOT EXISTS，版本回退重跑会撞
-- "duplicate column name"，因此由 Go 步骤 migrationStepsByVersion[11] →
-- addBookReadStatusStep 在同一事务内幂等执行（与 0008/0010 同一条路径）。
--
-- 只加列/加表，不删数据、不搬迁，因此不需要 VACUUM。

CREATE TABLE IF NOT EXISTS bookmarks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id     INTEGER NOT NULL,
    chapter_id  INTEGER NOT NULL,
    chapter_idx INTEGER NOT NULL,
    para_index  INTEGER NOT NULL,
    label       TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_bookmarks_pos
    ON bookmarks(book_id, chapter_id, para_index);
CREATE INDEX IF NOT EXISTS ix_bookmarks_book
    ON bookmarks(book_id, created_at DESC);
