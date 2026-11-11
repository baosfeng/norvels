#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把决策模型的判定结果写回库，按批次可回滚。

两个用途，风险差很多，用之前先想清楚是哪种：
  · 补空缺（推荐）：书本来就没有分类（空 / 未分类 / 未分类/xxx），模型给出
    任何分类都是增益，不会丢已有信息——这是当前实际采用的用法，385 本未知
    分类取 confidence >= 0.7，写入 249 本。
  · 覆盖已有分类（谨慎）：拿模型去改已有分类。单模型的 precision 只有
    55~80%（400 本金标准实测），高置信度里仍有 15~25% 会改错，所以默认
    不做；确需覆盖时按目标类加白名单（只采纳模型 precision 高的那几类）。

写库纪律（与 classify-backfill.py 一致）：

写库纪律（与 classify-backfill.py 一致）：
  · 默认 dry-run，只打印会改多少与抽样；--apply 才真写；
  · 一个批次一个事务，中途失败整批不留痕；
  · 每条都记 enrich_changes（old_value = 原值），回滚用 --rollback <批次>；
  · 只改 confidence >= 阈值的书，低置信度一律不碰（宁可留原值）。

用法：
  scripts/classify-apply.py --db data/norvels.db --labels labels.jsonl --dry-run
  scripts/classify-apply.py --db data/norvels.db --labels labels.jsonl --apply --min-confidence 0.8
  scripts/classify-apply.py --db data/norvels.db --rollback 20261111-decision-xxxxxx
"""
import argparse
import json
import os
import sqlite3
import time

SOURCE_ID = "model:decision"  # 决策模型（Ollama clef / 百炼 preview）判定
FIELD = "category"


def decode(value):
    """库里的 bytes 列解码成 str（老批次可能带非 UTF-8）。"""
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


def load_labels(path):
    """读 run 产出的 JSONL（同 id 重跑取最后一条），返回 id -> (分类, 置信度)。"""
    latest = {}
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            line = line.strip()
            if not line:
                continue
            try:
                row = json.loads(line)
            except ValueError:
                continue
            if row.get("category"):
                latest[int(row["id"])] = (row["category"], float(row.get("confidence") or 0.0))
    return latest


def plan(conn, labels, min_confidence):
    """挑出要改的书：模型有判定、置信度过线、且与原一级分类不同。"""
    todo = []
    skipped_same = 0
    skipped_low = 0
    for book_id, (new_l1, conf) in sorted(labels.items()):
        if conf < min_confidence:
            skipped_low += 1
            continue
        row = conn.execute("SELECT category_l1 FROM books WHERE id=?", (book_id,)).fetchone()
        if row is None:
            continue
        old_l1 = decode(row[0]).strip()
        if old_l1 == new_l1:
            skipped_same += 1
            continue
        todo.append((book_id, old_l1, new_l1, conf))
    return todo, skipped_low, skipped_same


def main():
    parser = argparse.ArgumentParser(description="把决策模型判定写回库（可回滚）")
    parser.add_argument("--db", required=True, help="主库路径")
    parser.add_argument("--labels", default="", help="run 产出的 JSONL")
    parser.add_argument("--min-confidence", type=float, default=0.8,
                        help="只写不低于该置信度的判定（默认 0.8）")
    parser.add_argument("--apply", action="store_true", help="真写库；不加只 dry-run")
    parser.add_argument("--rollback", default="", help="回滚指定批次")
    parser.add_argument("--limit", type=int, default=0, help="最多改多少本（0 = 不限）")
    args = parser.parse_args()

    conn = sqlite3.connect(args.db, timeout=180)

    if args.rollback:
        batch = args.rollback
        rows = conn.execute(
            "SELECT book_id, old_value FROM enrich_changes "
            "WHERE batch_id=? AND field=?", (batch, FIELD)).fetchall()
        if not rows:
            raise SystemExit("批次 %s 没有记录" % batch)
        if not args.apply:
            print("dry-run：会回滚 %d 本（批次 %s）；加 --apply 才真写" % (len(rows), batch))
            return
        with conn:
            for book_id, old_value in rows:
                old_l1, _, old_l2 = (old_value or "").partition("/")
                conn.execute("UPDATE books SET category_l1=?, category_l2=? WHERE id=?",
                             (old_l1, old_l2, book_id))
            conn.execute("DELETE FROM enrich_changes WHERE batch_id=? AND field=?", (batch, FIELD))
        print("已回滚 %d 本（批次 %s）" % (len(rows), batch))
        return

    if not args.labels:
        raise SystemExit("需要 --labels（run 产出的 JSONL）")
    labels = load_labels(args.labels)
    todo, skipped_low, skipped_same = plan(conn, labels, args.min_confidence)
    if args.limit:
        todo = todo[:args.limit]

    print("判定 %d 本 · 置信度不足 %.2f 跳过 %d 本 · 与原分类相同跳过 %d 本"
          % (len(labels), args.min_confidence, skipped_low, skipped_same))
    print("待改 %d 本" % len(todo))
    for book_id, old_l1, new_l1, conf in todo[:10]:
        row = conn.execute("SELECT title FROM books WHERE id=?", (book_id,)).fetchone()
        print("  %-8d %-12s -> %-12s (%.2f)  %s"
              % (book_id, old_l1 or "(空)", new_l1, conf, decode(row[0] if row else "")[:40]))

    if not args.apply:
        print("\n这是 dry-run：以上改动没有写库；加 --apply 才真写（并生成可回滚批次）。")
        return

    batch = "%s-decision-%s" % (time.strftime("%Y%m%d"), time.strftime("%H%M%S"))
    with conn:
        for book_id, old_l1, new_l1, conf in todo:
            conn.execute(
                "INSERT INTO enrich_changes (batch_id, book_id, field, old_value, new_value, "
                "source_id, confidence) VALUES (?, ?, ?, ?, ?, ?, ?)",
                (batch, book_id, FIELD, old_l1, new_l1, SOURCE_ID, conf))
            conn.execute("UPDATE books SET category_l1=?, category_l2='' WHERE id=?",
                         (new_l1, book_id))
    print("已写 %d 本 · 批次 %s（回滚用 --rollback %s --apply）" % (len(todo), batch, batch))


if __name__ == "__main__":
    main()
