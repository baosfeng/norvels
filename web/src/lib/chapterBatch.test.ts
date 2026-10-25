import { describe, expect, it } from "vitest";
import type { Volume } from "../api/types";
import {
  chaptersInScope,
  MERGE_NEED_TWO_TEXT,
  MERGE_RULE_TEXT,
  mergeVerdict,
  renumberPlan,
  renamedTitle,
  sortByChapter,
  spanOf,
  stripChapterPrefix,
  type ChapterLike,
} from "./chapterBatch";

function chapter(id: number, idx: number, volumeId: number, title = `第 ${idx + 1} 章`, wordCount = 100): ChapterLike {
  return { id, idx, volumeId, title, wordCount };
}

describe("合并规则", () => {
  it("单章不可合并，给出原因", () => {
    expect(mergeVerdict([chapter(1, 0, 1)])).toEqual({ ok: false, reason: MERGE_NEED_TWO_TEXT });
  });

  it("同卷相邻的章可以合并（顺序打乱也认）", () => {
    expect(mergeVerdict([chapter(2, 1, 1), chapter(1, 0, 1), chapter(3, 2, 1)])).toEqual({ ok: true, reason: "" });
  });

  it("跨卷不可合并", () => {
    expect(mergeVerdict([chapter(1, 0, 1), chapter(2, 1, 2)])).toEqual({ ok: false, reason: MERGE_RULE_TEXT });
  });

  it("同卷但不连续不可合并", () => {
    expect(mergeVerdict([chapter(1, 0, 1), chapter(3, 2, 1)])).toEqual({ ok: false, reason: MERGE_RULE_TEXT });
  });

  it("空选择不报原因（批量条这时不渲染）", () => {
    expect(mergeVerdict([])).toEqual({ ok: false, reason: "" });
  });
});

describe("区间与字数摘要", () => {
  it("给出章数、首尾章号与字数之和", () => {
    expect(spanOf([chapter(1, 0, 1, "甲", 1200), chapter(2, 1, 1, "乙", 3400), chapter(3, 2, 1, "丙", 100)])).toEqual({
      count: 3,
      first: 1,
      last: 3,
      words: 4700,
    });
  });

  it("空选择的摘要全为 0", () => {
    expect(spanOf([])).toEqual({ count: 0, first: 0, last: 0, words: 0 });
  });

  it("排序按章号，不按 id", () => {
    expect(sortByChapter([chapter(9, 2, 1), chapter(3, 0, 1)]).map((c) => c.id)).toEqual([3, 9]);
  });
});

describe("去章号前缀", () => {
  it("认出「第 N 章 / 回 / 节」与中文数字", () => {
    expect(stripChapterPrefix("第十二章 巷口的灯")).toBe("巷口的灯");
    expect(stripChapterPrefix("第 12 章 巷口的灯")).toBe("巷口的灯");
    expect(stripChapterPrefix("第12回 巷口的灯")).toBe("巷口的灯");
    expect(stripChapterPrefix("第 3 节 巷口的灯")).toBe("巷口的灯");
  });

  it("前缀后随的顿号、句点、冒号与空格一并去掉", () => {
    expect(stripChapterPrefix("第 3 章、巷口的灯")).toBe("巷口的灯");
    expect(stripChapterPrefix("第 3 章．巷口的灯")).toBe("巷口的灯");
    expect(stripChapterPrefix("第 3 章：巷口的灯")).toBe("巷口的灯");
  });

  it("本身没有前缀的标题原样返回", () => {
    expect(stripChapterPrefix("巷口的灯")).toBe("巷口的灯");
    expect(stripChapterPrefix("序")).toBe("序");
  });

  it("标题只有章号时返回空串", () => {
    expect(stripChapterPrefix("第三章")).toBe("");
  });
});

describe("按序号重命名", () => {
  it("补上「第 N 章 」，N = idx + 1", () => {
    expect(renamedTitle("巷口的灯", 3)).toBe("第 3 章 巷口的灯");
  });

  it("已有正确章号时不重复叠加", () => {
    expect(renamedTitle("第 3 章 巷口的灯", 3)).toBe("第 3 章 巷口的灯");
    expect(renamedTitle("第三章 巷口的灯", 3)).toBe("第 3 章 巷口的灯");
  });

  it("标题只有章号时只留新章号", () => {
    expect(renamedTitle("第三章", 7)).toBe("第 7 章");
  });

  it("计划里每行都带 chapterId 与改前改后", () => {
    expect(renumberPlan([chapter(11, 0, 1, "巷口的灯"), chapter(12, 1, 1, "第 2 章 夜雨")])).toEqual([
      { chapterId: 11, before: "巷口的灯", after: "第 1 章 巷口的灯" },
      { chapterId: 12, before: "第 2 章 夜雨", after: "第 2 章 夜雨" },
    ]);
  });
});

describe("重命名范围", () => {
  const volumes: Volume[] = [
    {
      id: 1,
      idx: 0,
      title: "卷一",
      chapters: [
        { id: 1, idx: 0, title: "甲", volumeId: 1, wordCount: 10 },
        { id: 2, idx: 1, title: "乙", volumeId: 1, wordCount: 10 },
      ],
    },
    {
      id: 2,
      idx: 1,
      title: "卷二",
      chapters: [{ id: 3, idx: 2, title: "丙", volumeId: 2, wordCount: 10 }],
    },
  ];

  it("当前卷只取这一卷的章，章号沿用全书序号", () => {
    const plan = renumberPlan(chaptersInScope(volumes, { kind: "volume", volumeId: 2 }));
    expect(plan).toEqual([{ chapterId: 3, before: "丙", after: "第 3 章 丙" }]);
  });

  it("全书取所有卷的章", () => {
    expect(chaptersInScope(volumes, { kind: "book" }).map((c) => c.id)).toEqual([1, 2, 3]);
  });
});
