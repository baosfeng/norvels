-- 0014 P3 阅读体验 / 发现 / 管理补全（只加列/加表，不搬数据）。
--
-- 四列：
--   books.rating               评分 0–5（0 = 未评，不参与 rating 筛选与 sort=rating）；
--   books.new_chapters         追更累计的新章数，只由追更链路写、ack 接口清零；
--   books.last_chapter_at      最新章节时间，系列列表按它排「最近更新」；
--   reading_progress.para_index 章内段落位置（0 = 章首），离开章节时上报。
--
-- 三张表：阅读会话（时长统计/热力图的数据源）、书单与书单-书关联。
--
-- ---- DDL 分工 ----
-- 建表与索引是条件 DDL，放本文件；四列是加列，SQLite 没有 ADD COLUMN IF NOT EXISTS，
-- 版本回退重跑会撞 "duplicate column name"，因此由 Go 步骤
-- migrationStepsByVersion[14] → addP3ColumnsStep 在同一事务内幂等执行
-- （与 0008/0010/0011 同一条路径）。
--
-- reading_sessions 与 collection_books 不声明 books 外键：删书由
-- internal/store/delete.go 的级联清单显式清理（P3 契约 §迁移 0014）。
-- collection_books 对 collections 声明 ON DELETE CASCADE：删书单自动摘书。
--
-- 存量行保持零值（rating=0、new_chapters=0、last_chapter_at=''、para_index=0），
-- 不回填、不猜。只加列/加表，不删数据、不搬迁，因此不需要 VACUUM。

CREATE TABLE IF NOT EXISTS reading_sessions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id    INTEGER NOT NULL,
    chapter_id INTEGER NOT NULL,
    started_at TEXT    NOT NULL,
    seconds    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS ix_sessions_started ON reading_sessions(started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS ix_sessions_book    ON reading_sessions(book_id, started_at DESC);

CREATE TABLE IF NOT EXISTS collections (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE,
    created_at TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS collection_books (
    collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    book_id       INTEGER NOT NULL,
    added_at      TEXT    NOT NULL,
    PRIMARY KEY (collection_id, book_id)
);
