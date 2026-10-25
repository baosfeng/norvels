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
  storageRatioNote,
} from "./charts";
import { findForbiddenTerms } from "./audit";
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
  // 正文是压缩后的实际占用；textBytesRaw 是压缩前的原始文字字节
  textBytes: 101563175,
  textBytesRaw: 300000000,
  indexBytes: 73533895,
  otherBytes: 151456530,
};

/**
 * 真实库实测（838 本 / 42.27 万章，跑完块压缩迁移并 VACUUM 之后）。
 *
 * 字节取自库里的实际值，不是换算出来的：
 *   textBytes     = SUM(length(packed)) + books 侧文本 = 1,690,357,569
 *   textBytesRaw  = SUM(raw_bytes)                    = 4,874,681,744
 *   dbBytes       = 主库文件大小                       = 1,743,683,584（1.62 GiB）
 *   indexBytes    = 文件 − 正文 − 书目文本（后端口径）  = 53,326,015
 *   otherBytes    = dbBytes − 正文 − 书目（后端 fillComposition 兜底后实际为 0）
 *
 * 注：口头常说的「正文 1.69 GB」是十进制 GB，前端按 1024 进制显示为 1.6 GB，两者是同一个数。
 */
const realStats: Stats = {
  ...baseStats,
  books: 838,
  chapters: 422715,
  dbBytes: 1_743_683_584,
  textBytes: 1_690_357_569,
  textBytesRaw: 4_874_681_744,
  indexBytes: 53_326_015,
  otherBytes: 0,
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
    // 只有 2 天有导入时不说「日均」，否则 838 本挤一天也会读成「日均 60 本」
    expect(model.note).toBe("近 14 天导入 9 本，只有 2 天有导入");
    const max = Math.max(...model.data.bars.map((b) => b.heightPct));
    expect(max).toBe(100);
    // 0 值高度为 0（渲染层据此不画柱），不再给 2% 的噪音基线
    expect(Math.min(...model.data.bars.map((b) => b.heightPct))).toBe(0);
  });

  it("全 0 时判为空，结论说「还没有导入」", () => {
    const model = buildImportTrend(trend([0, 0, 0]));
    expect(model.empty).toBe(true);
    expect(model.note).toContain("还没有导入记录");
  });

  it("每天都有导入时仍说日均", () => {
    const model = buildImportTrend(trend(new Array(14).fill(2)));
    expect(model.note).toBe("近 14 天导入 28 本，日均 2 本");
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

  it("后端给了聚合值就以它为准，不用再猜「其余的书」", () => {
    const model = buildReadingDist(progress([{ bookId: 1, percent: 0.2 }]), 1000, true, {
      unread: 900,
      reading: 60,
      finished: 40,
    });
    expect(model.data.finished).toBe(40);
    expect(model.data.reading).toBe(60);
    expect(model.data.unread).toBe(900);
    expect(model.data.partial).toBe(false);
    expect(model.note).toContain("翻过 100 本");
  });

  it("后端聚合值为全 0 时回退到按记录估算", () => {
    const model = buildReadingDist(progress([{ bookId: 1, percent: 0.5 }]), 10, false, {
      unread: 0,
      reading: 0,
      finished: 0,
    });
    expect(model.data.reading).toBe(1);
    expect(model.data.unread).toBe(9);
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
    // 归一化后最小宽度会按比例略微收缩（1.2 → ~1.19），仍远高于「看不见」的比例
    expect(index?.widthPct).toBeGreaterThanOrEqual(1);
    expect(index?.display).toBe("0.1 MB");
  });

  it("正常比例下三段按真实占比画，结论给出总量与正文占比", () => {
    const model = buildStorage(baseStats);
    const sum = model.data.parts.reduce((acc, p) => acc + p.widthPct, 0);
    expect(sum).toBeCloseTo(100, 6);
    expect(model.note).toContain("正文占");
    expect(model.data.parts.map((p) => p.label)).toEqual(["正文", "书目信息", "其他"]);
  });

  // 正文口径改成「压缩后字节」后，正文段会明显变大（占满全库），三段宽度仍然要落在 100% 上：
  // 既不能因为最小宽度放大而溢出，也不能因为取整而留下缝。
  it("压缩后的正文口径下，三段渲染宽度之和仍是 100", () => {
    const model = buildStorage(realStats);
    const sum = model.data.parts.reduce((acc, p) => acc + p.widthPct, 0);
    expect(Math.round(sum * 100) / 100).toBe(100);
    expect(Math.round(model.data.widthSum * 100) / 100).toBe(100);
    // 真实占比之和也是 100（渲染宽度做过最小宽度归一，真实占比不受影响）
    expect(model.data.shareSum).toBeCloseTo(100, 6);
    for (const part of model.data.parts) {
      expect(part.widthPct).toBeGreaterThanOrEqual(0);
      expect(Number.isFinite(part.widthPct)).toBe(true);
    }
  });

  it("压缩率按压缩后 / 原始算，给出「省了多少」的人话", () => {
    const model = buildStorage(realStats);
    // 实测 34.6763% → 一位小数 34.7%；取整会说成 35%，与真实压缩率差 0.3 个点
    expect(model.data.compressionPct).toBe(34.7);
    // 不复述图例里已有的正文体积，只给「比例 + 省下多少」两个新信息
    expect(storageRatioNote(model.data)).toBe("正文约为原始文字的 34.7%，比不压缩省下 3.0 GB");
    expect(storageRatioNote(model.data)).not.toContain("1.6 GB");
    // 去术语化：不出现算法名 / 实现词
    expect(findForbiddenTerms(storageRatioNote(model.data))).toEqual([]);
  });

  it("没有原始字节（老接口）时不编造压缩率", () => {
    const model = buildStorage({ ...baseStats, textBytesRaw: undefined });
    expect(model.data.compressionPct).toBeNull();
    expect(storageRatioNote(model.data)).toBe("");
  });

  it("正文还没拆出来（统计中）时不显示压缩率", () => {
    const model = buildStorage({ ...baseStats, textBytes: 0, textBytesRaw: 5_000_000_000, indexBytes: 0, otherBytes: 326_574_080 });
    expect(model.data.compressionPct).toBeNull();
    expect(storageRatioNote(model.data)).toBe("");
  });

  it("压缩后和原文一样大（没省下字节）时不硬凑一句压缩率", () => {
    const model = buildStorage({ ...baseStats, textBytes: 1_000_000, textBytesRaw: 1_000_000 });
    expect(model.data.compressionPct).toBe(100);
    expect(model.data.savedBytes).toBe(0);
    expect(storageRatioNote(model.data)).toBe("");
  });

  it("脏数据（负数 otherBytes）也不画出负宽度", () => {
    const model = buildStorage({ ...baseStats, textBytes: 100, indexBytes: 50, otherBytes: -80 });
    for (const part of model.data.parts) {
      expect(part.widthPct).toBeGreaterThanOrEqual(0);
    }
    expect(model.data.widthSum).toBeCloseTo(100, 6);
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