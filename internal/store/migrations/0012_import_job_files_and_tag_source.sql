-- 0012 导入文件级终态 + 标签来源（P1-B 导入侧 spec §2.1）。
--
-- import_job_files 记录**每个候选条目的终态**，是「单文件重试」的数据基础：
--   source      可重放源（目录文件 = 绝对路径；zip 条目 = 包绝对路径:包内路径；
--               合集拆分段沿用其伪路径）。重试只传这里的 source，不传展示路径。
--   display_path 只给前端展示（base 名 / 包名:包内路径）。
--   status      imported|failed|skipped|unsupported|canceled（终态，唯一键 job_id+source）。
--   reason      失败/跳过原因（人话）。
--   book_id     成功导入时的书籍 id（0 表示无）。
-- 唯一索引让「同一任务同一源重放」原地更新状态，不会堆重复行。
--
-- book_tags.source 标记标签来源：'' = 手动 / 历史，auto = 词表命中，epub = dc:subject。
-- 加列走 Go 步骤 addBookTagsSourceStep（SQLite 没有 ADD COLUMN IF NOT EXISTS，
-- 版本回退重跑会撞 duplicate column），与 0008/0010/0011 同一条幂等路径。
--
-- 只加列/加表，不删数据、不搬迁，因此不需要 VACUUM。

CREATE TABLE IF NOT EXISTS import_job_files (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id       INTEGER NOT NULL,
    source       TEXT    NOT NULL,
    display_path TEXT    NOT NULL,
    status       TEXT    NOT NULL,
    reason       TEXT    NOT NULL DEFAULT '',
    book_id      INTEGER NOT NULL DEFAULT 0,
    updated_at   TEXT    NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_import_job_files
    ON import_job_files(job_id, source);
