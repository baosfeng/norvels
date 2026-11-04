#!/usr/bin/env python3
"""容量压测：生成合成 SQLite 库并测量核心查询，输出可直接贴进文档的 markdown 表格。

设计要点
--------
* 默认小规模（500 本 x 500 章）；百万本 / 上亿章必须显式传参，且开跑前先查磁盘；
* 库建在临时目录（默认 $TMPDIR/norvels-bench-xxxx），跑完自动删除，--keep 保留；
* 主库形态对齐 0018 / 0022 之后的生产库：chapters 只有元数据列（正文在独立
  norvels-content.db），且带 chapter_length_hist / import_daily / author_agg 三张预计算表；
* 只依赖 Python 标准库 + sqlite3，不碰 data/norvels.db，也不需要 Go 二进制。

两条独立的外推轴（也就是「为什么可以抽样」）
--------------------------------------------
* **元数据轴**：books / authors / books_authors 的行数。书架分页、书名检索、authors 聚合
  只扫这几张表，与 chapters 无关 —— 要到百万本就必须真建百万行，抽不了样。
* **章节轴**：COUNT / SUM / 直方图 / import-trend 的章节侧，SQL 全是对 chapters 的一次
  顺序扫描，代价与行数严格线性 —— 所以章节只需建到千万级，用实测值按行数比例外推。

--sample-books 决定「给前几本书建章节」：元数据始终全量，章节按需缩减。

输出约定
--------
进度与警告走 stderr，markdown 表格走 stdout，因此：

    python3 scripts/bench-capacity.py > capacity-table.md

拿到的就是干净的文档片段。

用法
----
    python3 scripts/bench-capacity.py                                   # 500 本 x 500 章
    python3 scripts/bench-capacity.py --books 2000 --chapters 500
    python3 scripts/bench-capacity.py --books 50000 --chapters 0         # 仅元数据
    python3 scripts/bench-capacity.py --books 1000000 --chapters 346 --sample-books 86000
    python3 scripts/bench-capacity.py --books 1000000 --chapters 0
    python3 scripts/bench-capacity.py --books 5000 --chapters 5000 --keep

字段口径
--------
* 首查：该查询在本次进程里的第一次执行（含页缓存未命中的冷开销）；
* P50/P95/max：首查之后的 --samples 次采样统计；
* 结果：单次执行返回的行数 / 标量，用来确认查询真的算出了东西；
* 形态：**旧** = 请求路径上的全表聚合，**中间** = 0022 之前的最优解（覆盖索引等，
  常数更小但增长率不变），**新** = 0022 的预计算表（与行数无关）。
"""

from __future__ import annotations

import argparse
import os
import random
import shutil
import sqlite3
import statistics
import sys
import tempfile
import time
from typing import Callable, Dict, List, Optional, Sequence

NL = chr(10)
BQ = chr(96)

# ---------- 生产库实测口径（代码里推不出来，所以写死；全部来自 NAS 生产库）----------
# 71,320 本 / 24,697,647 章 —— 平均每本 346.3 章。
# 旧文档的「每本 504 章」来自 838 本的早期样本，已被全量生产库推翻，不再沿用。
PROD_BOOKS = 71320
PROD_CHAPTERS = 24697647
PROD_CHAPTERS_PER_BOOK = PROD_CHAPTERS / PROD_BOOKS

# 主库 / 正文库体积（单位就是下面写死的那一个：按同一单位等比缩放，不用换算）。
# 主库 = 元数据 + 章节行 + 索引；正文库 = chapter_blocks 的压缩块。
# 正文库取 61.8 GB（/api/stats 的 text_bytes 口径，也就是 packed 合计），而不是取整的 59 GB：
# 三分项 61.8 + 1.15 + 2.9 = 65.85 GB 与「主库 2.8 + 正文库 61.8 = 64.6 GB」只差 1.9%，
# 落在 fillComposition 的归一容差内；若正文库取 59 GB，三分项之和会比文件合计多 6% 以上。
PROD_MAIN_SIZE = 2.8         # 主库（元数据 + 章节行 + 索引），单位 GB
PROD_CONTENT_FILE_SIZE = 61.8  # 正文库（压缩后正文块），单位 GB
PROD_TEXT_BYTES = 61.8       # /api/stats 的 text_bytes，与上面同口径
PROD_INDEX_BYTES = 1.15      # index_bytes，单位 GB
PROD_OTHER_BYTES = 2.9       # other_bytes，单位 GB

# 章节长度 5 段：与 internal/store/health.go 的 lengthBucketEdges 一致，
# 权重与均值代理来自真实库 42.3 万章实测（2.3% / 67.4% / 14.7% / 11.8% / 3.9%）。
LENGTH_EDGES = (0, 1000, 3000, 5000, 10000)
LENGTH_SHARES = (0.023, 0.673, 0.147, 0.118, 0.039)
LENGTH_RANGES = ((400, 1000), (1000, 3000), (3000, 5000), (5000, 10000), (10000, 20000))
DEFAULT_CHARS = 3422  # 真实库实测平均每章字数

# 0022 的三张预计算表（与 internal/store/dashboard_precompute.go 的常量同名）。
TABLE_HIST = "chapter_length_hist"
TABLE_DAILY = "import_daily"
TABLE_AUTHOR_AGG = "author_agg"

# authorWorks 子查询：与 internal/store/authors.go 的常量逐字一致。
AUTHOR_WORKS = (
    "(SELECT author_id AS aid, id AS book_id FROM books WHERE author_id <> 0 "
    "UNION SELECT author_id AS aid, book_id FROM books_authors)"
)

CJK_POOL = (
    "长夜书简青川剑砚山河故人灯下客茶马古道霜天晓角风起时纸上谈兵"
    "木叶归尘孤舟听雨竹外桃花客舍青青北望星河一枕黄粱云中谁寄"
    "白露为霜秋水共长天落霞孤鹜雁字回时月满西楼雨打芭蕉灯花瘦"
    "春江花月夜渔舟唱晚远山含黛疏影横斜暗香浮动青苔石阶旧巷深"
)


def human_size(n: float) -> str:
    """按量级选单位：小于 1 GB 用 MB，小于 1 MB 用 KB。"""
    if n >= 1024 ** 3:
        return "%.1f GB" % (n / 1024 ** 3)
    if n >= 1024 ** 2:
        return "%.1f MB" % (n / 1024 ** 2)
    return "%.1f KB" % (n / 1024)


def human_gb(n: float) -> str:
    """字节 -> GB 字符串。已经是 GB 的数别往这里送（会差 1024^3 倍）。"""
    return "%.1f GB" % (n / 1024 ** 3)


def gb(v: float) -> str:
    """已是 GB 的量直接格式化，不做单位换算。"""
    return "%.1f GB" % v


