-- 0003 阅读进度（服务端权威源，替代前端 localStorage）
-- 单用户：不设 user_id，一本书一条记录（book_id 主键）。
-- percent 为 0~1 的书内进度比例；chapter_idx 冗余存章节序号供排序/展示。
-- 外键 ON DELETE CASCADE：删书自动清进度；依赖 open.go 的 foreign_keys(ON)。
CREATE TABLE IF NOT EXISTS reading_progress (
    book_id     INTEGER PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
    chapter_id  INTEGER NOT NULL,
    chapter_idx INTEGER NOT NULL,
    percent     REAL    NOT NULL DEFAULT 0,
    updated_at  TEXT    NOT NULL
);

-- 驾驶舱「最近在读」按 updated_at 倒序取前 5 条。
CREATE INDEX IF NOT EXISTS idx_reading_progress_updated
    ON reading_progress(updated_at DESC, book_id DESC);
