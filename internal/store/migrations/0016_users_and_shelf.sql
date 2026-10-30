-- 0016 多账号 + 我的书架。
--
-- 口径（leader 已冻结）：
--   ① 书库共享：books/authors/chapters/tags 不设 user 维度，所有人看同一个书库；
--   ② 个人记录按账号隔离：阅读进度 / 阅读会话 / 书签 / 书单 / 我的书架；
--   ③ 只注册 + 输账号登录，无密码 —— 因此 users 表不设任何口令列。
--
-- 单用户时代的存量数据全部归到默认账号 id=1（reader），升级后使用方式不变：
-- 未配置 NORVELS_AUTH_TOKEN 时所有请求都按默认账号读写，与今天的单用户行为等价。
--
-- ---- 为什么 user_shelf 是独立表 ----
-- 「我的书架」是用户主动加入书架的书（新概念），与 collections（自定义书单，可多个、
-- 可命名）不是一回事：一本书可以属于多个书单，只有一个书架归属。全部书仍可浏览。
--
-- ---- DDL 分工 ----
-- 建表是条件 DDL（IF NOT EXISTS），放本文件；
-- reading_progress / collections / collection_books 要改主键与唯一约束，SQLite 不支持
-- ALTER 主键，只能重建表并搬数据；重建必须先探测「是不是已经是新结构」（删版本号
-- 重跑是排查迁移问题的常规手段），因此整段放在 Go 步骤
-- migrationStepsByVersion[16] → addUserScopedTablesStep 的同一事务内。
-- reading_sessions / bookmarks 只加列 + 换索引，同样走该步骤（SQLite 没有
-- ADD COLUMN IF NOT EXISTS，与 0008/0010/0011/0014 同一条幂等路径）。
--
-- ---- 搬移了数据，但不自动 VACUUM ----
-- reading_progress / collections / collection_books 是「建新表 → 搬行 → 删旧表」，
-- 被删掉的旧表数据页会进入 freelist。这里**不**在启动路径上自动跑 VACUUM：
-- 搬的只是个人记录那几张表（单用户时代通常几十到几千行），而 VACUUM 会重写整个
-- 库文件、需要额外一份等效磁盘空间（1.6 GB 的库实测 25 秒 + 1.6 GB 临时空间），
-- 把启动失败的风险压在了最不该出问题的地方。
-- 迁移结束时会打印一行提示，由运维在停服窗口按 docs/deploy.md §7.3 手动执行。

-- ---------- 账号 ----------
-- name 唯一性按「TRIM + 大小写折叠」比较（表达式唯一索引），与 store 层建号/登录的
-- 判重口径完全一致：
--   - TRIM：否则「 reader」与「reader」会成为两个只差空格的账号；
--   - COLLATE NOCASE：否则「Alice」与「alice」也是两个账号（只折叠 ASCII，中文无大小写）。
-- 存储的 name 仍是用户输入的原样，折叠只发生在比较上。
-- last_seen_at 为 NULL 表示「建号后从未登录过」。
CREATE TABLE IF NOT EXISTS users (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL,
    created_at   TEXT    NOT NULL,
    last_seen_at TEXT
);

-- 先 DROP 再建：真正带 COLLATE NOCASE 的索引若被 IF NOT EXISTS 挡住，大小写折叠会静默失效。
DROP INDEX IF EXISTS ux_users_name;
CREATE UNIQUE INDEX IF NOT EXISTS ux_users_name ON users(TRIM(name) COLLATE NOCASE);

-- ---------- 会话 ----------
-- 只存 token 的 SHA-256（token_hash），库被读到也无法拿去冒充登录：
-- 明文 token 只存在于用户的 HttpOnly Cookie 里。
-- expires_at 是 UTC 文本，过期行由 store 侧在登录/解析时顺手清理，不靠定时任务。
CREATE TABLE IF NOT EXISTS user_sessions (
    token_hash TEXT    PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    created_at TEXT    NOT NULL,
    expires_at TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS ix_user_sessions_user    ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS ix_user_sessions_expires ON user_sessions(expires_at);

-- ---------- 我的书架 ----------
-- 联合主键让「重复加入」天然幂等（INSERT OR IGNORE），不需要额外唯一索引。
-- 不声明 books 外键：删书由 internal/store/delete.go 的级联清单显式清理
-- （与 reading_sessions / collection_books 同一条口径，见该文件注释）。
CREATE TABLE IF NOT EXISTS user_shelf (
    user_id  INTEGER NOT NULL,
    book_id  INTEGER NOT NULL,
    added_at TEXT    NOT NULL,
    PRIMARY KEY (user_id, book_id)
);

-- 书架列表按加入时间倒序分页。
CREATE INDEX IF NOT EXISTS ix_user_shelf_added ON user_shelf(user_id, added_at DESC, book_id DESC);

PRAGMA user_version = 16;