def chapter_total(args) -> int:
    """实际要建的章节行数：--sample-books 决定给多少本书建章节。"""
    if args.chapters <= 0:
        return 0
    books_with = args.sample_books if args.sample_books > 0 else args.books
    return min(books_with, args.books) * args.chapters


def estimate_bytes(books: int, total_chapters: int, chars: int) -> int:
    """按实测倍率估算合成库体积（字节），只用于跑之前的磁盘余量检查。

    元数据侧：books 宽表约 900 字节/本（含 6 个二级索引），authors 约 64 字节/人，
    books_authors 约 40 字节/行。章节侧（content off）：行 + 3 个二级索引约 140 字节/章 ——
    这一档在 2,500 万章规模上由本脚本自测校准。
    """
    meta = books * 900 + max(1, books // 3) * 64 + books * 40
    if total_chapters <= 0:
        return int(meta)
    return int(meta + total_chapters * 140)


def check_disk(need_bytes: int, workdir: str, args) -> None:
    """空间不够就拒绝（exit 2）。WAL / 临时表 / 建索引都会额外占空间，按 1.3x 预留。"""
    required = int(need_bytes * 1.3)
    usage = shutil.disk_usage(workdir)
    print(
        "[预留] 估算库体积 %s，含 WAL/临时文件按 1.3x 预留 %s；%s 剩余 %s"
        % (human_gb(need_bytes), human_gb(required), workdir, human_gb(usage.free)),
        file=sys.stderr,
    )
    if usage.free >= required:
        return
    lines = [
        "",
        "容量压测已拒绝执行：磁盘空间不足",
        "",
        "- 规模：%d 本 x %d 章（每章约 %d 字）"
        % (args.books, args.chapters, args.chars),
        "- 需要：约 %s（库 %s + 30%% 余量）"
        % (human_gb(required), human_gb(need_bytes)),
        "- 可用：%s（%s）" % (human_gb(usage.free), workdir),
        "- 缺口：约 %s" % human_gb(short_gap(required, usage.free)),
        "",
        "处理方式（任选其一）：",
        "  1. 换一个有空间的目录：--dir /path/to/big-disk",
        "  2. 缩小规模：--books / --chapters 减半即把需求降到约 1/4",
        "  3. 只给前 N 本书建章节：--sample-books 86000（元数据仍全量）",
        "  4. 只测元数据：--chapters 0（百万本书只占 1 GB 上下）",
        "",
    ]
    for line in lines:
        print(line, file=sys.stderr)
    raise SystemExit(2)


def short_gap(required: int, free: int) -> int:
    return max(0, required - free)

# ---------- schema：对齐 0018 之后的主库（chapters 无正文列、无分词镜像列）----------
# 只建压测需要的表；books 的列取到 0021，因为生产库的宽表体积口径就来自这些列。
SCHEMA_MAIN = """
CREATE TABLE authors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    book_count INTEGER NOT NULL DEFAULT 0,
    word_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE TABLE books (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    author_id INTEGER NOT NULL DEFAULT 0,
    source_path TEXT NOT NULL UNIQUE,
    source_name TEXT NOT NULL DEFAULT '',
    file_size INTEGER NOT NULL DEFAULT 0,
    file_sha256 TEXT NOT NULL UNIQUE,
    encoding TEXT NOT NULL DEFAULT '',
    word_count INTEGER NOT NULL DEFAULT 0,
    chapter_count INTEGER NOT NULL DEFAULT 0,
    volume_count INTEGER NOT NULL DEFAULT 0,
    intro TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'parsed',
    multi_book INTEGER NOT NULL DEFAULT 0,
    imported_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    language TEXT NOT NULL DEFAULT '',
    publisher TEXT NOT NULL DEFAULT '',
    isbn TEXT NOT NULL DEFAULT '',
    pub_date TEXT NOT NULL DEFAULT '',
    series TEXT NOT NULL DEFAULT '',
    series_index INTEGER NOT NULL DEFAULT 0,
    source_format TEXT NOT NULL DEFAULT 'txt',
    has_cover INTEGER NOT NULL DEFAULT 0,
    serial_status TEXT NOT NULL DEFAULT '',
    read_status TEXT NOT NULL DEFAULT '',
    rating INTEGER NOT NULL DEFAULT 0,
    new_chapters INTEGER NOT NULL DEFAULT 0,
    last_chapter_at TEXT NOT NULL DEFAULT '',
    garbage_ratio REAL NOT NULL DEFAULT 0,
    noise_ratio REAL NOT NULL DEFAULT 0,
    structure_score REAL NOT NULL DEFAULT 0,
    duplicate_ratio REAL NOT NULL DEFAULT 0,
    quality_chapters INTEGER NOT NULL DEFAULT 0,
    quality_chars INTEGER NOT NULL DEFAULT 0,
    quality_at TEXT NOT NULL DEFAULT '',
    superseded_by INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE volumes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id INTEGER NOT NULL,
    idx INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    UNIQUE(book_id, idx)
);
CREATE TABLE chapters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id INTEGER NOT NULL,
    volume_id INTEGER NOT NULL DEFAULT 0,
    idx INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    block_id INTEGER NOT NULL DEFAULT 0,
    block_off INTEGER NOT NULL DEFAULT 0,
    block_len INTEGER NOT NULL DEFAULT 0,
    word_count INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE books_authors (
    book_id INTEGER NOT NULL,
    author_id INTEGER NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (book_id, author_id)
);
CREATE TABLE store_metrics (
    key TEXT PRIMARY KEY,
    value INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE chapter_length_hist (
    bucket INTEGER PRIMARY KEY,
    count  INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE import_daily (
    day           TEXT PRIMARY KEY,
    book_count    INTEGER NOT NULL DEFAULT 0,
    chapter_count INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE author_agg (
    author_id  INTEGER PRIMARY KEY,
    book_count INTEGER NOT NULL DEFAULT 0,
    word_count INTEGER NOT NULL DEFAULT 0
);
"""

# 二级索引：与生产库迁移里建的同名同列（idx_chapters_wc 由 size.go 在升级时补建）。
INDEXES_MAIN = """
CREATE INDEX idx_books_author   ON books(author_id);
CREATE INDEX idx_books_updated  ON books(updated_at DESC, id DESC);
CREATE INDEX idx_books_imported ON books(imported_at DESC, id DESC);
CREATE INDEX idx_books_title    ON books(title);
CREATE INDEX idx_books_chapters ON books(chapter_count DESC, id DESC);
CREATE INDEX idx_books_words    ON books(word_count DESC, id DESC);
CREATE INDEX idx_volumes_book   ON volumes(book_id, idx);
CREATE INDEX idx_chapters_book_idx ON chapters(book_id, idx, id);
CREATE INDEX idx_chapters_volume   ON chapters(volume_id);
CREATE INDEX idx_chapters_wc       ON chapters(word_count);
CREATE INDEX idx_books_authors_author ON books_authors(author_id);
CREATE INDEX idx_author_agg_words ON author_agg(word_count DESC, author_id);
CREATE INDEX idx_author_agg_books ON author_agg(book_count DESC, author_id);
"""

def rand_text(pool: str, n: int, rng: random.Random) -> str:
    """从固定字池里取 n 个汉字（带随机偏移，让相邻章节内容不同）。"""
    start = rng.randrange(0, len(pool))
    return (pool * (n // len(pool) + 2))[start:start + n]


def pick_word_count(rng: random.Random, mean_chars: int) -> int:
    """按真实库的 5 段长度分布抽一个章字数（段内均匀），再按 --chars 整体缩放。"""
    r = rng.random()
    acc = 0.0
    bucket = len(LENGTH_SHARES) - 1
    for i, share in enumerate(LENGTH_SHARES):
        acc += share
        if r < acc:
            bucket = i
            break
    lo, hi = LENGTH_RANGES[bucket]
    base = rng.uniform(lo, hi)
    return max(50, int(base * mean_chars / DEFAULT_CHARS))


def build_authors(conn, books: int, rng: random.Random) -> int:
    """建作者行：约每 3 本 1 位，20% 的书没有作者（author_id = 0）。"""
    total = max(1, books // 3)
    batch = []
    for i in range(1, total + 1):
        batch.append((i, ("佚名%06d" % i) if i % 5 else ("示例作者%06d" % i), 0, 0))
        if len(batch) >= 20000:
            conn.executemany(
                "INSERT INTO authors(id, name, book_count, word_count) VALUES (?,?,?,?)", batch)
            batch = []
    if batch:
        conn.executemany(
            "INSERT INTO authors(id, name, book_count, word_count) VALUES (?,?,?,?)", batch)
    return total


def build_books(conn, args, author_total: int, rng: random.Random, pool: str) -> None:
    """建 books 行。imported_at 均匀铺在最近 --import-spread-days 天里，供趋势查询用。"""
    per_book_chars = max(0, args.chapters) * args.chars
    day_span = max(1, args.import_spread_days)
    batch = []
    for i in range(1, args.books + 1):
        title = rand_text(pool, rng.choice((2, 3, 4, 5)), rng)
        if i <= 20:
            title = "长夜书简" + title  # 给书名 LIKE 压测一个稳定可命中的子串
        day = rng.randrange(day_span)
        stamp = "2026-%02d-%02dT03:04:05.000Z" % (1 + day // 28, 1 + day % 28)
        batch.append((
            i, title, 0 if i % 5 == 0 else rng.randrange(1, author_total + 1),
            "/path/to/txt/sample-%07d.txt" % i, "sample-%07d.txt" % i,
            per_book_chars * 3, "%064x" % i,
            rng.choice(("gb18030", "utf-8", "utf-8-bom", "utf-16le")),
            per_book_chars, args.chapters, 0, "", "imported", 0, stamp, stamp,
        ))
        if len(batch) >= 20000:
            conn.executemany(BOOK_INSERT_SQL, batch)
            batch = []
    if batch:
        conn.executemany(BOOK_INSERT_SQL, batch)


BOOK_INSERT_SQL = (
    "INSERT INTO books(id, title, author_id, source_path, source_name, file_size,"
    " file_sha256, encoding, word_count, chapter_count, volume_count, intro, status,"
    " multi_book, imported_at, updated_at)"
    " VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
)


def build_books_authors(conn, args, rng: random.Random) -> None:
    """次作者关联：约 15% 的书挂第二个作者（UNION 子查询的真实规模来源）。"""
    extra = int(args.books * 0.15)
    batch = []
    for i in range(1, extra + 1):
        book_id = rng.randrange(1, args.books + 1)
        batch.append((book_id, rng.randrange(1, max(1, args.books // 3) + 1), 1))
        if len(batch) >= 20000:
            conn.executemany(
                "INSERT OR IGNORE INTO books_authors(book_id, author_id, position)"
                " VALUES (?,?,?)", batch)
            batch = []
    if batch:
        conn.executemany(
            "INSERT OR IGNORE INTO books_authors(book_id, author_id, position)"
            " VALUES (?,?,?)", batch)



def bulk_insert_chapters(conn, args, rng: random.Random) -> Dict[str, int]:
    """给前 sample-books 本书建章节行，返回行数与字数总和。

    只写 chapters 的元数据列：0018 之后正文在正文库，主库这张表本来就只有这些列，
    所以「元数据侧」的容量与延迟可以完全脱离正文来测。
    """
    books_with = args.sample_books if args.sample_books > 0 else args.books
    books_with = min(books_with, args.books)
    total = books_with * args.chapters
    sql = ("INSERT INTO chapters(book_id, volume_id, idx, title, block_id, block_off,"
           " block_len, word_count) VALUES (?,?,?,?,?,?,?,?)")
    batch = []
    written = 0
    word_sum = 0
    next_note = 5_000_000
    for b in range(1, books_with + 1):
        words = 0
        for c in range(1, args.chapters + 1):
            wc = pick_word_count(rng, args.chars)
            words += wc
            word_sum += wc
            batch.append((b, 0, c, "第%d章" % c, 0, 0, wc * 3, wc))
            if len(batch) >= 5000:
                conn.executemany(sql, batch)
                written += len(batch)
                batch = []
                if written >= next_note:
                    print("[建库] 章节 %d/%d（%.0f%%）"
                          % (written, total, written * 100.0 / max(1, total)), file=sys.stderr)
                    next_note += 5_000_000
        if b % 20000 == 0:
            conn.commit()
        conn.execute("UPDATE books SET word_count = ?, file_size = ? WHERE id = ?",
                     (words, words * 3, b))
    if batch:
        conn.executemany(sql, batch)
        written += len(batch)
    conn.commit()
    return {"rows": written, "word_sum": word_sum}


def backfill_precompute(conn, args) -> None:
    """回填 0022 的三张预计算表（迁移回填 + 写入路径增量维护的等价结果）。

    生产里这三张表由导入与编辑同事务维护，压测库没有写入路径，所以建完库
    直接按业务表算一次，与 refresh-metrics 的回填口径一致。
    """
    # chapter_length_hist：桶号 = lengthBucketEdges 的下标，与 store.bucketIndex 一致。
    conn.execute("DELETE FROM " + TABLE_HIST)
    rows = conn.execute(bucket_case_sql(LENGTH_EDGES)).fetchall()
    conn.executemany("INSERT INTO " + TABLE_HIST + "(bucket, count) VALUES (?, ?)",
                     [(b, c) for b, c in sorted(rows)])

    # import_daily：按 UTC 日聚（读侧再按本地时区映射），这里与库里的 imported_at 同源。
    conn.execute("DELETE FROM " + TABLE_DAILY)
    conn.execute(
        "INSERT INTO " + TABLE_DAILY + "(day, book_count, chapter_count) "
        "SELECT substr(imported_at, 1, 10), COUNT(*), COALESCE(SUM(chapter_count), 0) "
        "FROM books GROUP BY substr(imported_at, 1, 10)")

    # author_agg：与 authorWorks（主作者 ∪ 副作者）同口径。
    conn.execute("DELETE FROM " + TABLE_AUTHOR_AGG)
    conn.execute(
        "INSERT INTO " + TABLE_AUTHOR_AGG + "(author_id, book_count, word_count) "
        "SELECT a.id, COUNT(b.id), COALESCE(SUM(b.word_count), 0) "
        "FROM authors a LEFT JOIN " + AUTHOR_WORKS + " x ON x.aid = a.id "
        "LEFT JOIN books b ON b.id = x.book_id GROUP BY a.id")
    conn.commit()


def create_library(db_path: str, args) -> Dict[str, object]:
    """建合成库。元数据（books/authors/books_authors）全量，章节按 --sample-books 抽样。"""
    rng = random.Random(args.seed)
    pool = CJK_POOL * 8
    conn = sqlite3.connect(db_path)
    conn.executescript(
        "PRAGMA journal_mode=OFF; PRAGMA synchronous=OFF; PRAGMA temp_store=MEMORY;"
        "PRAGMA cache_size=-131072;"
    )
    conn.executescript(SCHEMA_MAIN)
    t_build = time.perf_counter()

    author_total = build_authors(conn, args.books, rng)
    print("[建库] 作者 %d 位" % author_total, file=sys.stderr)
    build_books(conn, args, author_total, rng, pool)
    print("[建库] 书目 %d 本" % args.books, file=sys.stderr)
    build_books_authors(conn, args, rng)

    chapter_rows = 0
    word_sum = 0
    if args.chapters > 0:
        print("[建库] 开始写章节（%d 本 x %d 章）" % (args.sample_books or args.books,
                                                     args.chapters), file=sys.stderr)
        built = bulk_insert_chapters(conn, args, rng)
        chapter_rows = built["rows"]
        word_sum = built["word_sum"]
        print("[建库] 章节 %s 行、%s 字，开始建索引"
              % (format(chapter_rows, ","), format(built["word_sum"], ",")), file=sys.stderr)
    # 索引最后建：先写数据再建索引比「带索引逐行插」快一个数量级。
    conn.executescript(INDEXES_MAIN)
    conn.commit()

    # store_metrics 预计算值：模拟导入时同事务累加的结果（新形态直接查这张表）。
    conn.executemany(
        "INSERT INTO store_metrics(key, value, updated_at) VALUES (?,?,?)",
        [
            ("chapter_count", chapter_rows, "2026-11-04T00:00:00.000Z"),
            ("chapter_word_sum", word_sum, "2026-11-04T00:00:00.000Z"),
            ("text_bytes", word_sum * 3, "2026-11-04T00:00:00.000Z"),
            ("metrics_version", 4, "2026-11-04T00:00:00.000Z"),
        ],
    )
    backfill_precompute(conn, args)
    conn.commit()
    conn.execute("PRAGMA optimize")
    conn.commit()
    build_secs = time.perf_counter() - t_build
    conn.close()
    return {"build_secs": build_secs, "chapter_rows": chapter_rows, "word_sum": word_sum}


def pct(values: Sequence[float], p: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    idx = min(len(ordered) - 1, max(0, int(round(p / 100.0 * (len(ordered) - 1)))))
    return ordered[idx]


def bench_query(
    conn: sqlite3.Connection,
    label: str,
    sql: str,
    params_factory: Callable[[int], Sequence],
    summarize: Callable[[List[tuple]], str],
    samples: int,
    warmup: int,
    form: str = "新",
    count_wrap: bool = False,
    group: str = "cockpit",
) -> Dict[str, object]:
    """跑一条查询的 warmup + samples 次，返回 P50/P95/max。

    count_wrap=True 时把语句包成 SELECT COUNT(*) FROM (…)：用来测「扫描 + 连接」
    的引擎侧代价，避免把上千万行结果搬进 Python 堆里，那测的是本脚本而不是库。
    """
    if count_wrap:
        sql = "SELECT COUNT(*) FROM (" + sql + ")"
        summarize = lambda rows: ("扫描 %s 行" % format(rows[0][0], ",")) if rows else "0 行"
    first_ms = 0.0
    times = []  # type: List[float]
    summary = ""
    for i in range(warmup + samples):
        params = params_factory(i)
        start = time.perf_counter()
        rows = conn.execute(sql, params).fetchall()
        elapsed = (time.perf_counter() - start) * 1000.0
        summary = summarize(rows)
        if i == 0:
            first_ms = elapsed
        if i >= warmup:
            times.append(elapsed)
    return {
        "label": label,
        "form": form,
        "group": group,
        "first": first_ms,
        "p50": statistics.median(times) if times else 0.0,
        "p95": pct(times, 95),
        "max": max(times) if times else 0.0,
        "samples": len(times),
        "summary": summary,
    }



# ---------- 驾驶舱式聚合：新形态 vs 旧形态 ----------

def bucket_range_sql(edges) -> str:
    """复刻 store.bucketRangeSQL：一条 SELECT 里 N 个区间 COUNT 子查询。

    每个子查询是 (SELECT COUNT(*) … WHERE col >= ? AND col < ?)，走 idx_chapters_wc
    覆盖索引做区间 SEARCH，只读 word_count 一列。刻意不写成 SUM(CASE WHEN …)：
    那是全索引扫描，慢一个量级，压测要压的是生产里真正跑的语句。
    """
    parts = []
    for i, lo in enumerate(edges):
        if i + 1 < len(edges):
            parts.append("(SELECT COUNT(*) FROM chapters WHERE word_count >= %d"
                         " AND word_count < %d)" % (lo, edges[i + 1]))
        else:
            parts.append("(SELECT COUNT(*) FROM chapters WHERE word_count >= %d)" % lo)
    return "SELECT " + ", ".join(parts)


def bucket_case_sql(edges) -> str:
    """旧形态：CASE WHEN 分桶 + GROUP BY，要对整条索引做一次全量扫描。"""
    expr = "CASE"
    for i in range(len(edges) - 1, 0, -1):
        expr += " WHEN word_count >= %d THEN %d" % (edges[i], i)
    expr += " ELSE 0 END"
    return "SELECT %s AS bucket, COUNT(*) FROM chapters GROUP BY bucket" % expr


def trend_tables(conn) -> Dict[str, int]:
    """两张趋势来源表：旧形态要走 chapters JOIN books，新形态只走 books。"""
    return {
        "chapters": conn.execute("SELECT COUNT(*) FROM chapters").fetchone()[0],
        "books": conn.execute("SELECT COUNT(*) FROM books").fetchone()[0],
    }


TREND_OLD_SQL = (
    "SELECT b.imported_at FROM chapters c JOIN books b ON b.id = c.book_id"
    " WHERE b.imported_at >= ?"
)
TREND_NEW_SQL = "SELECT imported_at, chapter_count FROM books WHERE imported_at >= ?"


def add_interactive_benches(conn, rows: List[Dict[str, object]], args, rng: random.Random,
                            books: int, chapters: int, max_id: int) -> None:
    """交互路径：这些应当与总行数无关，是「百万本还能不能用」的底线证据。"""
    draws = max(1, args.samples) + max(4, args.warmup) + 2
    read_ids = [rng.randrange(1, max(max_id, 1) + 1) for _ in range(draws)]
    book_ids = [rng.randrange(1, max(books, 1) + 1) for _ in range(draws)]
    s, w = args.samples, args.warmup

    rows.append(bench_query(
        conn, "书架分页（word_count DESC, LIMIT 20 + OFFSET）",
        "SELECT id, title, word_count FROM books ORDER BY word_count DESC, id DESC"
        " LIMIT 20 OFFSET ?",
        lambda i: (book_ids[i] % max(1, books - 20),),
        lambda r: "%d 行" % len(r), s, w, group="interactive"))

    rows.append(bench_query(
        conn, "书名 LIKE 全表（%长夜书简%）",
        "SELECT id, title FROM books WHERE title LIKE ?",
        lambda i: ("%长夜书简%",), lambda r: "%d 行" % len(r), s, w, group="interactive"))

    rows.append(bench_query(
        conn, "书名 + 作者 LIKE（JOIN authors）",
        "SELECT b.id, b.title, a.name FROM books b LEFT JOIN authors a ON a.id = b.author_id"
        " WHERE b.title LIKE ? OR IFNULL(a.name,'') LIKE ?",
        lambda i: ("%长夜书简%", "%佚名0000%"), lambda r: "%d 行" % len(r), s, w, group="interactive"))

    if chapters > 0:
        rows.append(bench_query(
            conn, "单本目录（按 book_id + idx，%d 章）" % args.chapters,
            "SELECT id, idx, title, word_count FROM chapters WHERE book_id = ? ORDER BY idx",
            lambda i: (book_ids[i] % max(1, min(args.sample_books or books, books)),),
            lambda r: "%d 行" % len(r), s, max(w, 3), group="interactive"))

        rows.append(bench_query(
            conn, "随机读一章元数据（按主键）",
            "SELECT id, book_id, idx, title, block_id, block_off, block_len, word_count"
            " FROM chapters WHERE id = ?",
            lambda i: (read_ids[i],),
            lambda r: ("块 %d + %d 字节" % (r[0][4], r[0][7])) if r else "0 行",
            s, max(w, 3), group="interactive"))


def add_cockpit_benches(conn, rows: List[Dict[str, object]], args) -> None:
    """驾驶舱聚合，每条查询都给三层形态的实测：

    旧    = 请求路径上做全表聚合（0022 之前的原始形态）
    中间  = 覆盖索引 / 只扫小表等局部优化（0022 之前的最优解，仍随行数线性）
    新    = 0022 的预计算表（写入时维护，读路径只查表，与行数无关）
    """
    s, w = args.samples, args.warmup
    counts = trend_tables(conn)
    chapter_rows = counts["chapters"]
    start_day = "2026-01-01T00:00:00.000Z"

    # 1) 章节总数。
    rows.append(bench_query(
        conn, "章节总数 COUNT(*) FROM chapters", "SELECT COUNT(*) FROM chapters",
        lambda i: (), lambda r: "%s 章" % format(r[0][0], ","), s, w, form="旧"))
    rows.append(bench_query(
        conn, "章节总数 读 store_metrics.chapter_count",
        "SELECT value FROM store_metrics WHERE key = ?", lambda i: ("chapter_count",),
        lambda r: "%s 章" % format(r[0][0], ","), s, w, form="新"))

    if chapter_rows > 0:
        # 2) 章节长度直方图：三层齐全，正好是同一件事的三次演进。
        rows.append(bench_query(
            conn, "章节长度直方图 5 段 CASE WHEN + GROUP BY",
            bucket_case_sql(LENGTH_EDGES), lambda i: (),
            lambda r: " / ".join(format(c, ",") for _, c in sorted(r)), s, w, form="旧"))
        rows.append(bench_query(
            conn, "章节长度直方图 5 段 区间 COUNT 走 idx_chapters_wc",
            bucket_range_sql(LENGTH_EDGES), lambda i: (),
            lambda r: " / ".join(format(v, ",") for v in r[0]), s, w, form="中间"))
        rows.append(bench_query(
            conn, "章节长度直方图 5 段 读 chapter_length_hist",
            "SELECT bucket, count FROM " + TABLE_HIST + " ORDER BY bucket", lambda i: (),
            lambda r: " / ".join(format(c, ",") for _, c in r), s, w, form="新"))

        # 3) import-trend 章节按日：三层。
        rows.append(bench_query(
            conn, "import-trend 章节按日 chapters JOIN books",
            TREND_OLD_SQL, lambda i: (start_day,),
            lambda r: "%d 行" % len(r), s, w, form="旧", count_wrap=True))
        rows.append(bench_query(
            conn, "import-trend 章节按日 books 归集 SUM(chapter_count)",
            TREND_NEW_SQL, lambda i: (start_day,),
            lambda r: "%d 行" % len(r), s, w, form="中间", count_wrap=True))
        rows.append(bench_query(
            conn, "import-trend 章节按日 读 import_daily",
            "SELECT day, book_count, chapter_count FROM " + TABLE_DAILY
            + " WHERE day >= ? ORDER BY day",
            lambda i: ("2026-01-01",),
            lambda r: "%d 天" % len(r), s, w, form="新"))

        # 4) 章节字数总和。
        rows.append(bench_query(
            conn, "章节字数总和 SUM(word_count) FROM chapters",
            "SELECT SUM(word_count) FROM chapters", lambda i: (),
            lambda r: "%s 字" % format(r[0][0] or 0, ","), s, w, form="旧"))
        rows.append(bench_query(
            conn, "章节字数总和 读 store_metrics.chapter_word_sum",
            "SELECT value FROM store_metrics WHERE key = ?", lambda i: ("chapter_word_sum",),
            lambda r: "%s 字" % format(r[0][0], ","), s, w, form="新"))

    # 5) authors 聚合：旧 = LEFT JOIN UNION 子查询 + GROUP BY；新 = author_agg 物化。
    # 两条都取 Top 50，与接口默认分页一致。
    author_live = (
        "SELECT a.id, a.name, COUNT(b.id), COALESCE(SUM(b.word_count), 0) "
        "FROM authors a LEFT JOIN " + AUTHOR_WORKS + " x ON x.aid = a.id "
        "LEFT JOIN books b ON b.id = x.book_id "
        "GROUP BY a.id, a.name ORDER BY COUNT(b.id) DESC, a.name COLLATE NOCASE, a.id LIMIT 50")
    author_agg = (
        "SELECT a.id, a.name, g.book_count, g.word_count "
        "FROM authors a JOIN " + TABLE_AUTHOR_AGG + " g ON g.author_id = a.id "
        "ORDER BY g.book_count DESC, a.name COLLATE NOCASE, a.id LIMIT 50")
    rows.append(bench_query(
        conn, "authors 聚合 LEFT JOIN UNION 子查询 + GROUP BY",
        author_live, lambda i: (), lambda r: "%d 行（首位 %d 本）" % (
            len(r), r[0][2] if r else 0), s, w, form="旧"))
    rows.append(bench_query(
        conn, "authors Top50 读 author_agg",
        author_agg, lambda i: (), lambda r: "%d 行（首位 %d 本）" % (
            len(r), r[0][2] if r else 0), s, w, form="新"))


# 外推轴：哪条查询随什么增长。固定 = 交互路径，与总量无关。
# (标签前缀, 形态) -> 外推轴。同一条业务查询的三层形态轴可能不同，
# 所以键必须带形态：「章节长度直方图 + 新」读预计算表是固定轴，
# 而「章节长度直方图 + 旧/中间」都要扫 chapters，是章节轴。
AXIS_RULES = (
    ("书架分页", "*", "书"),
    ("书名 LIKE", "*", "书"),
    ("书名 + 作者", "*", "书"),
    ("单本目录", "*", "固定"),
    ("随机读一章元数据", "*", "固定"),
    ("章节总数 COUNT", "旧", "章"),
    ("章节长度直方图", "旧", "章"),
    ("章节长度直方图", "中间", "章"),
    ("章节字数总和 SUM", "旧", "章"),
    ("import-trend 章节按日", "旧", "章"),
    ("import-trend 章节按日", "中间", "书"),
    ("authors 聚合", "旧", "书"),
)


def annotate_axes(rows: List[Dict[str, object]]) -> None:
    """按 (标签前缀, 形态) 标外推轴；没命中的一律当固定轴（交互路径）。"""
    for r in rows:
        label = str(r["label"])
        form = str(r["form"])
        r["axis"] = "固定"
        for prefix, want_form, axis in AXIS_RULES:
            if label.startswith(prefix) and want_form in ("*", form):
                r["axis"] = axis
                break


def run_benchmarks(db_path: str, args) -> List[Dict[str, object]]:
    conn = sqlite3.connect(db_path)
    books = conn.execute("SELECT COUNT(*) FROM books").fetchone()[0]
    chapters = conn.execute("SELECT COUNT(*) FROM chapters").fetchone()[0]
    max_id = conn.execute("SELECT COALESCE(MAX(id),0) FROM chapters").fetchone()[0]
    rng = random.Random(args.seed + 1)

    rows = []  # type: List[Dict[str, object]]
    add_interactive_benches(conn, rows, args, rng, books, chapters, max_id)
    add_cockpit_benches(conn, rows, args)
    conn.close()
    annotate_axes(rows)
    return rows


def db_size_bytes(path: str) -> int:
    total = 0
    for suffix in ("", "-wal", "-shm"):
        p = path + suffix
        if os.path.exists(p):
            total += os.path.getsize(p)
    return total


def fmt_ms(v: float) -> str:
    """单次耗时（毫秒）：>=1 s 用秒，>=10 ms 取整，更小的保留两位小数。"""
    if v >= 1000:
        return "%.2f s" % (v / 1000)
    return "%.0f" % v if v >= 10 else "%.2f" % v


def fmt_secs(ms: float) -> str:
    """毫秒转人类可读：>=1 s 用秒，否则用毫秒（保留两位小数看小值）。"""
    if ms >= 1000:
        return "%.2f s" % (ms / 1000)
    return "%.2f ms" % ms if ms < 10 else "%.0f ms" % ms


def scale_factor(row, args, measured_books: int, measured_chapters: int) -> float:
    axis = row["axis"]
    if axis == "书":
        return float(target_books(args)) / max(1, measured_books)
    if axis == "章":
        return float(target_chapters(args)) / max(1, measured_chapters)
    return 1.0


def target_books(args) -> int:
    """外推目标书数：未指定 --target-books 时就是本次实跑的规模。"""
    return args.target_books if args.target_books > 0 else args.books


def target_chapters(args) -> int:
    """外推目标章数。

    三个来源，优先级从高到低：
    1. 显式 --target-books：按生产库每本 346.3 章换算；
    2. 有抽样（--sample-books）：本次只给部分书建了章节，目标仍是全量书数 x 346.3；
    3. 没有抽样：目标就是本次实测的章数，不做任何外推（默认 500x500 不该被推成 17 万章）。
    """
    if args.target_books > 0:
        return int(round(args.target_books * PROD_CHAPTERS_PER_BOOK))
    if args.sample_books > 0:
        return int(round(args.books * PROD_CHAPTERS_PER_BOOK))
    return chapter_total(args)


def emit_scale_section(out: List[str], args, results, db_path: str) -> None:
    """第 1 段：本次实跑的规模与体积。"""
    size = db_size_bytes(db_path)
    books = args.books
    total_ch = chapter_total(args)
    out.append("### 容量压测：%d 本 x %s（合成库）" % (books, "%d 章/本" % args.chapters
                                                    if args.chapters else "仅元数据"))
    out.append("")
    out.append("| 指标 | 数值 |")
    out.append("|---|---|")
    out.append("| 书数 | %s |" % format(books, ","))
    if total_ch > 0:
        out.append("| 章节数（实测建库） | %s（前 %s 本有章节） |"
                   % (format(total_ch, ","), format(min(args.sample_books or books, books), ",")))
    out.append("| 生产库口径 | 每本 %.1f 章（外推按这个倍率） |" % PROD_CHAPTERS_PER_BOOK)
    out.append("| 章节正文 | 不在主库：0018 起正文在独立的 norvels-content.db |")
    out.append("| 正文索引（FTS5 + 分词镜像） | 未建（0017 之后的生产形态） |")
    out.append("| 主库体积 | %s |" % human_size(size))
    out.append("| 每本平均 | %s |" % human_size(size / books))
    if total_ch > 0:
        out.append("| 每章平均 | %.0f 字节（不含正文） |" % (size / total_ch))
    out.append("| 建库耗时 | %.1f s |" % results["build_secs"])
    out.append("")


def emit_bench_table(out: List[str], results: List[Dict[str, object]], title: str,
                     match: Callable[[Dict[str, object]], bool]) -> None:
    picked = [r for r in results["rows"] if match(r)]
    if not picked:
        return
    out.append(title)
    out.append("")
    out.append("| 操作 | 形态 | 首查 | P50 | P95 | max | 采样 | 结果 |")
    out.append("|---|---|---|---|---|---|---|---|")
    for r in picked:
        out.append("| %s | %s | %s | %s | %s | %s | %d | %s |"
                   % (r["label"], r["form"], fmt_ms(r["first"]), fmt_ms(r["p50"]),
                      fmt_ms(r["p95"]), fmt_ms(r["max"]), r["samples"], r["summary"]))
    out.append("")



def emit_projection(out: List[str], args, results) -> None:
    """第 2 段：把实测按外推轴换算到目标规模，并标出超 5s 的操作。"""
    measured_books = args.books
    measured_ch = chapter_total(args)
    goal_books = target_books(args)
    target_ch = target_chapters(args)
    if goal_books == measured_books and target_ch == measured_ch:
        return

    out.append("### 外推：%s 本 / %s 章" % (format(goal_books, ","), format(target_ch, ",")))
    out.append("")
    out.append("> 外推轴逐条见下表「轴」列：**章** = 对 chapters 的顺序扫描，代价与行数严格线性；")
    out.append("> **书** = 只扫 books / authors；**固定** = 走 B-tree SEARCH 或按主键读，与总量无关。")
    out.append("> 推算值 = 本次实测 P50 / P95 x 目标规模 / 实测规模，按轴取值。")
    out.append("")
    out.append("| 操作 | 形态 | 轴 | 实测 P50 | 推算 P50 | 推算 P95 | 判定（阈值 5 s） |")
    out.append("|---|---|---|---|---|---|---|")
    for r in results["rows"]:
        if r["axis"] == "章" and measured_ch <= 0:
            # 本次没建章节，章节侧的数字是空表的噪声，外推出去等于编数字。
            out.append("| %s | %s | 章 | 未测 | — | — | 需 --chapters > 0 |"
                       % (r["label"], r["form"]))
            continue
        f = scale_factor(r, args, measured_books, measured_ch)
        p50 = r["p50"] * f
        p95 = r["p95"] * f
        verdict = "**不可用**" if p50 > 5000 else ("临界" if p50 > 1000 else "可用")
        out.append("| %s | %s | %s | %s | %s | %s | %s |"
                   % (r["label"], r["form"], r["axis"], fmt_ms(r["p50"]),
                      fmt_secs(p50), fmt_secs(p95), verdict))
    out.append("")


def emit_size_projection(out: List[str], args, results, db_path: str) -> None:
    """第 3 段：主库与正文库体积外推。

    生产库里「每本 346.3 章」与总规模同比例放大（1,000,000 / 71,320 = 14.02，
    346,293,424 / 24,697,647 = 14.02），所以按章数线性外推等价于同时按书本数外推，
    不需要再单独加一项书目元数据。
    """
    measured_ch = chapter_total(args)
    target_ch = target_chapters(args)
    goal_books = target_books(args)
    if measured_ch <= 0:
        return
    main_bytes = db_size_bytes(db_path)
    per_chapter_main = main_bytes / max(1, measured_ch)
    ratio = target_ch / float(PROD_CHAPTERS)
    prod_per_chapter = PROD_MAIN_SIZE * 1024 ** 3 / PROD_CHAPTERS
    main_at_target = prod_per_chapter * target_ch

    out.append("### 体积推算：%s 本 / %s 章" % (format(goal_books, ","), format(target_ch, ",")))
    out.append("")
    out.append("| 口径 | 生产库实测 | 每章摊薄 | 推算 @ %s 章 |" % format(target_ch, ","))
    out.append("|---|---|---|---|")
    out.append("| **主库文件**（元数据 + 章节行 + 索引） | %.1f GB | %.0f 字节 | **%s** |"
               % (PROD_MAIN_SIZE, prod_per_chapter, human_gb(main_at_target)))
    out.append("| **正文库文件**（chapter_blocks 压缩块） | %.1f GB | %.2f KB | **%s** |"
               % (PROD_CONTENT_FILE_SIZE, PROD_CONTENT_FILE_SIZE * 1024 ** 2 / PROD_CHAPTERS,
                  gb(PROD_CONTENT_FILE_SIZE * ratio)))
    out.append("| **合计占用（两文件之和）** | **%.1f GB** | **%.2f KB** | **%s** |"
               % (PROD_MAIN_SIZE + PROD_CONTENT_FILE_SIZE,
                  (PROD_MAIN_SIZE + PROD_CONTENT_FILE_SIZE) * 1024 ** 2 / PROD_CHAPTERS,
                  gb((PROD_MAIN_SIZE + PROD_CONTENT_FILE_SIZE) * ratio)))
    out.append("")
    out.append("stats 的三分项是同一批数据的另一种切法，**不要与上面两行重复相加**：")
    out.append("")
    out.append("| 三分项 | 生产库实测 | 推算 @ %s 章 | 属于 |" % format(target_ch, ","))
    out.append("|---|---|---|---|")
    out.append("| text_bytes（正文块 packed） | %.1f GB | %s | 正文库 |"
               % (PROD_TEXT_BYTES, gb(PROD_TEXT_BYTES * ratio)))
    out.append("| index_bytes（btree + 页填充 + 碎片） | %.2f GB | %s | 主库 |"
               % (PROD_INDEX_BYTES, gb(PROD_INDEX_BYTES * ratio)))
    out.append("| other_bytes（表头 / 块索引 / 空闲页） | %.1f GB | %s | 两库之和的余量 |"
               % (PROD_OTHER_BYTES, gb(PROD_OTHER_BYTES * ratio)))
    out.append("")
    out.append("> 主库按生产库锚点（%.0f 字节/章）外推；本机合成库同口径实测 %s 字节/章"
               % (prod_per_chapter, format(int(per_chapter_main), ",")))
    out.append("> （%s 本 / %s 章）。合成库只建压测用到的列与索引，"
               % (format(args.books, ","), format(measured_ch, ",")))
    out.append("> 生产主库另含 volumes / tags / 阅读进度等表，故主库体积以生产锚点为准，")
    out.append("> 合成库数字只当量级校验。")
    out.append(">")
    out.append("> 三分项之和 65.85 GB 略大于两文件合计 64.6 GB：text_bytes 是正文块的 packed 口径，")
    out.append("> 与文件大小不是同一分母（正文库表头、块索引、空闲页计入 other_bytes）。")
    out.append("> fillComposition 在这种情形下按比例归一到文件合计，所以两者不必逐字节相等。")
    out.append("")


def emit_sampling_note(out: List[str], args) -> None:
    if args.sample_books <= 0 or args.sample_books >= args.books:
        return
    total_ch = chapter_total(args)
    out.append("### 抽样策略：为什么可以只给前 %s 本书建章节" % format(args.sample_books, ","))
    out.append("")
    out.append("两条正交的外推轴，各自单独验证：")
    out.append("")
    out.append("1. **元数据轴不抽样**：books / authors / books_authors 全部真建 %s 行。"
               % format(args.books, ","))
    out.append("   书架分页、书名检索、authors 聚合只扫这几张表，与 chapters 的行数无关；")
    out.append("   要给「百万本」下结论，就必须有百万行真实 B-tree，任何抽样都会让结论失效。")
    out.append("2. **章节轴按需缩减**：%s 章全部是对 chapters 的一次顺序扫描 ——"
               % format(total_ch, ","))
    out.append("   COUNT(*) / SUM(word_count) / 5 段区间 COUNT / 旧形态 chapters JOIN books，")
    out.append("   代价都是「每行读一次索引项或一次行头」，没有随机 I/O、没有排序、没有哈希表碰撞。")
    out.append("   所以只要实测段落在线性区（千万级已远超任何缓存效应），")
    out.append("   就可以按行数比例外推到 %s 章。" % format(target_chapters(args), ","))
    out.append("")
    out.append("不适用的例外已单列：按主键读、按 book_id 取目录、LIMIT 分页在表里标「固定」，")
    out.append("外推系数取 1，不做任何放大。")
    out.append("")


def emit_markdown(args, results, db_path: str) -> None:
    out = []  # type: List[str]
    emit_scale_section(out, args, results, db_path)
    emit_bench_table(out, results, "### 交互路径：与总量无关，必须始终毫秒级",
                     lambda r: r["group"] == "interactive")
    emit_bench_table(out, results,
                     "### 驾驶舱聚合：三层形态对照"
                     "（旧 = 请求路径全表聚合 · 中间 = 0022 之前的最优解 · 新 = 0022 预计算表）",
                     lambda r: r["group"] == "cockpit")
    emit_projection(out, args, results)
    emit_size_projection(out, args, results, db_path)
    emit_sampling_note(out, args)
    text = NL.join(out)
    if args.out:
        with open(args.out, "w", encoding="utf-8") as fh:
            fh.write(text + NL)
        print("[输出] 表格已写入 %s" % args.out, file=sys.stderr)
    print(text)


DEFAULT_SEED = 20261025


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        description="容量压测：合成 SQLite 库 + 核心查询基准，输出 markdown 表格",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="默认 500 本 x 500 章（小规模，make bench-capacity 用它）；"
               "百万本 / 上亿章必须显式传参，且会先做磁盘余量检查。",
    )
    p.add_argument("--books", type=int, default=500,
                   help="书目数，元数据全量建这么多行（默认 500）")
    p.add_argument("--chapters", type=int, default=500,
                   help="每本章节数（默认 500；0 = 仅元数据）")
    p.add_argument("--sample-books", type=int, default=0, dest="sample_books",
                   help="只给前 N 本书建章节（0 = 全部，默认 0）；元数据仍按 --books 全量")
    p.add_argument("--chars", type=int, default=DEFAULT_CHARS,
                   help="每章平均字数（默认 %d，取真实库实测均值）" % DEFAULT_CHARS)
    p.add_argument("--target-books", type=int, default=0, dest="target_books",
                   help="外推目标书数（默认 0 = 就按 --books 外推）；"
                        "目标章数按生产库每本 %.1f 章换算" % PROD_CHAPTERS_PER_BOOK)
    p.add_argument("--import-spread-days", type=int, default=365, dest="import_spread_days",
                   help="把 books.imported_at 铺开的天数（默认 365），供趋势查询命中")
    p.add_argument("--samples", type=int, default=20, help="每项采样次数（默认 20）")
    p.add_argument("--warmup", type=int, default=1, help="预热次数（默认 1）")
    p.add_argument("--dir", default=None, help="库所在目录（默认系统临时目录新建）")
    p.add_argument("--keep", action="store_true", help="跑完保留库文件（默认自动删除）")
    p.add_argument("--out", default=None,
                   help="把 markdown 表格另存到文件（默认只写 stdout）")
    p.add_argument("--seed", type=int, default=DEFAULT_SEED, help="随机种子（默认固定）")
    return p


def validate(p: argparse.ArgumentParser, args) -> None:
    if args.books <= 0:
        p.error("--books 必须为正整数")
    if args.chapters < 0:
        p.error("--chapters 不能为负")
    if args.chars <= 0:
        p.error("--chars 必须为正")
    if args.samples < 1:
        p.error("--samples 至少为 1")
    if args.sample_books < 0:
        p.error("--sample-books 不能为负")
    if args.sample_books > args.books:
        p.error("--sample-books 不能超过 --books（元数据是全量的）")
    if args.target_books < 0:
        p.error("--target-books 不能为负")


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    p = build_parser()
    args = p.parse_args(argv)
    validate(p, args)
    return args


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)

    workdir = args.dir or tempfile.mkdtemp(prefix="norvels-bench-")
    if not os.path.isdir(workdir):
        os.makedirs(workdir, exist_ok=True)

    need = estimate_bytes(args.books, chapter_total(args), args.chars)
    check_disk(need, workdir, args)

    db_path = os.path.join(workdir, "capacity.db")
    print("[目标] %s" % db_path, file=sys.stderr)
    keep = args.keep
    try:
        info = create_library(db_path, args)
        print("[建库] 完成，耗时 %.1f s" % info["build_secs"], file=sys.stderr)
        results = {"rows": run_benchmarks(db_path, args), "build_secs": info["build_secs"]}
        emit_markdown(args, results, db_path)
    except BaseException:
        keep = True
        print("[失败] 保留现场以便排查：%s" % db_path, file=sys.stderr)
        raise
    finally:
        if keep:
            print("[保留] %s" % db_path, file=sys.stderr)
        else:
            shutil.rmtree(workdir, ignore_errors=True)
            print("[清理] 已删除临时库", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())

