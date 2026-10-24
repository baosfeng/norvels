-- norvels 数据库结构
-- 幂等：全部 IF NOT EXISTS；Migrate 每次启动执行整份文件。
-- 时间字段统一存 UTC ISO-8601 文本（2006-01-02T15:04:05.000Z）。
-- 全文检索采用「字级分词」：chapters.title_seg / content_seg 是原文经 store.Segment()
-- 逐字加空格后的镜像列，chapters_fts 用 unicode61 分词，支持 2 字及以上中文检索。

PRAGMA user_version = 1;

-- ---------- 作者 ----------
CREATE TABLE IF NOT EXISTS authors (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE,
    book_count INTEGER NOT NULL DEFAULT 0,
    word_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- ---------- 书籍 ----------
CREATE TABLE IF NOT EXISTS books (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    title         TEXT    NOT NULL,
    author_id     INTEGER NOT NULL DEFAULT 0,
    source_path   TEXT    NOT NULL UNIQUE,
    source_name   TEXT    NOT NULL DEFAULT '',
    file_size     INTEGER NOT NULL DEFAULT 0,
    file_sha256   TEXT    NOT NULL UNIQUE,
    encoding      TEXT    NOT NULL DEFAULT '',
    word_count    INTEGER NOT NULL DEFAULT 0,
    chapter_count INTEGER NOT NULL DEFAULT 0,
    volume_count  INTEGER NOT NULL DEFAULT 0,
    intro         TEXT    NOT NULL DEFAULT '',
    status        TEXT    NOT NULL DEFAULT 'parsed',
    multi_book    INTEGER NOT NULL DEFAULT 0,
    imported_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_books_author   ON books(author_id);
CREATE INDEX IF NOT EXISTS idx_books_updated  ON books(updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_books_imported ON books(imported_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_books_title    ON books(title);
CREATE INDEX IF NOT EXISTS idx_books_chapters ON books(chapter_count DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_books_words    ON books(word_count DESC, id DESC);

-- ---------- 卷/部 ----------
CREATE TABLE IF NOT EXISTS volumes (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    idx     INTEGER NOT NULL,
    title   TEXT    NOT NULL DEFAULT '',
    UNIQUE(book_id, idx)
);

CREATE INDEX IF NOT EXISTS idx_volumes_book ON volumes(book_id, idx);

-- ---------- 章节 ----------
-- volume_id = 0 表示未分卷（api 层会包装成伪卷 id=0/idx=-1）。
-- title_seg / content_seg 是 FTS 镜像列，由 store.Segment() 生成。
CREATE TABLE IF NOT EXISTS chapters (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id     INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    volume_id   INTEGER NOT NULL DEFAULT 0,
    idx         INTEGER NOT NULL,
    title       TEXT    NOT NULL DEFAULT '',
    title_seg   TEXT    NOT NULL DEFAULT '',
    content     TEXT    NOT NULL DEFAULT '',
    content_seg TEXT    NOT NULL DEFAULT '',
    word_count  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_chapters_book_idx ON chapters(book_id, idx, id);
CREATE INDEX IF NOT EXISTS idx_chapters_volume   ON chapters(volume_id);

-- ---------- 标签 ----------
CREATE TABLE IF NOT EXISTS tags (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT    NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS book_tags (
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_book_tags_tag ON book_tags(tag_id, book_id);

-- ---------- 导入任务 ----------
CREATE TABLE IF NOT EXISTS import_jobs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source_dir  TEXT    NOT NULL,
    status      TEXT    NOT NULL DEFAULT 'running',
    total       INTEGER NOT NULL DEFAULT 0,
    imported    INTEGER NOT NULL DEFAULT 0,
    skipped     INTEGER NOT NULL DEFAULT 0,
    failed      INTEGER NOT NULL DEFAULT 0,
    started_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    finished_at TEXT,
    message     TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_import_jobs_started ON import_jobs(started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_import_jobs_status  ON import_jobs(status);

CREATE TABLE IF NOT EXISTS import_errors (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id     INTEGER NOT NULL REFERENCES import_jobs(id) ON DELETE CASCADE,
    file_path  TEXT    NOT NULL,
    reason     TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_import_errors_job ON import_errors(job_id, id);

-- ---------- 全文检索（external content + 触发器同步） ----------
-- 不用 contentless：external content 让 FTS 不重复存原文，delete/update 用 old.* 值同步。
CREATE VIRTUAL TABLE IF NOT EXISTS chapters_fts USING fts5(
    title_seg,
    content_seg,
    content='chapters',
    content_rowid='id',
    tokenize='unicode61'
);

CREATE TRIGGER IF NOT EXISTS chapters_ai AFTER INSERT ON chapters BEGIN
    INSERT INTO chapters_fts(rowid, title_seg, content_seg)
    VALUES (new.id, new.title_seg, new.content_seg);
END;

CREATE TRIGGER IF NOT EXISTS chapters_ad AFTER DELETE ON chapters BEGIN
    INSERT INTO chapters_fts(chapters_fts, rowid, title_seg, content_seg)
    VALUES ('delete', old.id, old.title_seg, old.content_seg);
END;

CREATE TRIGGER IF NOT EXISTS chapters_au AFTER UPDATE ON chapters BEGIN
    INSERT INTO chapters_fts(chapters_fts, rowid, title_seg, content_seg)
    VALUES ('delete', old.id, old.title_seg, old.content_seg);
    INSERT INTO chapters_fts(rowid, title_seg, content_seg)
    VALUES (new.id, new.title_seg, new.content_seg);
END;
