#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""用决策模型判题材分类：一次前向传播直接返回分类与各选项概率。

为什么分类走决策模型，而不是让对话模型自由生成：
  分类的本质是「从固定选项里挑一个」——不需要写理由、不需要调工具、不需要
  逐 token 生成再由解析器兜底。决策模型（Ollama System One · clef）把每个
  选项的打分放进同一次非自回归前向里，直接返回 choice + probabilities +
  confidence，延迟百毫秒级；云端按输入 token 计费，没有输出 token 成本。

接口（https://docs.ollama.com/api/systemone）：
  POST https://ollama.com/v1/systemone      Authorization: Bearer <key>
  POST http://127.0.0.1:11434/v1/systemone  本地模型，不需要 key
请求体 {model, state, questions}：state 放书名/作者/简介/正文摘录；
questions 支持一次带多个问题（云端上限 64 个），每题独立打分、答案不串给下一题。
问题集用 --preset 选内置的（genre 只判题材 / full 另带受众、是否网文、质量分、
有无广告），或用 --questions 传自己的 JSON。实测 5 问一次只多 36% 输入 token，
比同一本书问 5 次省 73%，延迟基本不变——批量提问题比逐个问划算得多。

职责边界：本脚本只做判断，不碰数据库。写库仍由
scripts/classify-backfill.py --agents 按批次写（可回滚）；
证据包由 scripts/classify-evidence.py 导出。

用法：
  # 1) 金标准评测：库里未被自动打标的原始分类当标准答案
  scripts/classify-decision.py eval --gold gold.json --report eval-report.json

  # 2) 批量判类：证据包 -> labels.jsonl（可断点续跑，逐行落盘）
  scripts/classify-decision.py run --evidence 'ev/*.json' --out labels.jsonl

  # 2b) 一次问多个问题：题材 + 受众 + 是否网文 + 质量分 + 有无广告
  scripts/classify-decision.py run --evidence 'ev/*.json' --out labels.jsonl --preset full

  # 2c) 用自己写的问题集（JSON：{问题名: {type, instructions, criteria}}）
  scripts/classify-decision.py run --evidence 'ev/*.json' --out labels.jsonl \
      --questions my-questions.json

  # 3) 导出 backfill 能直接吃的数组（低置信度的先不写库）
  scripts/classify-decision.py export --labels labels.jsonl --out labels.json \\
      --min-confidence 0.7

  # 4) 换模型 / 走本地
  scripts/classify-decision.py run ... --model clef-flash \\
      --endpoint http://127.0.0.1:11434/v1/systemone

凭据：只从环境变量 OLLAMA_DECISION_KEY 读，不落盘、不打印、不进日志。
macOS 上它通常写在 ~/.zshrc，非交互 shell 读不到，所以这样跑：
  zsh -lc 'scripts/classify-decision.py eval --gold gold.json'
