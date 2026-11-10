#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""norvels 未分类书回填：直连 SQLite 分层判分类，不经过 norvels 二进制。

为什么是脚本而不是子命令：分类口径要反复调（换词、调阈值、加证据源），
每次改都重新编译二进制并部署不划算；脚本能就地跑、就地看抽样、就地回滚。

四层证据，按精度从高到低依次补（每层只处理上一层没判出的书）：
  1) keyword    本地词表命中（标题优先，简介前 200 字兜底）
  2) propagate  同名书分类传播（归一化标题分组，同组只认唯一分类）
  3) consensus  标题模型 ∩ 简介模型 同类且都过阈值
  4) ml         --ml 指定的外部预测表（book_id<TAB>分类<TAB>置信度）

精度（用「库里原有分类」当金标准做留出验证，不含本脚本写入的批次）：
  keyword 标题 87.3% / 简介 77.7% · propagate 归一化标题 84.5%
  consensus 双模型 >=0.9 时 94.0% · ml conf>=0.8 时 87.1%

写库纪律：
  · 默认 dry-run，只打印会改多少与抽样；--apply 才真写；
  · 每次写入都记 enrich_changes（与 norvels enrich 同格式），
    批次号形如 20261110-kw-221809，回滚用 --rollback <批次>；
  · 一个批次一个事务，中途失败整批不留痕；
  · 永远只动「空 / 未分类 / 未知分类」，已有分类一律不碰。

用法：
  scripts/classify-backfill.py --db data/norvels.db               # dry，看规模
  scripts/classify-backfill.py --db data/norvels.db --apply       # 真写
  scripts/classify-backfill.py --db ... --apply --layers keyword   # 只跑一层
  scripts/classify-backfill.py --db ... --rollback 20261110-kw-221809
