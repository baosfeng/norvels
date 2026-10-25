#!/usr/bin/env python3
"""容量压测：生成合成 SQLite 库并测量核心查询，输出可直接贴进文档的 markdown 表格。

设计要点
--------
* 默认小规模（500 本 x 500 章）；几千万章的压测必须显式传 --books/--chapters；
* 开跑前先算所需空间并检查磁盘余量，不够直接拒绝（exit 2）并给出需要的 GB 数；
* 库建在临时目录（默认 $TMPDIR/norvels-bench-xxxx），跑完自动删除，--keep 保留；
* 默认不建 FTS 正文索引（--fts on 可复现现状形态），因为几万本的容量结论针对
  “砍掉正文索引后” 的形态；也避免在笔记本上白等一次 GB 级索引构建；
* 只依赖 Python 标准库 + sqlite3，不碰 data/norvels.db，也不需要 Go 二进制。

输出约定
--------
进度与警告走 stderr，markdown 表格走 stdout，因此：

    python3 scripts/bench-capacity.py > capacity-table.md

拿到的就是干净的文档片段。

用法
----
    python3 scripts/bench-capacity.py                              # 500 本 x 500 章
    python3 scripts/bench-capacity.py --books 2000 --chapters 500
    python3 scripts/bench-capacity.py --books 50000 --chapters 0    # 仅元数据
    python3 scripts/bench-capacity.py --books 5000 --chapters 5000 --keep

字段口径
--------
* 首查：该查询在本次进程里的第一次执行（含页缓存未命中的冷开销）；
* P50/P95/max：首查之后的 --samples 次采样统计；
* 结果：单次执行返回的行数 / 标量，用来确认查询真的算出了东西。
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

# 真实全量库实测均值（838 本 / 422,715 章 / 正文 4.87 GB），仅用于输出里的量级对照。
REAL_BYTES_PER_CHAPTER = 4.87 * 1024 * 1024 * 1024 / 422715  # 约 12.1 KB

CJK_POOL = (
    "长夜书简青川剑砚山河故人灯下客茶马古道霜天晓角风起时纸上谈兵"
    "木叶归尘孤舟听雨竹外桃花客舍青青北望星河一枕黄粱云中谁寄"
    "白露为霜秋水共长天落霞孤鹜雁字回时月满西楼雨打芭蕉灯花瘦"
    "春江花月夜渔舟唱晚远山含黛疏影横斜暗香浮动青苔石阶旧巷深"
)


def estimate_bytes(books: int, chapters: int, chars: int, fts: bool) -> int:
    """按实测倍率估算合成库体积（字节）。

    实测基线：正文 4.87 GB 的分词镜像 6.28 GB + FTS 影子表 3.30 GB + 其他约 0.3 GB，
    全库 14.84 GB，约 3.05x 正文。

    压缩后（无 content_seg / 无 chapters_fts）按 1.5x 估：真实库里 8 KB 量级的章节行
    索引开销摊得薄，约 1.2x；但压测库里章节更短（默认约 2.3 KB），行数多、索引占比高，
    本脚本自测 25 万章时实测 1.41x。取 1.5x 是保守上界——宁可多拒一次，也不写爆盘。
    """
    if chapters <= 0:
        return books * 2 * 1024  # 仅元数据：每本约 2 KB（books 行 + 索引 + 作者行）
    content = books * chapters * chars * 3  # 简体中文 UTF-8 每字 3 字节
    return int(content * (3.3 if fts else 1.5))


def human_size(n: float) -> str:
    """按量级选单位：小于 1 GB 用 MB，小于 1 MB 用 KB。"""
    if n >= 1024 ** 3:
        return "%.1f GB" % (n / 1024 ** 3)
    if n >= 1024 ** 2:
        return "%.1f MB" % (n / 1024 ** 2)
    return "%.1f KB" % (n / 1024)


def human_gb(n: float) -> str:
    return "%.1f GB" % (n / 1024 ** 3)


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
    short = required - usage.free
    lines = [
        "",
        "容量压测已拒绝执行：磁盘空间不足",
        "",
        "- 规模：%d 本 x %d 章（每章约 %d 字，--fts %s）"
        % (args.books, args.chapters, args.chars, args.fts),
        "- 需要：约 %s（库 %s + 30%% 余量）"
        % (human_gb(required), human_gb(need_bytes)),
        "- 可用：%s（%s）" % (human_gb(usage.free), workdir),
        "- 缺口：约 %s" % human_gb(short),
        "",
        "处理方式（任选其一）：",
        "  1. 换一个有空间的目录：--dir /path/to/big-disk",
        "  2. 缩小规模：--books / --chapters 减半即把需求降到约 1/4",
        "  3. 缩短章节：--chars 400（体积与耗时线性下降）",
        "  4. 只测元数据：--chapters 0（几万本书只占几十 MB）",
        "",
        "参考量级：2500 万章的合成库（章节较短）实测约 15 GB。",
        "",
    ]
    for line in lines:
        print(line, file=sys.stderr)
    raise SystemExit(2)


SCHEMA_NO_FTS = """
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
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_books_author   ON books(author_id);
CREATE INDEX idx_books_updated  ON books(updated_at DESC, id DESC);
CREATE INDEX idx_books_imported ON books(imported_at DESC, id DESC);
CREATE INDEX idx_books_title    ON books(title);
CREATE INDEX idx_books_chapters ON books(chapter_count DESC, id DESC);
CREATE INDEX idx_books_words    ON books(word_count DESC, id DESC);
CREATE TABLE volumes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id INTEGER NOT NULL,
    idx INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    UNIQUE(book_id, idx)
);
CREATE INDEX idx_volumes_book ON volumes(book_id, idx);
CREATE TABLE chapters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id INTEGER NOT NULL,
    volume_id INTEGER NOT NULL DEFAULT 0,
    idx INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    word_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_chapters_book_idx ON chapters(book_id, idx, id);
