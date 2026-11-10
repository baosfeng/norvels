#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""给待分类书导出一份「判断证据包」：书名 / 作者 / 简介 / 首章与中间章摘录。

为什么需要这一层：约半数未分类书库里没有简介，只靠书名判断，
连最强的模型也只能到 60% 出头。首章正文里的题材信号（穿越设定、
境界体系、职业设定）比书名可靠得多，所以把正文摘录一并给出。

正文从 HTTP API 读（content 库是 zstd 打包块，Python 侧没有解压库，
而服务本身就能解）：服务返回的 content 已是明文，摘前 400 字即可。

用法：
  scripts/classify-evidence.py --db data/norvels.db --out pilot.json --limit 300
  scripts/classify-evidence.py --db ... --out b2.json --offset 300 --limit 100
"""
import argparse
import json
import os
import sqlite3
import sys
import urllib.request

API = os.environ.get("NORVELS_API", "http://127.0.0.1:8080/api")


def load_token():
    """取鉴权 token：优先环境变量，其次从 --env 指定的 .env 读（取等号后第一段）。

    .env 里的值可能带行尾注释（「# 这里显式接受…」），直接 split("=")[1] 会把
    整句注释当 token 发出去，服务端不认，正文摘录就静默全空。
    """
    token = os.environ.get("NORVELS_AUTH_TOKEN", "").strip()
    if token:
        return token
    path = os.environ.get("NORVELS_ENV", "")
    if path and os.path.exists(path):
        with open(path, encoding="utf-8") as handle:
            for line in handle:
                if line.startswith("NORVELS_AUTH_TOKEN="):
                    return line.split("=", 1)[1].split("#")[0].strip()
    return ""


TOKEN = load_token()


def api_get(path, timeout=15):
    req = urllib.request.Request(API + path, headers={"Authorization": "Bearer " + TOKEN})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode() or "{}")


def decode(value):
    """库里的 bytes 列解码成 str。"""
    if isinstance(value, str):
        return value
    if not value:
        return ""
    for encoding in ("utf-8", "gb18030"):
        try:
            return value.decode(encoding)
        except UnicodeDecodeError:
            pass
    return value.decode("utf-8", "replace")


def clean_text(text, limit):
    """压掉广告行与空行，取前 limit 字（正文前几行常是站点广告）。"""
    lines = [line.strip() for line in (text or "").splitlines() if line.strip()]
    body = "\n".join(lines)
    return body[:limit]


def chapter_excerpts(book_id, want=2):
    """取首章与中间章的正文摘录；任何一步失败都返回空，不阻塞整批。"""
    try:
        chapters = api_get("/books/%d/chapters" % book_id).get("items") or []
    except Exception:
        return []
    if not chapters:
        return []
    picks = [chapters[0]]
    if len(chapters) > 3:
        picks.append(chapters[len(chapters) // 2])
    out = []
    for ch in picks:
        try:
            body = api_get("/books/%d/chapters/%d" % (book_id, ch["id"]))
        except Exception:
            continue
        excerpt = clean_text(body.get("content"), 400)
        if excerpt:
            out.append(excerpt)
    return out


def main():
    parser = argparse.ArgumentParser(description="导出待分类书的判断证据包")
    parser.add_argument("--db", required=True, help="norvels.db 路径")
    parser.add_argument("--out", required=True, help="输出 JSON 路径")
    parser.add_argument("--limit", type=int, default=300, help="本批多少本")
    parser.add_argument("--offset", type=int, default=0, help="跳过前多少本（分批用）")
    parser.add_argument("--sort", default="word_count", help="排序键（word_count / id）")
    parser.add_argument("--no-chapter", action="store_true", help="不取正文摘录（只导书名与简介）")
    args = parser.parse_args()

    conn = sqlite3.connect(args.db, timeout=30)
    conn.text_factory = bytes
    rows = conn.execute(
        "SELECT b.id, COALESCE(b.title, ''), COALESCE(b.intro, ''), COALESCE(a.name, '') "
        "FROM books b LEFT JOIN authors a ON a.id = b.author_id "
        "WHERE (b.category_l1 IS NULL OR TRIM(b.category_l1) = '' "
        "  OR b.category_l1 IN ('未分类', '未知分类')) "
        "ORDER BY b.%s DESC, b.id ASC LIMIT ? OFFSET ?" % args.sort,
        (args.limit, args.offset)).fetchall()

    out = []
    for book_id, title, intro, author in rows:
        item = {
            "id": book_id,
            "title": decode(title)[:60],
            "author": decode(author).strip()[:20],
            "intro": clean_text(decode(intro), 300),
        }
        if not args.no_chapter:
            item["excerpts"] = chapter_excerpts(book_id)
        out.append(item)
        if len(out) % 50 == 0:
            print("  已导出 %d 本" % len(out), file=sys.stderr)

    with open(args.out, "w", encoding="utf-8") as handle:
        json.dump(out, handle, ensure_ascii=False, indent=1)
    filled = sum(1 for x in out if x.get("excerpts"))
    print("导出 %d 本到 %s（含正文摘录 %d 本）" % (len(out), args.out, filled))


if __name__ == "__main__":
    main()
