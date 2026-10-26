#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""norvels 待整理合集包整理器。

做四件事：
  1) 扫描 --src 下的可导入文件（txt/epub/zip）；
  2) 与库内书名比对，已在库的移到 --dup-dir（保留相对路径，可恢复）；
  3) 同一批内同名书只保留最大的一份，其余移到 --dup-dir；
  4) 其余按目录/关键词推断分类，复制到 --staging/<大分类>/<小分类>/<文件名>。

导入不在这里做：跑完用 POST /api/import {"dir":"<容器内 staging>"}。
默认 --dry-run；加 --apply 才动文件。
"""
import argparse
import collections
import hashlib
import os
import re
import shutil
import sqlite3
import sys
import unicodedata
from concurrent.futures import ThreadPoolExecutor, as_completed

IMPORTABLE = {'.txt', '.epub', '.zip'}
MAX_ZIP = 512 * 1024 * 1024

# 目录名自带分类时优先用目录（400+本高质量完本合集/01、玄幻/...）
DIR_ALIAS = {
    '玄幻': '玄幻', '仙侠': '仙侠', '都市': '都市', '科幻': '科幻',
    '奇幻': '奇幻', '历史': '历史', '武侠': '武侠', '游戏': '游戏',
    '高武': '玄幻', '灵异': '悬疑灵异', '悬疑': '悬疑灵异',
    '军事': '军事', '轻小说': '轻小说', '体育': '体育',
}

# 文件名里出现这些词才归类，否则算「未分类」
KEYWORDS = [
    ('仙侠', ('修仙', '修真', '仙侠', '仙尊', '仙帝', '飞升', '渡劫', '灵根', '剑仙')),
    ('玄幻', ('玄幻', '异界', '斗气', '武魂', '斗破', '斗罗', '神帝', '武帝', '万古', '吞噬星空', '圣域')),
    ('都市', ('都市', '校花', '兵王', '总裁', '娱乐明星', '重生之都市', '保镖')),
    ('历史', ('大唐', '大明', '三国', '抗战', '铁血', '军阀', '锦衣卫', '回到明朝', '大清', '宋朝')),
    ('科幻', ('星际', '机甲', '末世', '末日', '银河', '太空', '赛博', '虫族', '未来世界')),
    ('游戏', ('网游', '游戏', '电竞', '副本', '全服', '公会')),
    ('武侠', ('武侠', '江湖', '侠客', '门派')),
    ('悬疑灵异', ('悬疑', '灵异', '盗墓', '捉鬼', '诡事', '推理')),
    ('军事', ('军事', '战争', '特种兵', '军旅', '雇佣兵')),
    ('体育', ('足球', '篮球', '体育')),
    ('奇幻', ('魔法', '精灵', '巫师', '骑士')),
    ('轻小说', ('轻小说',)),
]


def norm(text):
    text = unicodedata.normalize('NFKC', text or '')
    return re.sub(r'\s+', '', text).lower()


def parse_title(filename):
    base = os.path.splitext(filename)[0]
    m = re.search(r'《(.+?)》', base)
    if m:
        return m.group(1)
    return base


def category_for(rel_parts, filename):
    """先看目录，再看文件名关键词；都不中返回 ('未分类', '')。"""
    for part in rel_parts:
        m = re.match(r'^\d+[、.．]\s*(.+)$', part)
        name = m.group(1) if m else part
        if name in DIR_ALIAS:
            return DIR_ALIAS[name], ''
    stem = os.path.splitext(filename)[0]
    for cat, words in KEYWORDS:
        if any(w in stem for w in words):
            return cat, ''
    return '未分类', ''


def sha256_of(path):
    h = hashlib.sha256()
    with open(path, 'rb') as fh:
        for chunk in iter(lambda: fh.read(4 * 1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--src', required=True)
    ap.add_argument('--db', required=True)
    ap.add_argument('--staging', required=True)
    ap.add_argument('--dup-dir', required=True)
    ap.add_argument('--report', default='/tmp/tidy_pending_report.tsv')
    ap.add_argument('--workers', type=int, default=8)
    ap.add_argument('--apply', action='store_true')
    args = ap.parse_args()

    con = sqlite3.connect('file:%s?mode=ro&immutable=1' % args.db, uri=True)
    lib_titles = {norm(t) for (t,) in con.execute('select title from books')}
    lib_sha = {s for (s,) in con.execute('select file_sha256 from books')}
    print('library: %d titles, %d sha' % (len(lib_titles), len(lib_sha)))

    entries = []
    for root, _dirs, files in os.walk(args.src):
        for fn in files:
            ext = os.path.splitext(fn)[1].lower()
            path = os.path.join(root, fn)
            rel = os.path.relpath(path, args.src)
            try:
                size = os.path.getsize(path)
            except OSError as exc:
                entries.append((path, rel, -1, 'skip_unreadable', '', ''))
                print('unreadable: %s (%s)' % (rel, exc), file=sys.stderr)
                continue
            entries.append((path, rel, size, '', '', ''))
    print('scanned %d files' % len(entries))

    # 分类：跳过 / 重复 / 导入
    plan = []
    title_groups = collections.defaultdict(list)
    for path, rel, size, _a, _b, _c in entries:
        fn = os.path.basename(path)
        ext = os.path.splitext(fn)[1].lower()
        if ext not in IMPORTABLE:
            plan.append((path, rel, size, 'skip_unsupported', '', ''))
            continue
        if size < 0:
            continue
        if ext == '.zip' and size > MAX_ZIP:
            plan.append((path, rel, size, 'skip_big_zip', '', ''))
            continue
        title = parse_title(fn)
        if norm(title) in lib_titles:
            plan.append((path, rel, size, 'move_dup_library', '', ''))
            continue
        title_groups[norm(title)].append((path, rel, size, title))

    for key, group in title_groups.items():
        group.sort(key=lambda x: -x[2])
        keep = group[0]
        l1, l2 = category_for(keep[1].split(os.sep)[:-1], os.path.basename(keep[0]))
        plan.append((keep[0], keep[1], keep[2], 'import', l1, l2))
        for item in group[1:]:
            plan.append((item[0], item[1], item[2], 'move_dup_batch', '', ''))

    stats = collections.Counter()
    bytes_by = collections.Counter()
    for _p, _r, size, action, l1, _l2 in plan:
        stats[action] += 1
        bytes_by[action] += max(size, 0)
    print('\n=== plan ===')
    for action, count in stats.most_common():
        print('  %-20s %6d files  %7.1f GB' % (action, count, bytes_by[action] / 1024 ** 3))
    cats = collections.Counter(l1 for *_, action, l1, _l2 in plan if action == 'import')
    print('\n=== categories to import ===')
    for cat, count in cats.most_common():
        print('  %-10s %6d' % (cat, count))

    with open(args.report, 'w', encoding='utf-8') as fh:
        fh.write('action\tl1\tl2\tsize\tpath\n')
        for path, rel, size, action, l1, l2 in plan:
            fh.write('%s\t%s\t%s\t%d\t%s\n' % (action, l1, l2, size, rel))
    print('\nreport: %s' % args.report)

    if not args.apply:
        print('\ndry-run: nothing changed. rerun with --apply')
        return

    # 1) 重复文件移到 dup-dir
    moved = 0
    for path, rel, _size, action, _l1, _l2 in plan:
        if not action.startswith('move_dup'):
            continue
        dst = os.path.join(args.dup_dir, rel)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.move(path, dst)
        moved += 1
        if moved % 200 == 0:
            print('moved dups: %d' % moved)
    print('moved dups: %d' % moved)

    # 2) 并发复制到 staging（边抓取边按分类落盘）
    todo = [(path, rel, l1, l2) for path, rel, _s, action, l1, l2 in plan if action == 'import']
    os.makedirs(args.staging, exist_ok=True)
    done = 0
    def copy_one(item):
        path, rel, l1, l2 = item
        sub = os.path.join(args.staging, l1, l2) if l2 else os.path.join(args.staging, l1)
        os.makedirs(sub, exist_ok=True)
        dst = os.path.join(sub, os.path.basename(path))
        base, ext = os.path.splitext(dst)
        n = 2
        while os.path.exists(dst):
            dst = '%s (%d)%s' % (base, n, ext)
            n += 1
        shutil.copyfile(path, dst)
        return dst
    with ThreadPoolExecutor(max_workers=args.workers) as pool:
        futures = [pool.submit(copy_one, item) for item in todo]
        for fut in as_completed(futures):
            fut.result()
            done += 1
            if done % 200 == 0:
                print('copied: %d / %d' % (done, len(todo)))
    print('copied: %d' % done)


if __name__ == '__main__':
    main()