CREATE INDEX idx_chapters_volume   ON chapters(volume_id);
CREATE INDEX idx_chapters_wc       ON chapters(word_count);
"""

SCHEMA_FTS_EXTRA = """
ALTER TABLE chapters ADD COLUMN title_seg TEXT NOT NULL DEFAULT '';
ALTER TABLE chapters ADD COLUMN content_seg TEXT NOT NULL DEFAULT '';
CREATE VIRTUAL TABLE chapters_fts USING fts5(
    title_seg, content_seg, content='chapters', content_rowid='id', tokenize='unicode61'
);
"""


def rand_text(pool: str, n: int, rng: random.Random) -> str:
    """从固定字池里取 n 个汉字（带随机偏移，让相邻章节内容不同）。"""
    start = rng.randrange(0, len(pool))
    return (pool * (n // len(pool) + 2))[start:start + n]


def segment(text: str) -> str:
    """复刻 store.Segment()：逐字加空格。"""
    return " ".join(text)


def create_library(db_path: str, args) -> Dict[str, float]:
    fts = args.fts == "on"  # argparse 给的是字符串，"off" 也是 truthy
    rng = random.Random(args.seed)
    pool = CJK_POOL * 8
    conn = sqlite3.connect(db_path)
    conn.executescript(
        "PRAGMA journal_mode=OFF; PRAGMA synchronous=OFF; PRAGMA temp_store=MEMORY;"
        "PRAGMA cache_size=-65536;"
    )
    conn.executescript(SCHEMA_NO_FTS)
    if fts:
        conn.executescript(SCHEMA_FTS_EXTRA)

    t_build = time.perf_counter()

    author_total = max(1, args.books // 3)
    conn.executemany(
        "INSERT INTO authors(id, name, book_count, word_count) VALUES (?,?,?,?)",
        [(i, ("佚名%03d" % i) if i % 5 else ("示例作者%03d" % i), 0, 0)
         for i in range(1, author_total + 1)],
    )

    chapter_count = max(0, args.chapters)
    per_book_chars = chapter_count * args.chars
    book_rows = []
    for i in range(1, args.books + 1):
        title = rand_text(pool, rng.choice((2, 3, 4, 5)), rng)
        if i <= 20:
            title = "长夜书简" + title  # 给书名 LIKE 压测一个稳定可命中的子串
        book_rows.append((
            i,
            title,
            0 if i % 5 == 0 else rng.randrange(1, author_total + 1),
            "/path/to/txt/sample-%06d.txt" % i,
            "sample-%06d.txt" % i,
            per_book_chars * 3,
            "%064x" % i,
            rng.choice(("gb18030", "utf-8", "utf-8-bom", "utf-16le")),
            per_book_chars,
            chapter_count,
            0,
            "",
            "imported",
            0,
        ))
    conn.executemany(
        "INSERT INTO books(id, title, author_id, source_path, source_name, file_size,"
        " file_sha256, encoding, word_count, chapter_count, volume_count, intro, status,"
        " multi_book) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
        book_rows,
    )

    if chapter_count > 0:
        insert_sql = (
            "INSERT INTO chapters(book_id, volume_id, idx, title, content, word_count)"
            " VALUES (?,?,?,?,?,?)"
        )
        if fts:
            insert_sql = (
                "INSERT INTO chapters(book_id, volume_id, idx, title, title_seg, content,"
                " content_seg, word_count) VALUES (?,?,?,?,?,?,?,?)"
            )
        batch = []
        total = args.books * chapter_count
        written = 0
        # 逐本章字数累加：章节长度有抖动（真实语料里章节长短差很多），
        # books.word_count 要自洽，否则 SUM(word_count) 与章节实际内容对不上。
        book_words = [0] * (args.books + 1)
        for b in range(1, args.books + 1):
            for c in range(1, chapter_count + 1):
                # 抖动 0.6x ~ 1.5x：让区间分桶不是退化成一个桶
                wc = max(50, int(args.chars * rng.uniform(0.6, 1.5)))
                body = rand_text(pool, wc, rng)
                ctitle = ("第%d章" % c) + rand_text(pool, rng.choice((2, 3, 4, 6)), rng)
                book_words[b] += wc
                if fts:
                    batch.append((b, 0, c, ctitle, segment(ctitle), body,
                                  segment(body), wc))
                else:
                    batch.append((b, 0, c, ctitle, body, wc))
                if len(batch) >= 2000:
                    conn.executemany(insert_sql, batch)
                    written += len(batch)
                    batch = []
                    if written % 100000 < 2000:
                        print("[建库] 章节 %d/%d" % (written, total), file=sys.stderr)
        if batch:
            conn.executemany(insert_sql, batch)
        conn.executemany(
            "UPDATE books SET word_count = ?, file_size = ? WHERE id = ?",
            [(book_words[b], book_words[b] * 3, b) for b in range(1, args.books + 1)],
        )
        conn.commit()
        if fts:
            print("[建库] 重建 FTS 索引（--fts on，耗时较长）", file=sys.stderr)
            conn.execute("INSERT INTO chapters_fts(chapters_fts) VALUES('rebuild')")

    conn.commit()
    conn.execute("PRAGMA optimize")
    conn.commit()
    build_secs = time.perf_counter() - t_build
    conn.close()
    return {"build_secs": build_secs}


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
) -> Dict[str, object]:
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
        "first": first_ms,
        "p50": statistics.median(times) if times else 0.0,
        "p95": pct(times, 95),
        "max": max(times) if times else 0.0,
        "samples": len(times),
        "summary": summary,
    }


# 与 internal/store/health.go 的 lengthBucketEdges 一致：章节长度分布的实际分桶边界。
BUCKET_EDGES = (0, 1000, 5000, 20000, 100000, 500000)


def bucket_range_sql(edges) -> str:
    """复刻 store.bucketRangeSQL：一条 SELECT 里 N 个区间 COUNT 子查询。

    应用侧用的是 (SELECT COUNT(*) ... WHERE col >= ? AND col < ?) 形态，
    每个子查询走 idx_chapters_wc 覆盖索引做区间 SEARCH。
    刻意不写成 SUM(CASE WHEN ...)：那是全索引扫描，慢一个量级，
    压测要压的是生产里真正跑的语句。
    """
    parts = []
    for i, lo in enumerate(edges):
        if i + 1 < len(edges):
            parts.append("(SELECT COUNT(*) FROM chapters WHERE word_count >= %d"
                         " AND word_count < %d)" % (lo, edges[i + 1]))
        else:
            parts.append("(SELECT COUNT(*) FROM chapters WHERE word_count >= %d)" % lo)
    return "SELECT " + ", ".join(parts)


def run_benchmarks(db_path: str, args) -> List[Dict[str, object]]:
    conn = sqlite3.connect(db_path)
    books = conn.execute("SELECT COUNT(*) FROM books").fetchone()[0]
    chapters = conn.execute("SELECT COUNT(*) FROM chapters").fetchone()[0]
    max_id = conn.execute("SELECT COALESCE(MAX(id),0) FROM chapters").fetchone()[0]
    rng = random.Random(args.seed + 1)

    # 随机取样用的 id 先一次性生成，随机数开销不计入查询耗时。
    # 长度必须 >= samples + 实际预热次数（部分查询把预热抬到 3 次），
    # 否则预热那几轮会越界；这里直接按最坏情况多取样。
    draws = max(1, args.samples) + max(3, args.warmup) + 1
    read_ids = [rng.randrange(1, max(max_id, 1) + 1) for _ in range(draws)]
    book_ids = [rng.randrange(1, max(books, 1) + 1) for _ in range(draws)]

    rows = []  # type: List[Dict[str, object]]
    s = args.samples
    w = args.warmup

    rows.append(bench_query(
        conn, "书总数 COUNT(*) FROM books", "SELECT COUNT(*) FROM books",
        lambda i: (), lambda r: "%d 本" % r[0][0], s, w))

    rows.append(bench_query(
        conn, "书架分页（word_count DESC, LIMIT 20 + OFFSET）",
        "SELECT id, title, word_count FROM books ORDER BY word_count DESC, id DESC"
        " LIMIT 20 OFFSET ?",
        lambda i: (book_ids[i] % max(1, books - 20),),
        lambda r: "%d 行" % len(r), s, w))

    rows.append(bench_query(
        conn, "书名 LIKE 全表（%长夜书简%）",
        "SELECT id, title FROM books WHERE title LIKE ?",
        lambda i: ("%长夜书简%",), lambda r: "%d 行" % len(r), s, w))

    rows.append(bench_query(
        conn, "书名 + 作者 LIKE（JOIN authors）",
        "SELECT b.id, b.title, a.name FROM books b LEFT JOIN authors a ON a.id = b.author_id"
        " WHERE b.title LIKE ? OR IFNULL(a.name,'') LIKE ?",
        lambda i: ("%长夜书简%", "%佚名01%"), lambda r: "%d 行" % len(r), s, w))

    if chapters > 0:
        rows.append(bench_query(
            conn, "章节总数 COUNT(*) FROM chapters", "SELECT COUNT(*) FROM chapters",
            lambda i: (), lambda r: "%d 章" % r[0][0], s, w))

        rows.append(bench_query(
            conn, "字数总和 SUM(word_count) FROM chapters",
            "SELECT SUM(word_count) FROM chapters",
            lambda i: (), lambda r: "%d 字" % r[0][0], s, w))

        rows.append(bench_query(
            conn, "随机读一章正文（按主键）",
            "SELECT id, title, content, word_count FROM chapters WHERE id = ?",
            lambda i: (read_ids[i],),
            lambda r: "%d 字节" % len(r[0][2]), s, max(w, 3)))

        rows.append(bench_query(
            conn, "单本目录（%d 章，ORDER BY idx）" % args.chapters,
            "SELECT id, idx, title, word_count FROM chapters WHERE book_id = ? ORDER BY idx",
            lambda i: (book_ids[i],),
            lambda r: "%d 行" % len(r), s, max(w, 3)))

        rows.append(bench_query(
            conn, "区间计数（%d 边界，复刻 bucketRangeSQL 子查询形态）" % len(BUCKET_EDGES),
            bucket_range_sql(BUCKET_EDGES),
            lambda i: (), lambda r: "%d 章" % sum(r[0]), s, w))

        rows.append(bench_query(
            conn, "按 (book_id, idx) 读一章正文",
            "SELECT id, content FROM chapters WHERE book_id = ? AND idx = ?",
            lambda i: (book_ids[i], (read_ids[i] % max(1, args.chapters)) + 1),
            lambda r: ("%d 字节" % len(r[0][1])) if r else "0 行", s, max(w, 3)))

    conn.close()
    return rows


def db_size_bytes(path: str) -> int:
    total = 0
    for suffix in ("", "-wal", "-shm"):
        p = path + suffix
        if os.path.exists(p):
            total += os.path.getsize(p)
    return total


def fmt_ms(v: float) -> str:
    if v >= 1000:
        return "%.3f s" % (v / 1000)
    return "%.3f" % v


def emit_markdown(args, build_info, results, db_path: str) -> None:
    size = db_size_bytes(db_path)
    mode = "仅元数据" if args.chapters <= 0 else ("%d 章/本" % args.chapters)
    out = []  # type: List[str]
    out.append("### 容量压测：%d 本 x %s（合成库）" % (args.books, mode))
    out.append("")
    out.append(
        "> 生成命令：%spython3 scripts/bench-capacity.py --books %d --chapters %d"
        " --chars %d --fts %s --samples %d%s（SQLite %s）"
        % (BQ, args.books, args.chapters, args.chars, args.fts, args.samples, BQ,
           sqlite3.sqlite_version)
    )
    out.append(">")
    out.append("> 合成数据，只含结构与量级，不含任何真实书名/作者/路径。")
    out.append("")
    out.append("| 指标 | 数值 |")
    out.append("|---|---|")
    out.append("| 书数 | %d |" % args.books)
    if args.chapters > 0:
        out.append("| 章节数 | %s |" % format(args.books * args.chapters, ","))
    out.append("| 正文索引（FTS5 + 分词镜像列） | %s |"
               % ("已建" if args.fts == "on" else "未建"))
    if args.chapters > 0:
        out.append("| 每章正文 | 约 %d 字 / %.1f KB |" % (args.chars, args.chars * 3 / 1024.0))
    out.append("| 库体积 | %s（%d 字节） |" % (human_size(size), size))
    out.append("| 每本平均 | %s |" % human_size(size / args.books))
    if args.chapters > 0:
        per_ch = size / (args.books * args.chapters)
        out.append("| 每章平均 | %.0f 字节 |" % per_ch)
        out.append("| 与真实语料对比 | 真实库每章约 %.1f KB，本库为 %d%% |"
                   % (REAL_BYTES_PER_CHAPTER / 1024.0, per_ch / REAL_BYTES_PER_CHAPTER * 100))
    out.append("| 建库耗时 | %.1f s |" % build_info["build_secs"])
    out.append("")
    out.append("| 操作 | 首查 (ms) | P50 (ms) | P95 (ms) | max (ms) | 采样 | 结果 |")
    out.append("|---|---|---|---|---|---|---|")
    for r in results:
        out.append("| %s | %s | %s | %s | %s | %d | %s |"
                   % (r["label"], fmt_ms(r["first"]), fmt_ms(r["p50"]),
                      fmt_ms(r["p95"]), fmt_ms(r["max"]), r["samples"], r["summary"]))
    out.append("")
    print(*out, sep=NL)


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="容量压测：合成 SQLite 库 + 核心查询基准，输出 markdown 表格",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="默认 500 本 x 500 章（小规模，make bench-capacity 用它）；"
               "几千万章必须显式传参，且会先做磁盘余量检查。",
    )
    p.add_argument("--books", type=int, default=500, help="书数（默认 500）")
    p.add_argument("--chapters", type=int, default=500,
                   help="每本章节数（默认 500；0 = 仅元数据）")
    p.add_argument("--chars", type=int, default=800,
                   help="每章正文汉字数（默认 800，约 2.3 KB）")
    p.add_argument("--fts", choices=("on", "off"), default="off",
                   help="是否建 FTS5 正文索引与分词镜像列（默认 off）")
    p.add_argument("--samples", type=int, default=40, help="每项采样次数（默认 40）")
    p.add_argument("--warmup", type=int, default=1, help="预热次数（默认 1）")
    p.add_argument("--dir", default=None, help="库所在目录（默认系统临时目录新建）")
    p.add_argument("--keep", action="store_true", help="跑完保留库文件（默认自动删除）")
    p.add_argument("--seed", type=int, default=20261025, help="随机种子（默认固定）")
    args = p.parse_args(argv)

    if args.books <= 0:
        p.error("--books 必须为正整数")
    if args.chapters < 0:
        p.error("--chapters 不能为负")
    if args.chars <= 0:
        p.error("--chars 必须为正")
    if args.samples < 1:
        p.error("--samples 至少为 1")
    return args


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    fts = args.fts == "on"

    workdir = args.dir or tempfile.mkdtemp(prefix="norvels-bench-")
    if not os.path.isdir(workdir):
        os.makedirs(workdir, exist_ok=True)

    need = estimate_bytes(args.books, args.chapters, args.chars, fts)
    check_disk(need, workdir, args)

    db_path = os.path.join(workdir, "capacity.db")
    print("[目标] %s" % db_path, file=sys.stderr)
    keep = args.keep
    try:
        build_info = create_library(db_path, args)
        print("[建库] 完成，耗时 %.1f s" % build_info["build_secs"], file=sys.stderr)
        results = run_benchmarks(db_path, args)
        emit_markdown(args, build_info, results, db_path)
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