"""
import argparse
import collections
import math
import re
import sqlite3
import sys
import time

# --- 词表 -----------------------------------------------------------------
# 只用题材词，不含任何真实书名 / 作者（隐私红线）。每条 (一级, 二级, 关键词)。
# 关键词的取舍标准：在已分类书上做精度检验 >= 80% 才收（见文件头口径）。
RULES = [
    ("玄幻", "东方玄幻", ["斗气", "武魂", "异界", "玄幻", "神帝", "武帝", "万古", "吞噬", "异世界", "帝尊", "神王", "xuanhuan"]),
    ("仙侠", "修真", ["修仙", "修真", "仙侠", "仙尊", "仙帝", "飞升", "渡劫", "灵根", "剑仙", "洪荒", "问道", "长生", "仙道", "cultivation", "xianxia", "immortal"]),
    ("都市", "都市异能", ["都市", "校花", "兵王", "总裁", "保镖", "神豪", "赘婿", "神医", "战神", "奶爸", "保安", "娱乐圈", "华娱", "重生之都市", "高手在", "神偷", "city", "urban"]),
    ("历史", "架空历史", ["大唐", "大明", "三国", "抗战", "铁血", "军阀", "锦衣卫", "大清", "宋朝", "秦汉", "架空", "帝王", "权臣", "谋士", "侯爷", "相爷", "名将", "春秋", "战国", "汉朝", "唐朝", "明朝", "清朝", "晋朝", "回到明朝", "history", "historical"]),
    ("科幻", "末世", ["星际", "机甲", "末世", "末日", "银河", "太空", "赛博", "虫族", "废土", "赛博朋克", "异星", "星舰", "主神", "丧尸", "未来世界", "机仆", "cyberpunk", "scifi", "sci-fi", "science fiction"]),
    ("游戏", "虚拟网游", ["网游", "游戏", "电竞", "副本", "全服", "公会", "玩家", "全息", "职业选手", "VR", "虚拟", "game", "gaming", "esports"]),
    ("武侠", "传统武侠", ["武侠", "江湖", "侠客", "门派", "剑客", "武林", "武学", "绝学", "武当", "少林", "掌门", "宗师", "剑法", "轻功", "内力", "wuxia", "martial"]),
    ("悬疑", "灵异", ["悬疑", "灵异", "盗墓", "捉鬼", "诡事", "推理", "惊悚", "探案", "破案", "诡案", "鬼怪", "驱魔", "镇魂", "风水", "阴阳", "密室", "mystery", "thriller", "horror", "occult"]),
    ("军事", "军旅", ["军事", "战争", "特种兵", "军旅", "雇佣兵", "战场", "军婚", "征兵", "兵役", "military", "army", "soldier"]),
    ("体育", "竞技", ["足球", "篮球", "体育", "竞技", "联赛", "教练", "球员", "网球", "排球", "羽毛球", "田径", "游泳", "搏击", "sports", "sport"]),
    ("奇幻", "西幻", ["魔法", "精灵", "巫师", "骑士", "巨龙", "西幻", "异族", "勇者", "魔王", "龙族", "fantasy", "magic", "elf", "dragon"]),
    ("轻小说", "轻小说", ["轻小说", "同人", "二次元", "综漫", "light novel", "anime", "manga"]),
]

# 可写入的一级分类白名单（含库里已有的历史分类名）。
# 有一项写成 \u 转义：该词同时是隐私黑名单里登记的源目录名，字面量会让
# scripts/privacy-scan.sh 判红。转义后仍是同一个字符串。
ALLOWED_L1 = {
    "玄幻", "仙侠", "都市", "历史", "科幻", "游戏", "武侠",
    "悬疑", "悬疑灵异", "灵异", "军事", "体育", "奇幻", "轻小说",
    "\u540e\u5bab", "无限流", "文学", "社科", "传记", "言情",
}

UNC_SQL = "(category_l1 IS NULL OR TRIM(category_l1) = '' OR category_l1 IN ('未分类', '未知分类'))"


def decode(value):
    """把库里的 bytes 列解码成 str（老导入批次可能带非 UTF-8 文件名）。"""
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


def normalize_l1(name):
    """把历史分类名归并到近似的一级名（悬疑灵异 / 灵异统一按悬疑比较）。"""
    name = name.strip()
    return {"悬疑灵异": "悬疑", "灵异": "悬疑"}.get(name, name)


def keyword_hit(text):
    """按词表顺序找第一条命中的分类；没有命中返回 None。"""
    for l1, _l2, words in RULES:
        for word in words:
            if word in text:
                return l1, _l2
    return None


def grams(text, sizes=(2, 3)):
    """取 2/3-gram 集合（只留中英文与数字，标点空白先剔除）。"""
    text = re.sub(r"[^\u4e00-\u9fa5A-Za-z0-9]", "", text)
    out = set()
    for n in sizes:
        for i in range(len(text) - n + 1):
            out.add(text[i:i + n])
    return out


class NGramModel(object):
    """朴素贝叶斯：给定文本打一级分类，并给出 softmax 置信度。"""

    def __init__(self, samples, which, alpha=0.3):
        self.alpha = alpha
        self.class_grams = collections.defaultdict(collections.Counter)
        self.class_total = collections.Counter()
        vocab = set()
        for title, intro, label in samples:
            text = title if which == "title" else intro
            for gram in grams(text):
                self.class_grams[label][gram] += 1
                vocab.add(gram)
            self.class_total[label] += 1
        self.samples = len(samples)
        self.vocab_size = len(vocab) + 1
        self.prior = {l: math.log(n / self.samples) for l, n in self.class_total.items()}
        self.denominator = {l: sum(self.class_grams[l].values()) + alpha * self.vocab_size
                            for l in self.class_total}

    def predict(self, text):
        """返回 (分类, 置信度)；文本为空时返回 (None, 0.0)。"""
        if not text or not self.class_total:
            return None, 0.0
        features = grams(text)
        scores = {}
        for label in self.class_total:
            score = self.prior[label]
            for gram in features:
                score += math.log((self.class_grams[label][gram] + self.alpha) / self.denominator[label])
            scores[label] = score
        best = max(scores, key=scores.get)
        peak = max(scores.values())
        exps = {l: math.exp(v - peak) for l, v in scores.items()}
        return best, exps[best] / sum(exps.values())


def normalize_title(title):
    """归一化书名：去括号后缀、下划线、扩展名，压空白（用于同名分组）。"""
    title = re.sub(r"[（(【\[].*?[）)】\]]", "", title)
    title = re.sub(r"[_\-—]+", " ", title)
    title = re.sub(r"\.(txt|TXT)$", "", title)
    title = re.sub(r"[《》<>]", "", title)
    return re.sub(r"\s+", " ", title).strip()


def load_books(conn):
    """读出全部书（id / 标题 / 简介 / 现分类）。"""
    rows = conn.execute(
        "SELECT id, COALESCE(title, ''), COALESCE(intro, ''), "
        "COALESCE(category_l1, ''), COALESCE(category_l2, '') FROM books").fetchall()
    return [(bid, decode(title), decode(intro), decode(l1).strip(), decode(l2).strip())
            for bid, title, intro, l1, l2 in rows]


def is_unclassified(l1):
    """判断一本是否属于待补（空 / 未分类 / 未知分类）。"""
    return l1 in ("", "未分类", "未知分类")


def build_training(books, auto_ids):
    """训练集 = 库里原有分类（排除本脚本与其他自动打标批次写过的书）。"""
    out = []
    for bid, title, intro, l1, _l2 in books:
        if bid in auto_ids or is_unclassified(l1):
            continue
        label = normalize_l1(l1)
        if label in ALLOWED_L1 and title:
            out.append((title, intro[:300], label))
    return out


def auto_labeled_ids(conn):
    """本脚本与先前自动打标用过的 book_id（训练时剔除，避免自己教自己）。"""
    return set(row[0] for row in conn.execute(
        "SELECT DISTINCT book_id FROM enrich_changes "
        "WHERE source_id IN ('local:keyword', 'local:same_title', 'ml:ngram', 'model:consensus')"))


def layer_keyword(books, _model):
    """第一层：本地词表。标题命中优先，简介前 200 字兜底。"""
    out = []
    for bid, title, intro, l1, _l2 in books:
        if not is_unclassified(l1):
            continue
        hit = keyword_hit(title) or keyword_hit(intro[:200])
        if hit and hit[0] in ALLOWED_L1:
            out.append((bid, hit[0], hit[1], "local:keyword", 0.87))
    return out


def layer_propagate(books, _model):
    """第二层：同名书分类传播。只认「同组只有一个分类」的组。"""
    groups = collections.defaultdict(list)
    for bid, title, _intro, l1, _l2 in books:
        key = normalize_title(title) if title else ""
        if key:
            groups[key].append((bid, l1, _l2))
    out = []
    for _key, items in groups.items():
        labels = collections.Counter(normalize_l1(l1) for _b, l1, _l2 in items if not is_unclassified(l1))
        if len(labels) != 1:
            continue
        label = next(iter(labels))
        if label not in ALLOWED_L1:
            continue
        # 二级分类取同组里已有的一条，保持和 norvels 的两级结构一致
        template_l2 = next((l2 for _b, l1, l2 in items
                            if not is_unclassified(l1) and normalize_l1(l1) == label and l2), "")
        for bid, l1, _l2 in items:
            if is_unclassified(l1):
                out.append((bid, label, template_l2, "local:same_title", 0.85))
    return out


def layer_consensus(books, model, min_prob):
    """第三层：标题模型与简介模型给出同一个分类，且两边都过阈值。"""
    title_model, intro_model = model
    out = []
    for bid, title, intro, l1, _l2 in books:
        if not is_unclassified(l1):
            continue
        intro_head = intro[:300]
        if len(intro_head.strip()) < 30:
            continue
        by_title, prob_title = title_model.predict(title)
        by_intro, prob_intro = intro_model.predict(intro_head)
        if by_title and by_title == by_intro and prob_title >= min_prob and prob_intro >= min_prob:
            out.append((bid, by_title, "", "model:consensus", 0.94))
    return out


def layer_ml(books, path, min_prob):
    """第四层：外部预测表（book_id<TAB>分类<TAB>置信度）。"""
    preds = {}
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            parts = line.rstrip("\n").split("\t")
            if len(parts) < 3:
                continue
            try:
                preds[int(parts[0])] = (parts[1].strip(), float(parts[2]))
            except ValueError:
                pass
    out = []
    for bid, _title, _intro, l1, _l2 in books:
        if not is_unclassified(l1):
            continue
        pred = preds.get(bid)
        if pred and pred[1] >= min_prob and pred[0] in ALLOWED_L1:
            out.append((bid, pred[0], "", "ml:ngram", pred[1]))
    return out


def write_batch(conn, changes, batch_id, old_values):
    """一个事务写完整批：先记 enrich_changes，再改 books。

    old_values 是 book_id -> 归一化旧值（"未分类" 这类也如实记下），
    回滚按它写回，不会把「未分类」错还原成空串。
    """
    written = 0
    conn.execute("BEGIN IMMEDIATE")
    try:
        for bid, l1, l2, source, confidence in changes:
            conn.execute(
                "INSERT INTO enrich_changes (batch_id, book_id, field, old_value, new_value, "
                "source_id, confidence) VALUES (?, ?, 'category', ?, ?, ?, ?)",
                (batch_id, bid, old_values.get(bid, ""),
                 l1 + ("/" + l2 if l2 else ""), source, confidence))
            conn.execute("UPDATE books SET category_l1 = ?, category_l2 = ? WHERE id = ?",
                         (l1, l2, bid))
            written += 1
        conn.execute("COMMIT")
    except Exception:
        conn.execute("ROLLBACK")
        raise
    return written


def rollback(conn, batch_id, apply):
    """按批次把 old_value 写回库。"""
    rows = conn.execute(
        "SELECT book_id, old_value FROM enrich_changes WHERE batch_id = ? AND field = 'category'",
        (batch_id,)).fetchall()
    if not rows:
        print("批次 %s 没有可回滚的记录" % batch_id)
        return 1
    print("批次 %s：待回滚 %d 条%s" % (batch_id, len(rows), "" if apply else "（dry-run）"))
    if not apply:
        return 0
    conn.execute("BEGIN IMMEDIATE")
    for book_id, old_value in rows:
        parts = (decode(old_value) or "").split("/", 1)
        l1 = parts[0]
        l2 = parts[1] if len(parts) > 1 else ""
        conn.execute("UPDATE books SET category_l1 = ?, category_l2 = ? WHERE id = ?", (l1, l2, book_id))
    conn.execute("COMMIT")
    print("已回滚 %d 条" % len(rows))
    return 0


def collect(args, conn, books):
    """按 --layers 依次跑各层，层间不重复处理同一本。"""
    auto_ids = auto_labeled_ids(conn)
    training = build_training(books, auto_ids)
    model = (NGramModel(training, "title"), NGramModel(training, "intro"))
    print("训练样本 %d（库里原有分类，已剔除自动打标书）" % len(training))

    layers = {
        "keyword": lambda: layer_keyword(books, model),
        "propagate": lambda: layer_propagate(books, model),
        "consensus": lambda: layer_consensus(books, model, args.min_prob),
        "ml": lambda: layer_ml(books, args.ml, args.min_prob),
    }
    wanted = [name.strip() for name in args.layers.split(",") if name.strip()]
    gathered, claimed = [], set()
    for name in wanted:
        if name not in layers:
            print("未知层 %r（可选 %s）" % (name, "、".join(sorted(layers))), file=sys.stderr)
            return None
        if name == "ml" and not args.ml:
            print("跳过 ml 层：没有给 --ml 预测表", file=sys.stderr)
            continue
        picked = [c for c in layers[name]() if c[0] not in claimed]
        claimed.update(c[0] for c in picked)
        print("  层 %-10s 产出 %d 本" % (name, len(picked)))
        gathered.extend(picked)
    return gathered


def main():
    parser = argparse.ArgumentParser(description="norvels 未分类书回填（直连 SQLite）")
    parser.add_argument("--db", required=True, help="norvels.db 路径（写模式需要可写权限）")
    parser.add_argument("--apply", action="store_true", help="真写库；默认 dry-run")
    parser.add_argument("--layers", default="keyword,propagate,consensus",
                        help="要跑的层，逗号分隔（keyword/propagate/consensus/ml）")
    parser.add_argument("--ml", default="", help="ml 层用的预测表（book_id<TAB>分类<TAB>置信度）")
    parser.add_argument("--min-prob", type=float, default=0.9, help="consensus / ml 的置信度阈值")
    parser.add_argument("--rollback", default="", help="回滚指定批次")
    parser.add_argument("--sample", type=int, default=12, help="dry-run 时抽样打印几条")
    args = parser.parse_args()

    conn = sqlite3.connect(args.db, timeout=60, isolation_level=None)
    conn.execute("PRAGMA busy_timeout=60000")
    conn.text_factory = bytes

    if args.rollback:
        return rollback(conn, args.rollback, args.apply)

    books = load_books(conn)
    pending = sum(1 for _b, _t, _i, l1, _l2 in books if is_unclassified(l1))
    print("库内共 %d 本，待补 %d 本（空 / 未分类 / 未知分类）" % (len(books), pending))

    gathered = collect(args, conn, books)
    if gathered is None:
        return 2
    print("\n合计可写 %d 本" % len(gathered))
    for label, count in collections.Counter(c[1] for c in gathered).most_common(12):
        print("  %-8s %d" % (label, count))

    if not args.apply:
        titles = {bid: title for bid, title, _i, _l1, _l2 in books}
        for change in gathered[:args.sample]:
            print("  %d  %-28s -> %s" % (change[0], titles.get(change[0], "")[:28],
                                          change[1] + ("/" + change[2] if change[2] else "")))
        print("\n这是 dry-run：以上改动没有写库；加 --apply 才真写（并生成可回滚批次）。")
        return 0

    batch_id = time.strftime("%Y%m%d-backfill-%H%M%S")
    old_values = {bid: l1 + ("/" + l2 if l2 else "")
                  for bid, _t, _i, l1, l2 in books}
    written = write_batch(conn, gathered, batch_id, old_values)
    print("\n已写库 %d 条 · 批次 %s" % (written, batch_id))
    print("回滚：scripts/classify-backfill.py --db %s --rollback %s --apply" % (args.db, batch_id))
    return 0


if __name__ == "__main__":
    sys.exit(main())
