import type { HealthIssueKind } from "../../api/types";

/** 一类健康问题在卡片与抽屉里的全部文案与口径说明（四类各有一套，不共用一句话）。 */
export interface HealthIssueMeta {
  /** 抽屉标题 */
  title: string;
  /** 卡片上按钮的短标签（「缺作者 · 查看 3 本」） */
  entry: string;
  /** 计数单位：书级是「本」，章节级是「个」 */
  unit: "本" | "个";
  /** 没有这类问题时的整句说明（入口置灰时显示这句） */
  empty: string;
  /** 抽屉空态的描述 */
  emptyHint: string;
  /** 判定口径：抽屉里说明「这些条目是按什么筛出来的」 */
  rule: string;
  /** 问题出在章节上：条目显示章节标题 + 书名 */
  chapterLevel: boolean;
}

/**
 * kind → 文案的唯一来源。卡片入口、抽屉标题、空态说明都读这里，
 * 避免同一个问题在三个地方各写一句而产生不一致。
 */
export const HEALTH_ISSUE_META: Record<HealthIssueKind, HealthIssueMeta> = {
  missing_author: {
    title: "缺作者的书",
    entry: "缺作者",
    unit: "本",
    empty: "没有缺作者的书",
    emptyHint: "库里每本书都挂着作者。",
    rule: "筛选条件：书目里没有登记作者。补上作者后，书架与搜索结果里更容易分辨。",
    chapterLevel: false,
  },
  missing_intro: {
    title: "缺简介的书",
    entry: "缺简介",
    unit: "本",
    empty: "没有缺简介的书",
    emptyHint: "每本书都有简介。",
    rule: "筛选条件：书目里的简介为空。补一句简介，书架卡片上就能看出这本书讲什么。",
    chapterLevel: false,
  },
  thin_book: {
    title: "章节数偏少的书",
    entry: "章节数偏少",
    unit: "本",
    empty: "没有章节数偏少的书",
    emptyHint: "每本书都拆出了两章以上。",
    rule: "筛选条件：整本书只拆出 1 章或更少，通常是原文缺少章节标记。",
    chapterLevel: false,
  },
  huge_chapter: {
    title: "超长章节",
    entry: "超长章节",
    unit: "个",
    empty: "没有超长章节",
    emptyHint: "没有单章超过 3 万字的章节。",
    rule: "筛选条件：单章字数 ≥ 3 万字，可能是章节分界没识别到。",
    chapterLevel: true,
  },
};
