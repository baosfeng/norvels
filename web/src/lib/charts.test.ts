import { describe, expect, it } from "vitest";
import {
  buildChapterBuckets,
  buildChapterLength,
  buildImportTrend,
  buildReadingDist,
  buildStorage,
  buildTopAuthors,
  CIRCUMFERENCE,
  compact,
} from "./charts";
import type { Bucket, ImportTrendPoint, ReadingProgress, Stats } from "../api/types";

const trend = (books: number[]): ImportTrendPoint[] =>
  books.map((b, i) => ({ date: `2026-10-${String(i + 1).padStart(2, "0")}`, books: b, chapters: b * 100 }));

const buckets = (counts: number[]): Bucket[] =>
  counts.map((count, i) => ({ label: `桶${i}`, count }));

const progress = (entries: { bookId: number; percent: number; updatedAt?: string }[]): ReadingProgress[] =>
  entries.map((e) => ({
    bookId: e.bookId,
    bookTitle: `书${e.bookId}`,
    chapterId: e.bookId * 10,
    chapterIdx: 0,
    chapterTitle: "第一章",
    percent: e.percent,
    updatedAt: e.updatedAt ?? "2026-10-24T12:00:00.000Z",
  }));

const baseStats: Stats = {
  books: 19,
  chapters: 11668,
  authors: 17,
  words: 30418864,
  dbBytes: 326553600,
  lastImportAt: "2026-10-24T20:25:26.409Z",
  textBytes: 101563175,
  indexBytes: 73533895,
  otherBytes: 151456530,
};

describe("数字压缩显示", () => {
  it("万级/亿级/零值", () => {
    expect(compact(0)).toBe("0");
    expect(compact(9999)).toBe("9999");
    expect(compact(12345)).toBe("1.2 万");
    expect(compact(2_280_000_000)).toBe("22.8 亿");
  });
});

describe("1 近 14 天导入量", () => {
  it("总量与日均按实际条数算，柱高归一到最大值", () => {
    const model = buildImportTrend(trend([0, 0, 3, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0]));
    expect(model.data.total).toBe(9);
    expect(model.note).toBe("近 14 天导入 19 本，日均 1 本".replace("19", "9"));
    const max = Math.max(...model.data.bars.map((b) => b.heightPct));
    expect(max).toBe(100);
    // 0 值也保留可见基线
    expect(Math.min(...model.data.bars.map((b) => b.heightPct))).toBe(2);
  });

  it("全 0 时判为空，结论说「还没有导入」", () => {
    const model = buildImportTrend(trend([0, 0, 0]));
    expect(model.empty).toBe(true);
    expect(model.note).toContain("还没有导入记录");
  });

  it("日期标签稀疏化，且末尾两根不会贴在一起", () => {
    const model = buildImportTrend(trend(new Array(14).fill(1)));
    const visible = model.data.bars.filter((b) => b.labelVisible).map((b) => b.label);
    expect(visible[visible.length - 1]).toBe("10/14");
    // 14 条时原型口径是 5 个标签（0/3/6/9/13）
    expect(visible).toEqual(["10/1", "10/4", "10/7", "10/10", "10/14"]);
  });
});

describe("2 章节数分布", () => {
  it("找出主力区间并写进结论", () => {
    const model = buildChapterBuckets(buckets([0, 0, 1, 8, 6, 4]));
    expect(model.note).toBe("19 本书里，桶3 章的最多（8 本）");
    expect(model.data.peak).toBe("桶3");
  });

  it("空数据判空", () => {
    expect(buildChapterBuckets([]).empty).toBe(true);
    expect(buildChapterBuckets(buckets([0, 0])).empty).toBe(true);
  });
});

