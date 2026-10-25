-- 0005 修复 store_metrics 表结构（0004 早期版本遗留）。
--
-- 背景：0004 早期版本的建表语句用了 CREATE TABLE IF NOT EXISTS，而开发期曾存在过
-- 一版旧结构 (id, text_bytes, index_bytes, chapters_at, computed_at)。遇到同名旧表时
-- IF NOT EXISTS 直接跳过，新结构从未建上，导致每次写 metrics 都报
--   SQL logic error: table store_metrics has no column named key
--
-- 对「已经应用了 0004 旧版」的库，改动 0004 文件不会重跑（版本号已记录），
-- 因此需要这条独立迁移来重建。
--
-- store_metrics 只存可从业务表重算的派生值（正文字节、字数总和、索引字节），
-- DROP 无数据损失：表清空后，服务启动时会后台异步重算（13.8 GiB 库约 75 s），
-- 算完前 /api/stats 返回 stale / compositionEstimated = true。
DROP TABLE IF EXISTS store_metrics;
CREATE TABLE store_metrics (
    key        TEXT PRIMARY KEY,
    value      INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO store_metrics(key, value, updated_at)
VALUES ('metrics_version', 1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ON CONFLICT(key) DO NOTHING;