"""
import argparse
import glob
import json
import os
import sys
import threading
import time
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor

# 两套后端，协议都是 TypeSafe System One，只是域名、模型名、凭据与上限不同。
#   ollama  云端 clef / clef-flash：一次最多 64 个问题，上下文 64K
#   aliyun  decision-model-preview：一次最多 16 个问题，上下文 64K，华北2/新加坡限时免费
BACKENDS = {
    "ollama": {
        "endpoint": os.environ.get("OLLAMA_DECISION_ENDPOINT", "https://ollama.com/v1/systemone"),
        "model": os.environ.get("OLLAMA_DECISION_MODEL", "clef"),
        "key_env": "OLLAMA_DECISION_KEY",
        "max_questions": 64,
        "price_per_mtok": 0.24,  # 美元/百万输入 token；clef-flash 按同价估
    },
    "aliyun": {
        # 端点带业务空间 ID，格式 llm-xxxx；不填 workspace 时拼不出来，启动即报错
        "endpoint": "",
        "model": os.environ.get("ALIYUN_DECISION_MODEL", "decision-model-preview"),
        "key_env": "ALIYUN_DECISION_KEY",
        "max_questions": 16,
        "price_per_mtok": 0.0,  # 文档标注「限时免费」，按 0 计
        "workspace_env": "ALIYUN_DECISION_WORKSPACE",
        "region": os.environ.get("ALIYUN_DECISION_REGION", "cn-beijing"),
    },
}
KEY_ENV = "OLLAMA_DECISION_KEY"  # 兼容旧引用；实际按 backend 取

# 金标准归一：库里历史分类名与一级选项名不完全一致，比较前先收敛到同一个字。
# 与 scripts/classify-backfill.py 的 normalize_l1 同口径（否则「悬疑灵异」必然全判错）。
GOLD_NORMALIZE = {"\u60ac\u7591\u7075\u5f02": "\u60ac\u7591", "\u7075\u5f02": "\u60ac\u7591"}


def normalize_label(name):
    """把历史分类名收敛到一级选项名。"""
    name = (name or "").strip()
    return GOLD_NORMALIZE.get(name, name)


# 一级分类判据。顺序即并列时的优先级：常用类靠前，兜底类靠后。
# 判据只写「读正文能验证的题材信号」，不写真实书名 / 作者（隐私红线）。
# 选项名必须与库里的 category_l1 一致；命中隐私黑名单的字面量用 \u 转义。
L1_CHOICES = [
    ("玄幻", "东方玄幻：斗气、武魂、异界大陆、神帝武帝、万古吞噬等架空修炼世界"),
    ("仙侠", "修真仙侠：宗门、灵根、炼气筑基结丹、渡劫飞升、仙尊洪荒"),
    ("都市", "现代城市里的故事：校园职场娱乐圈、兵王总裁神豪保镖、现实社会背景"),
    ("历史", "古代或架空历史：朝代权谋争霸、帝王将相、穿越到古代"),
    ("科幻", "星际机甲、末世废土、赛博朋克、虫族银河等未来或科技设定"),
    ("游戏", "网游电竞、虚拟现实游戏世界、副本公会全息"),
    ("武侠", "传统武侠：江湖门派、侠客剑客、武林绝学内力"),
    ("悬疑", "悬疑推理、灵异盗墓、探案惊悚驱魔"),
    ("军事", "军事战争、军旅特种兵、雇佣兵战场"),
    ("体育", "体育竞技：足球篮球联赛、教练球员、专业运动"),
    ("奇幻", "西方奇幻：魔法精灵骑士巨龙勇者魔王"),
    ("轻小说", "二次元轻小说、同人综漫、日式风格"),
    ("言情", "以恋爱感情为主线的女频作品"),
    ("无限流", "主神空间、轮回副本、无限世界任务"),
    ("后宫", "多女性角色感情线为主的男性向作品"),
    ("文学", "严肃文学、散文杂文、文艺风格"),
    ("社科", "社科人文、历史非虚构、纪实读物"),
    ("传记", "人物传记、真实人物纪实"),
]


# 问题集：一次请求带多个问题，每题独立打分。
# 实测（clef 云端，同 10 本）：1 问 ≈ 1042 tok/本；5 问 ≈ 1419 tok/本，只多 36%，
# 相当于比「问 5 次」省 73%，延迟基本不变（同一本书只跑一次前向）。
# 云端硬上限：一次最多 64 个问题，问题名字母数字 _. 最多 100 字符。
QUESTION_PRESETS = {
    # 只判一级题材，最省——批量回填默认用它
    "genre": {
        "genre": {
            "type": "choice",
            "instructions": "这本书属于哪个一级题材分类？按题材设定判断，"
                            "不要被书名里的字面或作者名带偏。",
            "criteria": dict(L1_CHOICES),
        }
    },
    # 判题材的同时顺手抽取几个筛选字段：受众、是否网文、正文质量、有无广告
    "full": {
        "genre": {
            "type": "choice",
            "instructions": "这本书属于哪个一级题材分类？按题材设定判断，"
                            "不要被书名里的字面或作者名带偏。",
            "criteria": dict(L1_CHOICES),
        },
        "is_webnovel": {
            "type": "noul",
            "instructions": "这是网络小说连载，而不是出版散文、社科专著或传记吗？",
        },
        "audience": {
            "type": "choice",
            "instructions": "目标读者偏男性向还是女性向？",
            "criteria": {"male": "男性向（男频）", "female": "女性向（女频）",
                         "neutral": "不区分性别"},
        },
        "quality": {
            "type": "score",
            "instructions": "正文可读性如何？按是否残缺、是否水字数、可读程度评分。",
            "criteria": ["很差（明显残缺或水字数）", "一般", "较好", "很好"],
        },
        "has_promo": {
            "type": "noul",
            "instructions": "正文里是否混入大量站点广告、乱码或无关内容？",
        },
    },
}


def load_questions(preset, path, limit=None):
    """取本次要问的问题集：--questions 指定的 JSON 优先，否则用内置 preset。

    limit 是后端的问题数上限（ollama 64 / 阿里云 16）：超出直接报错，
    让调用方在开跑前就知道，而不是跑到一半被服务端 400 打断。
    """
    if path:
        with open(path, encoding="utf-8") as handle:
            questions = json.load(handle)
        if not isinstance(questions, dict) or not questions:
            raise SystemExit("问题集必须是 {问题名: {type, instructions, criteria}} 的对象")
    else:
        if preset not in QUESTION_PRESETS:
            raise SystemExit("未知 preset：%s（可选 %s）"
                             % (preset, "、".join(sorted(QUESTION_PRESETS))))
        questions = QUESTION_PRESETS[preset]
    limit = int(limit or 64)
    if len(questions) > limit:
        raise SystemExit("问题数 %d 超过该后端上限 %d 个" % (len(questions), limit))
    return questions


class AuthError(Exception):
    """凭据缺失或无效：不重试，立刻停。"""


class RetryableError(Exception):
    """超时、限流、5xx：退避后重试。"""


class SkippedError(Exception):
    """这一本被判不了（内容审核拒收、超长等）：记一条跳过，继续跑别的。"""


class FatalError(Exception):
    """额度不足、模型不存在、凭据无效：重试也没用，整批停。"""


def resolve_backend(name, args):
    """把后端名 + 命令行覆盖合成一份实际使用的配置。

    命令行给了 --endpoint / --model / --api-key-env 就覆盖后端默认值，
    这样同一份脚本既能跑 ollama 云端，也能跑阿里云百炼。
    """
    if name not in BACKENDS:
        raise SystemExit("未知后端：%s（可选 %s）" % (name, "、".join(sorted(BACKENDS))))
    cfg = dict(BACKENDS[name])
    endpoint = args.endpoint or cfg.get("endpoint") or ""
    if not endpoint and name == "aliyun":
        workspace = (os.environ.get(cfg["workspace_env"]) or "").strip()
        if not workspace:
            raise SystemExit(
                "阿里云后端需要业务空间 ID：设置环境变量 " + cfg["workspace_env"]
                + "（控制台右上角图标里复制，形如 llm-xxxx），或用 --endpoint 直接给全 URL"
            )
        endpoint = ("https://%s.%s.maas.aliyuncs.com/compatible-mode/v1/systemone"
                    % (workspace, cfg["region"]))
    cfg["endpoint"] = endpoint
    cfg["model"] = args.model or cfg.get("model") or ""
    cfg["key_env"] = args.api_key_env or cfg.get("key_env") or ""
    cfg["max_questions"] = int(cfg.get("max_questions") or 64)
    return cfg


def load_key(env_name):
    """从指定环境变量取 key，缺失时报错并给出改写办法。绝不打印值。"""
    key = (os.environ.get(env_name) or "").strip()
    if not key:
        raise AuthError(
            "环境变量 " + env_name + " 为空。macOS 上它通常写在 ~/.zshrc，"
            "非交互 shell 读不到，改用 zsh -ic '<command>' 运行；"
            "也可以 --api-key-env 换成别的变量名。"
        )
    return key


def build_state(item):
    """把一本书组装成 state：有正文摘录就用摘录，正文信号比书名可靠。"""
    state = {"title": item.get("title") or "", "author": item.get("author") or ""}
    intro = (item.get("intro") or "").strip()
    if intro:
        state["intro"] = intro
    excerpts = [x.strip() for x in (item.get("excerpts") or []) if (x or "").strip()]
    if excerpts:
        state["excerpts"] = excerpts
    return state


def build_payload(model, item, questions):
    """拼请求：state 是这本书，questions 是本次要问的全部问题。

    System One 一次请求最多带 64 个问题，每题独立打分、答案不串给下一题。
    多问一次比「同一本书反复问」省得多：实测 5 问一次只多 36% 输入 token，
    相当于比问 5 次省 73%，延迟基本不变（同一次前向）。
    """
    return {"model": model, "state": build_state(item), "questions": questions}


def post_once(endpoint, key, payload, timeout):
    """POST 一次，返回 (answers, usage)。按错误类型决定是否值得重试。

    answers 是 {问题名: 答案对象}：choice 型带 choice/probabilities/confidence，
    noul 型带 noul 概率，score 型带 score/legend。全部原样返回，由调用方挑选。
    """
    headers = {"Content-Type": "application/json"}
    if key:
        headers["Authorization"] = "Bearer " + key
    req = urllib.request.Request(endpoint, data=json.dumps(payload).encode("utf-8"),
                                 headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as err:
        detail = ""
        try:
            detail = err.read().decode("utf-8", "replace")[:200]
        except Exception:
            pass
        if err.code in (401, 403):
            raise AuthError("决策接口拒绝凭据（HTTP %d）：%s" % (err.code, detail))
        if err.code == 402:
            raise FatalError("云端额度不足（HTTP 402）：补 usage credits，或换本地模型")
        if err.code == 400 and "DataInspectionFailed" in detail:
            # 内容安全审核拒收：只影响这一本，不当成致命错误
            raise SkippedError("内容审核拒收：%s" % detail[:120])
        if err.code in (400, 404, 413):
            raise FatalError("请求被拒（HTTP %d）：%s" % (err.code, detail))
        raise RetryableError("HTTP %d：%s" % (err.code, detail))
    except (urllib.error.URLError, TimeoutError, OSError) as err:
        raise RetryableError("网络错误：%s" % err)
    except ValueError as err:
        raise RetryableError("响应不是合法 JSON：%s" % err)
    answers = body.get("answers") or {}
    if not answers:
        raise RetryableError("响应里没有 answers")
    return answers, body.get("usage") or {}


def classify_one(opts, item, usage, lock):
    """判一本：一次请求问完 opts.questions，网络类失败退避重试。

    结果行形状：{id, category, confidence, runner_up, probabilities, extra}。
    category/confidence/runner_up/probabilities 这四项是给
    scripts/classify-backfill.py --agents 用的固定契约；其余问题（受众、
    是否网文、质量分等）原样放进 extra，不影响既有写库链路。
    """
    payload = build_payload(opts.model, item, opts.questions)
    for attempt in range(opts.retries + 1):
        try:
            answers, used = post_once(opts.endpoint, opts.key, payload, opts.timeout)
        except RetryableError as err:
            if attempt >= opts.retries:
                return {"id": int(item["id"]), "error": str(err)}
            time.sleep(min(2 ** attempt, 8))
            continue
        except SkippedError as err:
            return {"id": int(item["id"]), "error": "skipped: " + str(err)}
        with lock:
            usage["input_tokens"] += int(used.get("input_tokens") or 0)
            usage["output_tokens"] += int(used.get("output_tokens") or 0)
        genre = answers.get("genre") or {}
        if not genre.get("choice"):
            return {"id": int(item["id"]), "error": "响应里没有 genre.choice"}
        probs = {k: round(float(v), 4) for k, v in (genre.get("probabilities") or {}).items()}
        ranked = sorted(probs.items(), key=lambda kv: kv[1], reverse=True)
        row = {
            "id": int(item["id"]),
            "category": genre["choice"],
            "confidence": round(float(genre.get("confidence") or 0.0), 4),
            "runner_up": ranked[1][0] if len(ranked) > 1 else "",
            "probabilities": probs,
        }
        extra = {k: v for k, v in answers.items() if k != "genre"}
        if extra:
            row["extra"] = extra
        return row
    return {"id": int(item["id"]), "error": "exhausted"}


def load_json_items(patterns):
    """按 glob 读全部证据包，按 id 去重排序。"""
    items = {}
    for pattern in patterns:
        for path in sorted(glob.glob(pattern)):
            with open(path, encoding="utf-8") as handle:
                for item in json.load(handle):
                    try:
                        items[int(item["id"])] = item
                    except (KeyError, TypeError, ValueError):
                        continue
    return [items[k] for k in sorted(items)]


def load_done_ids(path):
    """读已有 JSONL，返回已判出分类的 book_id 集合（续跑用）。"""
    done = set()
    if not os.path.exists(path):
        return done
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
                done.add(int(row["id"]))
    return done


def load_rows(path):
    """读结果 JSONL，按 book_id 去重：同一本重跑多次只保留最后一条。

    没有去重会让 report 的 n / 准确率被重复行放大——续跑、补跑、
    中途重试用同一个输出文件时必然出现重复。
    """
    latest = {}
    if not os.path.exists(path):
        return []
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            line = line.strip()
            if not line:
                continue
            try:
                row = json.loads(line)
                latest[int(row["id"])] = row
            except (KeyError, TypeError, ValueError):
                continue
    return [latest[k] for k in sorted(latest)]


def run_batch(opts, items, out_path):
    """多线程跑完 items，每条结果即时追加进 JSONL。"""
    lock = threading.Lock()
    usage = {"input_tokens": 0, "output_tokens": 0}
    stats = {"ok": 0, "failed": 0}
    started = time.time()
    sink = open(out_path, "a", encoding="utf-8")

    def worker(item):
        return classify_one(opts, item, usage, lock)

    try:
        with ThreadPoolExecutor(max_workers=opts.workers) as pool:
            for row in pool.map(worker, items):
                with lock:
                    sink.write(json.dumps(row, ensure_ascii=False) + "\n")
                    sink.flush()
                    stats["failed" if row.get("error") else "ok"] += 1
                    done = stats["ok"] + stats["failed"]
                if opts.progress and done % opts.progress == 0:
                    rate = done / max(time.time() - started, 1e-6)
                    print("  进度 %d/%d · %.1f 条/秒" % (done, len(items), rate), flush=True)
    finally:
        sink.close()
    return stats, usage, time.time() - started


def write_report(report, path):
    """报告落盘；同时把摘要打到 stdout。"""
    if path:
        with open(path, "w", encoding="utf-8") as handle:
            json.dump(report, handle, ensure_ascii=False, indent=1)
    summary = report["summary"]
    print("\n== 金标准评测 ==")
    print("样本 %d（判出 %d / 失败 %d）" % (summary["n"], summary["ok"], summary["failed"]))
    print("总体准确率 %.1f%%" % (summary["accuracy"] * 100))
    print("Top-2 命中率 %.1f%%" % (summary["top2_accuracy"] * 100))
    print("平均 confidence %.3f" % summary["mean_confidence"])
    if summary.get("normalized_gold"):
        print("金标准已归一：%s（与库里 category_l1 历史名对齐）"
              % "、".join(summary["normalized_gold"]))
    print("\n== 按 confidence 阈值取舍（覆盖 = 肯写库的比例）==")
    print("%-10s %-12s %s" % ("阈值", "覆盖", "准确率"))
    for row in report["thresholds"]:
        print("%-10.2f %-12s %.1f%%" % (row["threshold"], "%.1f%%" % (row["coverage"] * 100),
                                        row["accuracy"] * 100))
    print("\n== 各类准确率（样本 >= 5）==")
    for row in report["per_class"]:
        if row["n"] >= 5:
            print("%-8s n=%-5d 准确率 %.1f%%" % (row["label"], row["n"], row["accuracy"] * 100))
    if report["confusions"]:
        print("\n== 最常错配（gold -> 预测）==")
        for gold, pred, count in report["confusions"]:
            print("%-8s -> %-8s %d" % (gold, pred, count))
    print("\n成本：输入 %d tokens ≈ $%.4f · 耗时 %.1f 秒"
          % (summary["input_tokens"], summary["cost_usd"], summary["elapsed"]))


def build_report(rows, gold_map, usage, elapsed, price_per_mtok):
    """把判定结果与金标准对齐，算精度、阈值取舍与混淆。

    入参 rows 已按 id 去重（见 load_rows）；这里再按 id 收敛一次，
    保证无论调用方怎么组织数据，分母都不会被重复行放大。
    """
    unique = {int(r["id"]): r for r in rows}
    graded = [r for r in unique.values() if r.get("category") and r["id"] in gold_map]
    hits = [r for r in graded if r["category"] == gold_map[r["id"]]]
    per_class = {}
    for row in graded:
        item = per_class.setdefault(gold_map[row["id"]], {"n": 0, "hit": 0})
        item["n"] += 1
        item["hit"] += 1 if row["category"] == gold_map[row["id"]] else 0
    confusions = {}
    for row in graded:
        gold = gold_map[row["id"]]
        if row["category"] != gold:
            confusions[(gold, row["category"])] = confusions.get((gold, row["category"]), 0) + 1
    thresholds = []
    for threshold in (0.0, 0.5, 0.6, 0.7, 0.8, 0.9):
        kept = [r for r in graded if r["confidence"] >= threshold]
        hits_kept = [r for r in kept if r["category"] == gold_map[r["id"]]]
        thresholds.append({
            "threshold": threshold,
            "coverage": (len(kept) / len(graded)) if graded else 0.0,
            "accuracy": (len(hits_kept) / len(kept)) if kept else 0.0,
        })
    top2 = sum(1 for r in graded
               if r["category"] == gold_map[r["id"]] or r.get("runner_up") == gold_map[r["id"]])
    return {
        "summary": {
            "n": len(rows),
            "ok": len([r for r in rows if r.get("category")]),
            "failed": len([r for r in rows if r.get("error")]),
            "accuracy": (len(hits) / len(graded)) if graded else 0.0,
            "top2_accuracy": (top2 / len(graded)) if graded else 0.0,
            "mean_confidence": (sum(r["confidence"] for r in graded) / len(graded)) if graded else 0.0,
            "input_tokens": usage["input_tokens"],
            "output_tokens": usage["output_tokens"],
            "cost_usd": round(usage["input_tokens"] / 1e6 * price_per_mtok, 4),
            "elapsed": round(elapsed, 1),
            "normalized_gold": ["\u60ac\u7591\u7075\u5f02", "\u7075\u5f02"],
        },
        "thresholds": thresholds,
        "per_class": [{"label": k, "n": v["n"], "accuracy": v["hit"] / v["n"]}
                      for k, v in sorted(per_class.items(), key=lambda kv: -kv[1]["n"])],
        "confusions": [[g, p, c] for (g, p), c in
                       sorted(confusions.items(), key=lambda kv: -kv[1])[:10]],
    }


def cmd_run(args):
    """批量判类：证据包 -> JSONL，支持续跑。"""
    opts = argparse.Namespace(**vars(args))
    cfg = resolve_backend(args.backend, args)
    opts.endpoint = cfg["endpoint"]
    opts.model = cfg["model"]
    opts.key = load_key(cfg["key_env"])
    opts.questions = load_questions(args.preset, args.questions, cfg["max_questions"])
    opts.price = float(cfg["price_per_mtok"])
    print("后端 %s · 模型 %s · 端点 %s" % (args.backend, opts.model, opts.endpoint))
    items = load_json_items(args.evidence)
    if not items:
        raise SystemExit("证据包为空：检查 --evidence 的 glob 是否匹配到文件")
    done = load_done_ids(args.out)
    todo = [x for x in items if int(x["id"]) not in done]
    print("待判 %d 本（已判 %d 本跳过）· 模型 %s · 并发 %d"
          % (len(todo), len(items) - len(todo), args.model, args.workers))
    if not todo:
        return 0
    stats, usage, elapsed = run_batch(opts, todo, args.out)
    print("完成 ok=%d failed=%d · 输入 %d tokens ≈ $%.4f · 耗时 %.1f 秒"
          % (stats["ok"], stats["failed"], usage["input_tokens"],
             usage["input_tokens"] / 1e6 * opts.price, elapsed))
    return 0 if stats["failed"] == 0 else 1


def cmd_eval(args):
    """金标准评测：算总体精度、阈值取舍与混淆。"""
    opts = argparse.Namespace(**vars(args))
    cfg = resolve_backend(args.backend, args)
    opts.endpoint = cfg["endpoint"]
    opts.model = cfg["model"]
    opts.key = load_key(cfg["key_env"])
    opts.questions = load_questions(args.preset, args.questions, cfg["max_questions"])
    opts.price = float(cfg["price_per_mtok"])
    print("后端 %s · 模型 %s · 端点 %s" % (args.backend, opts.model, opts.endpoint))
    with open(args.gold, encoding="utf-8") as handle:
        gold = json.load(handle)
    gold_map = {int(x["id"]): normalize_label(x.get("gold") or x.get("category")) for x in gold}
    samples = [x for x in gold if gold_map.get(int(x["id"]))]
    done = load_done_ids(args.out)
    todo = [x for x in samples if int(x["id"]) not in done]
    print("评测 %d 本（金标准来自库里未被自动打标的原始分类）· 模型 %s · 并发 %d"
          % (len(samples), args.model, args.workers))
    if todo:
        print("  本次判 %d 本（已判 %d 本跳过）" % (len(todo), len(samples) - len(todo)))
    else:
        print("  全部已判，直接出报告")
    stats, usage, elapsed = run_batch(opts, todo, args.out)
    rows = load_rows(args.out)
    report = build_report(rows, gold_map, usage, elapsed, opts.price)
    report["summary"]["failed"] = stats["failed"]
    write_report(report, args.report)
    return 0


def cmd_export(args):
    """把 JSONL 转成 backfill --agents 能吃的数组，按 confidence 过滤。"""
    rows = []
    with open(args.labels, encoding="utf-8") as handle:
        for line in handle:
            line = line.strip()
            if not line:
                continue
            row = json.loads(line)
            if not row.get("category"):
                continue
            if float(row.get("confidence") or 0.0) < args.min_confidence:
                continue
            rows.append({"id": row["id"], "category": row["category"]})
    rows.sort(key=lambda x: x["id"])
    with open(args.out, "w", encoding="utf-8") as handle:
        json.dump(rows, handle, ensure_ascii=False)
    print("导出 %d 本（confidence >= %.2f）-> %s" % (len(rows), args.min_confidence, args.out))
    return 0


def build_parser():
    """命令行：eval / run / export 三个子命令，公共参数复用。"""
    parser = argparse.ArgumentParser(description="用决策模型（Ollama System One）判题材分类")
    sub = parser.add_subparsers(dest="command", required=True)

    def add_common(p):
        p.add_argument("--backend", default="ollama", help="后端：ollama（clef 云端）或 aliyun（百炼）")
        p.add_argument("--endpoint", default="", help="覆盖后端端点（默认按后端拼）")
        p.add_argument("--model", default="", help="覆盖模型名（默认按后端取）")
        p.add_argument("--api-key-env", default="", dest="api_key_env",
                       help="换成别的凭据环境变量名（默认按后端取）")
        p.add_argument("--workers", type=int, default=4, help="并发请求数（云端按套餐限并发）")
        p.add_argument("--retries", type=int, default=3, help="网络类失败重试次数")
        p.add_argument("--timeout", type=float, default=60.0, help="单请求超时秒数")
        p.add_argument("--progress", type=int, default=100, help="每 N 条打一行进度（0 = 静默）")
        p.add_argument("--out", required=True, help="结果 JSONL 路径（续跑会跳过已判 id）")
        p.add_argument("--preset", default="genre", help="内置问题集名（genre / full，默认 genre）")
        p.add_argument("--questions", default="",
                       help="自定义问题集 JSON：{问题名: {type, instructions, criteria}}，"
                            "一次最多 64 个问题")

    p_eval = sub.add_parser("eval", help="金标准评测")
    add_common(p_eval)
    p_eval.add_argument("--gold", required=True, help="金标准 JSON：[{id,title,intro,gold}]")
    p_eval.add_argument("--report", default="", help="评测报告输出路径（JSON）")
    p_eval.set_defaults(func=cmd_eval)

    p_run = sub.add_parser("run", help="批量判类")
    add_common(p_run)
    p_run.add_argument("--evidence", nargs="+", required=True, help="证据包 glob（可多个）")
    p_run.set_defaults(func=cmd_run)

    p_export = sub.add_parser("export", help="导出 backfill 用的标注数组")
    p_export.add_argument("--labels", required=True, help="run 产出的 JSONL")
    p_export.add_argument("--out", required=True, help="输出 JSON（[{id,category}]）")
    p_export.add_argument("--min-confidence", type=float, default=0.7, help="只导出不低于该置信度的")
    p_export.set_defaults(func=cmd_export)
    return parser


def main(argv=None):
    """入口：解析参数，按子命令分派。"""
    args = build_parser().parse_args(argv)
    try:
        return args.func(args)
    except AuthError as err:
        print("[错误] " + str(err), file=sys.stderr)
        return 2
    except FatalError as err:
        print("[错误] " + str(err), file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