describe("3 字数 Top 10 作者", () => {
  const authors = Array.from({ length: 12 }, (_, i) => ({
    name: `作者${i}`,
    wordCount: (12 - i) * 1_000_000,
  }));

  it("只取前 10，按字数降序，占比用全库总字数算", () => {
    const model = buildTopAuthors(authors, 78_000_000);
    expect(model.data.rows).toHaveLength(10);
    expect(model.data.rows[0].name).toBe("作者0");
    expect(model.data.rows[0].widthPct).toBe(100);
    // Top10 之和是 12+11+…+3 = 75 万，占全库 78 万的 96%
    expect(model.data.sharePct).toBe(96);
    expect(model.note).toContain("作者0");
  });

  it("百分比按全库总字数而不是 Top10 之和", () => {
    const model = buildTopAuthors([{ name: "甲", wordCount: 500 }], 1000);
    expect(model.data.sharePct).toBe(50);
  });

  it("没有作者时判空", () => {
    expect(buildTopAuthors([], 100).empty).toBe(true);
  });
});

describe("4 阅读进度分布", () => {
  it("按书去重、只保留每本最新一条，三态相加等于总书数", () => {
    const model = buildReadingDist(
      progress([
        { bookId: 1, percent: 0.1, updatedAt: "2026-10-24T10:00:00.000Z" },
        { bookId: 1, percent: 1.0, updatedAt: "2026-10-24T12:00:00.000Z" },
        { bookId: 2, percent: 0.4 },
      ]),
      10,
    );
    expect(model.data.finished).toBe(1);
    expect(model.data.reading).toBe(1);
    expect(model.data.unread).toBe(8);
    expect(model.data.finished + model.data.reading + model.data.unread).toBe(10);
    expect(model.note).toContain("17 本还没开始".replace("17", "8"));
  });

  it("环形各段弧长之和等于周长", () => {
    const model = buildReadingDist(progress([{ bookId: 1, percent: 1 }, { bookId: 2, percent: 0.2 }]), 4);
    const total = model.data.segments.reduce((sum, s) => sum + Number(s.dash.split(" ")[0]), 0);
    expect(Math.round(total)).toBe(Math.round(CIRCUMFERENCE));
  });

  it("记录数达到上限时标注为不完整", () => {
    const model = buildReadingDist(progress([{ bookId: 1, percent: 0.2 }]), 100, true);
    expect(model.data.partial).toBe(true);
  });

  it("总书数为 0 时判空", () => {
    expect(buildReadingDist([], 0).empty).toBe(true);
  });
});

describe("5 库体积构成", () => {
  it("占比极小的一段也有可见宽度，图例仍给真实数值", () => {
    const model = buildStorage({ ...baseStats, textBytes: 96_819_177, indexBytes: 134_477, otherBytes: 229_620_426 });
    const index = model.data.parts.find((p) => p.key === "index");
    expect(index?.widthPct).toBeGreaterThanOrEqual(1.2);
    expect(index?.display).toBe("0.1 MB");
  });

  it("正常比例下三段按真实占比画，结论给出总量与正文占比", () => {
    const model = buildStorage(baseStats);
    const sum = model.data.parts.reduce((acc, p) => acc + p.widthPct, 0);
    expect(sum).toBeCloseTo(100, 1);
    expect(model.note).toContain("正文占");
    expect(model.data.parts.map((p) => p.label)).toEqual(["正文", "书目信息", "其他"]);
  });

  it("总量有值但正文/索引还没拆出来时，不谎报「正文占 0%」", () => {
    const model = buildStorage({ ...baseStats, textBytes: 0, indexBytes: 0, otherBytes: 326_574_080 });
    expect(model.note).toContain("体积构成正在统计");
    expect(model.data.compositionPending).toBe(true);
    expect(model.empty).toBe(false);
  });

  it("没有字节字段时判空（老接口没扩展的情况）", () => {
    const model = buildStorage({ ...baseStats, textBytes: undefined, indexBytes: undefined, otherBytes: undefined });
    expect(model.empty).toBe(true);
    expect(model.note).toContain("还没有可统计");
  });

  it("stats 为 null 也不崩", () => {
    expect(buildStorage(null).empty).toBe(true);
  });
});

describe("6 章节长度分布", () => {
  it("标出峰值桶与占比", () => {
    const model = buildChapterLength(buckets([1, 8, 2]));
    expect(model.data.peak).toBe("桶1");
    expect(model.note).toContain("73%");
  });

  it("空数据判空", () => {
    expect(buildChapterLength([]).empty).toBe(true);
  });
});