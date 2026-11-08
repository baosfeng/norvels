-- 0027 元数据补全的变更日志（enrich_changes）。
--
-- 为什么必须落库而不是只打日志：18 万本量级的批量改写一旦错了，唯一的退路是「按批次
-- 把旧值写回」。日志文件在容器里随重启消失，而库不会。
--
-- 口径：
--   · 一次 --apply 是一个 batch（CLI 生成 20261108-015300-abcdef 形态的 id）；
--   · 一行一个字段的改动，old_value / new_value 存文本（分类存 "L1/L2"）；
--   · source_id 记「谁给的结论」：书源 id，或 local:keyword（本地词表）；
--   · 本表只追加，不更新：回滚也只是一次新的写入（由调用方记 transferred 字段值）。
--
-- 幂等：CREATE TABLE / CREATE INDEX 全是 IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS enrich_changes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_id   TEXT    NOT NULL,
    book_id    INTEGER NOT NULL,
    field      TEXT    NOT NULL CHECK (field IN ('author', 'intro', 'category', 'chapter_split')),
    old_value  TEXT    NOT NULL DEFAULT '',
    new_value  TEXT    NOT NULL DEFAULT '',
    source_id  TEXT    NOT NULL DEFAULT '',
    confidence REAL    NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_enrich_changes_batch ON enrich_changes(batch_id);
CREATE INDEX IF NOT EXISTS idx_enrich_changes_book  ON enrich_changes(book_id, created_at);
