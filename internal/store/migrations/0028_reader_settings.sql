-- 0028 per-user reader settings（JSON 落盘 + 账号同步）。
--
-- 阅读器主题/字号/分页/自动滚动等 ReaderSettings 从 localStorage 单机方案
-- 扩展为「本地落盘 + 服务端按账号同步」：每账号一行，settings 列存 JSON 全文。
-- 匿名/默认账号（id=1）同样有行（首次 PUT 时 upsert）。

CREATE TABLE IF NOT EXISTS user_reader_settings (
    user_id    INTEGER PRIMARY KEY,
    settings   TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);

PRAGMA user_version = 28;
