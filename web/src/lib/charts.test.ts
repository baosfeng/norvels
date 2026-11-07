import { describe, expect, it } from "vitest";
import {
  buildCategoryClicks,
  buildChapterBuckets,
  buildChapterLength,
  buildEventTrend,
  buildFunnel,
  buildHotBooks,
  buildImportCumulative,
  buildImportTrend,
  buildInterestBars,
  buildReasonMix,
  buildReadingDist,
  buildReadingTrend,
  buildRecommendMetrics,
  buildStorage,
  buildTopAuthors,
  CIRCUMFERENCE,
  compact,
  HOT_READ_WEIGHT,
  sharePctText,
  storageRatioNote,
} from "./charts";
import { findForbiddenTerms } from "./audit";
import type {
  Bucket,
  DaySeconds,
  ImportTrendPoint,
  ReadingProgress,
  RecommendHotBook,
  RecommendInterest,
  RecommendStatsDay,
  RecommendStatsSummary,
  Stats,
} from "../api/types";

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
  it("总量与日均按实际条数算，柱子留真实本数（高度由渲染层的线性刻度算）", () => {
    const model = buildImportTrend(trend([0, 0, 3, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0]));
    expect(model.data.total).toBe(9);
    // 只有 2 天有导入时不说「日均」，否则 838 本挤一天也会读成「日均 60 本」
    expect(model.note).toBe("近 14 天导入 9 本，只有 2 天有导入");
    // 0 值留在数据里（渲染层画 1px 基线），不再归一成百分比：归一化正是「0 值日完全不画」的根因
    expect(model.data.bars.map((b) => b.value)).toEqual([0, 0, 3, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0]);
    expect(model.data.bars[6].title).toContain("导入 6 本");
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

  it("每天一条日期标签数据，抽稀交给渲染层（chartScale 的 labelStride / labelVisible）", () => {
    const model = buildImportTrend(trend(new Array(14).fill(1)));
    expect(model.data.bars.map((b) => b.label)).toEqual([
      "10/1", "10/2", "10/3", "10/4", "10/5", "10/6", "10/7",
      "10/8", "10/9", "10/10", "10/11", "10/12", "10/13", "10/14",
    ]);
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
    expect(model.data.rows[0].value).toBe(12_000_000);
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
  it("占比极小的一段按真实比例画，图例写「<1%」而不是放大到 1.2%", () => {
    const model = buildStorage({ ...baseStats, textBytes: 96_819_177, indexBytes: 134_477, otherBytes: 229_620_426 });
    const index = model.data.parts.find((p) => p.key === "index");
    // 0.04% 就是 0.04%：段宽不再抬到 1.2%（那是虚报 30 倍），要说的那句由文案承担
    expect(index?.sharePct).toBeLessThan(0.1);
    expect(index?.pctText).toBe("<1%");
    expect(index?.display).toBe("0.1 MB");
  });

  it("正常比例下三段按真实占比画，结论给出总量与正文占比", () => {
    const model = buildStorage(baseStats);
    const sum = model.data.parts.reduce((acc, p) => acc + p.sharePct, 0);
    expect(sum).toBeCloseTo(100, 6);
    expect(model.note).toContain("正文占");
    expect(model.data.parts.map((p) => p.label)).toEqual(["正文", "书目信息", "其他"]);
  });

  // 正文口径改成「压缩后字节」后，正文段会明显变大（占满全库），三段宽度仍然要落在 100% 上：
  // 既不能因为最小宽度放大而溢出，也不能因为取整而留下缝。
  it("压缩后的正文口径下，三段真实占比之和仍是 100（段宽就用真实占比）", () => {
    const model = buildStorage(realStats);
    expect(model.data.shareSum).toBeCloseTo(100, 6);
    for (const part of model.data.parts) {
      expect(part.sharePct).toBeGreaterThanOrEqual(0);
      expect(Number.isFinite(part.sharePct)).toBe(true);
    }
    // 正文段占绝大多数：图例文案按一位小数给，不是 100%
    expect(model.data.parts[0].pctText).toMatch(/^9\d(\.\d)?%$/);
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
      expect(part.sharePct).toBeGreaterThanOrEqual(0);
    }
    expect(model.data.parts[2].sharePct).toBe(0);
    expect(model.data.shareSum).toBeCloseTo(100, 6);
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

/* ── 推荐与埋点（驾驶舱新区块） ── */

const days = (rows: [number, number, number, number][]): RecommendStatsDay[] =>
  rows.map(([impressions, clicks, reads, seconds], index) => ({
    // 10-18 起连续若干天：与页面上的「近 14 天」同一口径
    date: `2026-10-${String(18 + index).padStart(2, "0")}`,
    impressions,
    clicks,
    reads,
    seconds,
  }));

const summary = (over: Partial<RecommendStatsSummary> = {}): RecommendStatsSummary => ({
  days: 14,
  impressions: 100,
  clicks: 10,
  reads: 4,
  readSeconds: 600,
  shelfAdds: 2,
  ...over,
});

const hot = (title: string, reads: number, clicks: number, seconds = 0): RecommendHotBook => ({
  bookId: title.length,
  title,
  authorName: "示例作者",
  reads,
  clicks,
  seconds,
});

describe("7 事件趋势", () => {
  it("三色给真实次数（不再各自归一），阅读面另给分钟", () => {
    const model = buildEventTrend(days([
      [100, 1, 0, 0],
      [40, 8, 2, 120],
    ]));
    // 归一化是实测缺陷的根因（116/30/130px 的三根柱不可比），现在只给真实值，量纲不同的分面画
    expect(model.data.days.map((d) => d.impressions)).toEqual([100, 40]);
    expect(model.data.days.map((d) => d.clicks)).toEqual([1, 8]);
    expect(model.data.days.map((d) => d.reads)).toEqual([0, 2]);
    expect(model.data.days.map((d) => d.minutes)).toEqual([0, 2]);
    // 浮层是绝对次数 + 当天点击率
    expect(model.data.days[1].title).toContain("曝光 40、点击 8、阅读 2");
    expect(model.data.days[1].title).toContain("读了 2 分钟");
    expect(model.data.days[1].title).toContain("点击率 20%");
    expect(model.data.days[0].title).toContain("点击率 1%");
  });

  it("结论由窗口合计实算（曝光 / 点击率 / 阅读分钟）", () => {
    const model = buildEventTrend(days([[100, 5, 1, 60], [300, 15, 3, 240]]));
    expect(model.data.impressions).toBe(400);
    expect(model.data.clicks).toBe(20);
    expect(model.data.ctr).toBe("5%");
    expect(model.data.readMinutes).toBe(5);
    expect(model.note).toContain("2 天曝光 400");
    expect(model.note).toContain("点击率 5%");
    expect(model.note).toContain("读了 5 分钟");
    // 结论里不许出现实现词（「索引」「bookId」这类）
    expect(findForbiddenTerms(model.note)).toEqual([]);
  });

  it("点击率不足 1% 写「<1%」，不写成 0%", () => {
    const model = buildEventTrend(days([[1000, 3, 0, 0]]));
    expect(model.data.ctr).toBe("<1%");
    expect(model.data.days[0].title).toContain("点击率 <1%");
  });

  it("没有事件（全 0）与没有日期都判空", () => {
    expect(buildEventTrend([]).empty).toBe(true);
    expect(buildEventTrend(days([[0, 0, 0, 0]])).empty).toBe(true);
    expect(buildEventTrend(days([[1, 0, 0, 0]])).empty).toBe(false);
  });

  it("日期按升序排：后端顺序变了图也不会倒着画", () => {
    const model = buildEventTrend([
      { date: "2026-10-20", impressions: 5, clicks: 0, reads: 0, seconds: 0 },
      { date: "2026-10-18", impressions: 1, clicks: 0, reads: 0, seconds: 0 },
    ]);
    expect(model.data.days.map((d) => d.date)).toEqual(["2026-10-18", "2026-10-20"]);
    expect(model.data.days[model.data.days.length - 1].label).toBe("10/20");
  });
});

describe("8 兴趣画像（大分类 / 小分类 / 作者 / 标签 / 系列五组）", () => {
  const interests: RecommendInterest[] = [
    { dim: "category_l1", key: "武侠", name: "武侠", weight: 69.4 },
    { dim: "category_l1", key: "科幻", name: "科幻", weight: 60.1 },
    { dim: "category_l2", key: "江湖", name: "江湖", weight: 69.4 },
    { dim: "category_l2", key: "太空", name: "太空", weight: 60.1 },
    { dim: "author", key: "9", name: "演示作者己", weight: 38.6 },
    { dim: "author", key: "7", name: "演示作者丁", weight: 37.1 },
    { dim: "category_l1", key: "悬疑", name: "悬疑", weight: 0 },
  ];

  /** 五维齐活：后端 interests 里 5 种 dim 各来一行（tag/series 的 name 与 key 相同）。 */
  const fiveDims: RecommendInterest[] = [
    ...interests,
    { dim: "tag", key: "武侠", name: "武侠", weight: 1.7 },
    { dim: "series", key: "示例系列", name: "示例系列", weight: 1.2 },
  ];

  it("五维按「大分类 → 小分类 → 作者 → 标签 → 系列」的固定顺序分开，各自组内归一", () => {
    const model = buildInterestBars(fiveDims);
    expect(model.data.groups.map((g) => g.dim)).toEqual([
      "category_l1",
      "category_l2",
      "author",
      "tag",
      "series",
    ]);
    expect(model.data.groups.map((g) => g.label)).toEqual(["分类·大", "分类·小", "作者", "标签", "系列"]);
    expect(model.data.groups[0].rows.map((r) => r.name)).toEqual(["武侠", "科幻"]);
    expect(model.data.groups[0].rows.map((r) => r.value)).toEqual([69.4, 60.1]);
    // 小分类自己归一：同名不同组的「武侠 / 江湖」不会再被当成同一行
    expect(model.data.groups[1].rows.map((r) => r.name)).toEqual(["江湖", "太空"]);
    expect(model.data.groups[1].rows[0].value).toBe(69.4);
    // 作者组也自己归一
    expect(model.data.groups[2].rows.map((r) => r.name)).toEqual(["演示作者己", "演示作者丁"]);
    expect(model.data.groups[2].rows[0].value).toBe(38.6);
  });

  it("同名出现在两组时不合并、浮层按组说清是大分类还是小分类", () => {
    const model = buildInterestBars([
      { dim: "category_l1", key: "武侠", name: "武侠", weight: 5 },
      { dim: "category_l2", key: "武侠", name: "武侠", weight: 5 },
    ]);
    expect(model.data.groups).toHaveLength(2);
    expect(model.data.groups[0].rows[0].title).toBe("大分类「武侠」：兴趣 5.0");
    expect(model.data.groups[1].rows[0].title).toBe("小分类「武侠」：兴趣 5.0");
  });

  it("空组整组不渲染：没有小分类兴趣时只出两组，不留空组图例", () => {
    const model = buildInterestBars([
      { dim: "category_l1", key: "武侠", name: "武侠", weight: 3 },
      { dim: "author", key: "9", name: "演示作者己", weight: 2 },
    ]);
    expect(model.data.groups.map((g) => g.dim)).toEqual(["category_l1", "author"]);
    expect(model.data.groups.map((g) => g.label)).toEqual(["分类·大", "作者"]);
    expect(model.data.groups.every((g) => g.rows.length > 0)).toBe(true);
    expect(buildInterestBars([]).empty).toBe(true);
  });

  it("标签 / 系列两维与分类同形：组标题是「标签」「系列」，名取 name ?? key", () => {
    const model = buildInterestBars([
      { dim: "tag", key: "武侠", name: "武侠", weight: 1.7 },
      { dim: "series", key: "示例系列", weight: 1.2 },
    ]);
    const tag = model.data.groups.find((g) => g.dim === "tag");
    const series = model.data.groups.find((g) => g.dim === "series");
    expect(tag?.label).toBe("标签");
    expect(tag?.rows.map((r) => r.name)).toEqual(["武侠"]);
    expect(series?.label).toBe("系列");
    // 系列这行没给 name，退回 key
    expect(series?.rows.map((r) => r.name)).toEqual(["示例系列"]);
  });

  it("标签 / 系列不套作者文案：浮层与结论里都不出现「作者」", () => {
    const model = buildInterestBars([
      { dim: "tag", key: "武侠", name: "武侠", weight: 5 },
      { dim: "series", key: "示例系列", name: "示例系列", weight: 4 },
    ]);
    const [tag, series] = model.data.groups;
    expect(tag.rows[0].title).toBe("标签「武侠」：兴趣 5.0");
    expect(series.rows[0].title).toBe("系列「示例系列」：兴趣 4.0");
    for (const row of model.data.groups.flatMap((g) => g.rows)) {
      expect(row.title).not.toContain("作者");
    }
    // 最重的是标签那行：结论要说是哪一组，不是「最重的是作者」
    expect(model.note).toBe("最重的是「武侠」（标签，兴趣 5.0）");
    expect(model.note).not.toContain("作者");
  });

  it("五维都有数据时五组都渲染，结论仍按最重的那一组念", () => {
    const model = buildInterestBars(fiveDims);
    expect(model.data.groups).toHaveLength(5);
    expect(model.empty).toBe(false);
    expect(model.data.top).toBe("武侠");
    expect(model.note).toBe("最重的是「武侠」（分类·大，兴趣 69.4）");
  });

  it("不认识的 dim（老后端的 category）一律忽略，不报错也不占组位", () => {
    const legacy = [
      { dim: "category", key: "武侠", name: "武侠", weight: 5 },
      { dim: "whatever", key: "x", weight: 1 },
    ] as unknown as RecommendInterest[];
    expect(buildInterestBars(legacy).empty).toBe(true);
  });

  it("作者组显示作者名（后端给的 name），不是 id", () => {
    const model = buildInterestBars(interests);
    const authorRow = model.data.groups[2].rows[0];
    expect(authorRow.name).toBe("演示作者己");
    expect(authorRow.title).toBe("作者 演示作者己：兴趣 38.6");
    expect(authorRow.title).not.toContain("编号");
  });

  it("name 缺省或空串时退回 key：旧后端 / 作者已删也不显示空白行", () => {
    const model = buildInterestBars([
      { dim: "author", key: "12", weight: 3 },
      { dim: "author", key: "7", name: "  ", weight: 2 },
      { dim: "category_l1", key: "武侠", weight: 1 },
    ]);
    const authors = model.data.groups.find((g) => g.dim === "author");
    expect(authors?.rows.map((r) => r.name)).toEqual(["12", "7"]);
    // 浮层同样用兜底后的名字，不会出现「作者 ：」
    expect(authors?.rows[0].title).toBe("作者 12：兴趣 3.0");
    expect(authors?.rows[1].title).toBe("作者 7：兴趣 2.0");
    expect(model.note).toContain("最重的是作者「12」");
  });

  it("权重 0 与空 key 不进榜（后端会留下衰减到 0 的行）", () => {
    const model = buildInterestBars(interests);
    expect(model.data.groups[0].rows.map((r) => r.name)).not.toContain("悬疑");
    expect(buildInterestBars([{ dim: "category_l1", key: "  ", weight: 9 }]).empty).toBe(true);
  });

  it("每组各取 top 5，结论里带上它属于哪一组", () => {
    const many: RecommendInterest[] = Array.from({ length: 7 }, (_, i) => ({
      dim: "category_l1",
      key: `类${i}`,
      name: `类${i}`,
      weight: 7 - i,
    }));
    const model = buildInterestBars(many);
    expect(model.data.groups[0].rows).toHaveLength(5);
    expect(model.data.groups[0].rows.map((r) => r.name)).toEqual(["类0", "类1", "类2", "类3", "类4"]);
    // 多组并排时「最重的」要说清在哪一组：同名分类可能同时出现在大 / 小分类里
    expect(model.note).toBe("最重的是「类0」（分类·大，兴趣 7.0）");
  });
});

describe("9 热门书", () => {
  it("读过 2 分 / 点击 1 分：读懂比点开值钱", () => {
    const model = buildHotBooks([
      hot("点开三次的书", 0, 3),
      hot("读完两次的书", 2, 0),
      hot("点开一次的书", 0, 1),
    ]);
    expect(HOT_READ_WEIGHT).toBe(2);
    // 2 次阅读（4 分）压过 3 次点击（3 分），1 次点击（1 分）垫底
    expect(model.data.rows.map((r) => r.name)).toEqual(["读完两次的书", "点开三次的书", "点开一次的书"]);
    expect(model.data.rows.map((r) => r.value)).toEqual([4, 3, 1]);
  });

  it("按分数排序而不是按输入顺序，且最多 10 本", () => {
    const books: RecommendHotBook[] = Array.from({ length: 12 }, (_, i) => hot(`书${i}`, 0, i + 1));
    const model = buildHotBooks(books);
    expect(model.data.rows).toHaveLength(10);
    expect(model.data.rows[0].name).toBe("书11");
    expect(model.data.rows[0].value).toBe(12);
    // 分母是这一榜的总热度：最大那本占 12 / (12+...+3) ≈ 15%
    expect(model.data.rows[0].title).toContain("热度占");
  });

  it("结论给「最热的是 X」并带读过 / 点开次数", () => {
    const model = buildHotBooks([hot("剑影山河", 3, 5, 600)]);
    expect(model.note).toContain("最热的是《剑影山河》");
    expect(model.note).toContain("3 次阅读、5 次点击");
    expect(model.data.rows[0].display).toBe("3 读 · 5 点");
    expect(model.data.rows[0].title).toContain("读了 10 分钟");
  });

  it("只有点击为 0 的书时判空（不至于画一条 0 分的榜）", () => {
    expect(buildHotBooks([hot("没人看的书", 0, 0)]).empty).toBe(true);
    expect(buildHotBooks([]).empty).toBe(true);
  });
});

describe("10 曝光到收藏的漏斗", () => {
  it("四级齐全，条长按曝光归一（渲染层算长度），右侧给级间转化率", () => {
    const model = buildFunnel(summary({ impressions: 200, clicks: 20, reads: 10, shelfAdds: 4 }));
    expect(model.data.steps.map((s) => s.label)).toEqual(["曝光", "点击", "阅读", "收藏"]);
    expect(model.data.steps.map((s) => s.value)).toEqual([200, 20, 10, 4]);
    expect(model.data.steps.map((s) => s.stepRate)).toEqual(["起始", "10%", "50%", "40%"]);
    expect(model.data.overall).toBe("2%");
    expect(model.note).toContain("整体转化 2%");
  });

  it("极小的一级按真实比例画（0.2% 就是 0.2%）；0 值那级留 1px 基线", () => {
    const model = buildFunnel(summary({ impressions: 10_000, clicks: 50, reads: 5, shelfAdds: 0 }));
    const shelf = model.data.steps[3];
    expect(shelf.value).toBe(0);
    expect(shelf.stepRate).toBe("0%");
    // 曝光 10000 只画 0.2% 的宽度：视觉下限由渲染层的 1px 基线表达，不靠放大条长
    expect(model.data.steps[3].value / model.data.steps[0].value).toBeCloseTo(0, 6);
  });

  it("下一级超过上一级时不出百分比，改成人均次数（阅读可以比点击多）", () => {
    // 本地实测：曝光 110 / 点击 1 / 阅读 128 / 收藏 0 —— 阅读这一级按点击算会是 12800%
    const model = buildFunnel(summary({ impressions: 110, clicks: 1, reads: 128, shelfAdds: 0 }));
    const [impression, click, read, shelf] = model.data.steps;
    expect(impression.stepRate).toBe("起始");
    // 1 / 110 = 0.9%：四舍五入到 1%（还不到 <1% 那一档）
    expect(click.stepRate).toBe("1%");
    expect(read.stepRate).toBe("每点击 128 次");
    // 浮层与格子共用同一句：两处都不该再出现百分比
    expect(read.stepRate).not.toContain("%");
    expect(read.title).not.toContain("%");
    expect(read.title).toContain("每点击 128 次");
    // 不超的那一级仍是转化率，0 收藏也仍是 0%
    expect(shelf.stepRate).toBe("0%");
    // 1.3 倍这种小数保留一位
    const mild = buildFunnel(summary({ impressions: 10, clicks: 2, reads: 3, shelfAdds: 0 }));
    expect(mild.data.steps[2].stepRate).toBe("每点击 1.5 次");
  });

  it("上一级是 0 而这一级有值时不编比例（除零），给「—」并在浮层里说明", () => {
    const model = buildFunnel(summary({ impressions: 0, clicks: 0, reads: 5, shelfAdds: 0 }));
    expect(model.data.steps[2].stepRate).toBe("—");
    expect(model.data.steps[2].title).toContain("阅读 5 次");
  });

  it("全 0 判空；只有曝光的漏斗不算空", () => {
    expect(buildFunnel(summary({ impressions: 0, clicks: 0, reads: 0, shelfAdds: 0 })).empty).toBe(true);
    expect(buildFunnel(summary({ impressions: 50, clicks: 0, reads: 0, shelfAdds: 0 })).empty).toBe(false);
  });
});

describe("指标卡行", () => {
  const today = "2026-10-19";

  it("今日曝光按 UTC 日期精确匹配，不是取数组最后一个", () => {
    const model = buildRecommendMetrics(summary(), days([[9, 0, 0, 0], [30, 0, 0, 0], [7, 0, 0, 0]]), "2026-10-19");
    // 18 / 19 / 20 三天，今日是 19 号
    expect(model.todayImpressions).toBe(30);
    expect(model.metrics[0]).toMatchObject({ key: "impressions", label: "今日曝光", value: "30" });
  });

  it("窗口里没有今天时退回最近一天，不显示凭空算出的 0", () => {
    const model = buildRecommendMetrics(summary(), days([[5, 0, 0, 0], [11, 0, 0, 0]]), today);
    expect(model.todayImpressions).toBe(11);
  });

  it("四个指标与图表同一份数字：点击率、阅读分钟、收藏", () => {
    const model = buildRecommendMetrics(
      summary({ impressions: 400, clicks: 20, reads: 6, readSeconds: 1500, shelfAdds: 3, days: 14 }),
      [days([[1, 0, 0, 0]])[0]],
      "2026-10-18",
    );
    expect(model.metrics.map((m) => m.key)).toEqual(["impressions", "ctr", "reads", "shelf"]);
    expect(model.metrics[1].value).toBe("5%");
    expect(model.metrics[2].value).toBe("25");
    expect(model.metrics[3].value).toBe("3");
    // 可见文案里不许出现实现词
    expect(findForbiddenTerms(model.metrics.map((m) => `${m.label}${m.value}${m.hint}`).join(""))).toEqual([]);
  });

  it("没有任何 daily 行时今日曝光是 0，不抛错", () => {
    expect(buildRecommendMetrics(summary({ impressions: 0 }), [], today).todayImpressions).toBe(0);
  });
});

/* ── 11 点击理由分布 / 12 分类点击分布（task-11 新增两张图） ── */

describe("11 点击理由分布", () => {
  it("按点击量排序、最多 10 条，条长对最高那条归一", () => {
    const model = buildReasonMix([
      { reason: "因为你常看武侠", impressions: 200, clicks: 12 },
      { reason: "最近很多人读", impressions: 40, clicks: 30 },
      { reason: "随机发现", impressions: 10, clicks: 3 },
    ]);
    expect(model.data.rows.map((r) => r.name)).toEqual(["最近很多人读", "因为你常看武侠", "随机发现"]);
    expect(model.data.rows.map((r) => r.value)).toEqual([30, 12, 3]);
    expect(model.data.totalClicks).toBe(45);
  });

  it("有曝光时浮层给点击率；只给点击时也不谎报", () => {
    const model = buildReasonMix([
      { reason: "因为你常看武侠", impressions: 200, clicks: 12 },
      { reason: "最近入库", clicks: 5 },
    ]);
    // 按点击量降序：武侠 12 在前，最近入库 5 在后
    expect(model.data.rows[0].title).toBe("「因为你常看武侠」：点击 12 / 曝光 200（点击率 6%）");
    expect(model.data.rows[1].title).toBe("「最近入库」：点击 5 次");
    // 结论只对「最抓人」那条给点击率（这条有曝光就写 6%）
    expect(model.note).toBe("最抓人的是「因为你常看武侠」：12 次点击（点击率 6%）");
  });

  it("点击率不足 1% 写「<1%」；0 点击 / 纯空白理由不进榜", () => {
    const model = buildReasonMix([
      { reason: "因为你常看科幻", impressions: 900, clicks: 4 },
      { reason: "零点击的理由", impressions: 30, clicks: 0 },
      { reason: "   ", impressions: 10, clicks: 2 },
    ]);
    expect(model.data.rows.map((r) => r.name)).toEqual(["因为你常看科幻"]);
    expect(model.data.rows[0].title).toContain("点击率 <1%");
    expect(buildReasonMix([]).empty).toBe(true);
  });
});

describe("12 分类点击分布", () => {
  it("按点击排序、最多 10 条，空分类归到「未分类」", () => {
    const model = buildCategoryClicks([
      { categoryL1: "武侠", clicks: 8 },
      { categoryL1: "", clicks: 20 },
      { categoryL1: "科幻", clicks: 4 },
    ]);
    expect(model.data.rows.map((r) => r.name)).toEqual(["未分类", "武侠", "科幻"]);
    expect(model.data.rows[0].value).toBe(20);
    expect(model.data.rows[0].title).toContain("占 63%");
    expect(model.note).toBe("点得最多的分类是「未分类」：20 次，占 63%");
  });

  it("0 点击与空数组都判空", () => {
    expect(buildCategoryClicks([{ categoryL1: "武侠", clicks: 0 }]).empty).toBe(true);
    expect(buildCategoryClicks([]).empty).toBe(true);
  });
});

/* ── 13 近 14 天阅读时长 / 14 导入累计（task-12 新增两张图） ── */

const daySeconds = (rows: [string, number][]): DaySeconds[] => rows.map(([date, seconds]) => ({ date, seconds }));

describe("13 近 14 天阅读时长", () => {
  it("按天出点，纵轴是分钟；0 值留在序列里，不跳过", () => {
    const model = buildReadingTrend(daySeconds([
      ["2026-11-01", 0],
      ["2026-11-02", 7200],
      ["2026-11-03", 180],
    ]));
    expect(model.data.points.map((p) => p.value)).toEqual([0, 120, 3]);
    expect(model.data.points.map((p) => p.label)).toEqual(["11/1", "11/2", "11/3"]);
    expect(model.data.activeDays).toBe(2);
    expect(model.data.totalSeconds).toBe(7380);
    expect(model.data.peak?.value).toBe(120);
  });

  it("只保留最近 N 天，且按日期升序（后端顺序变了也不倒着画）", () => {
    const rows: [string, number][] = Array.from({ length: 20 }, (_, i) => [
      `2026-11-${String(i + 1).padStart(2, "0")}`,
      (i + 1) * 60,
    ]);
    const model = buildReadingTrend(daySeconds([...rows].reverse()), 14);
    expect(model.data.points).toHaveLength(14);
    expect(model.data.points[0].label).toBe("11/7");
    expect(model.data.points[13].label).toBe("11/20");
  });

  it("一天都没读时判空，结论说「还没有阅读记录」", () => {
    const model = buildReadingTrend(daySeconds([["2026-11-01", 0]]));
    expect(model.empty).toBe(true);
    expect(model.note).toContain("还没有阅读记录");
  });

  it("结论给总时长、有记录天数与单日最长，且不含实现词", () => {
    const model = buildReadingTrend(daySeconds([["2026-11-02", 7200], ["2026-11-03", 3600]]));
    expect(model.note).toBe("近 2 天读了 3 小时，2 天有记录，单日最长 2 小时");
    expect(findForbiddenTerms(model.note)).toEqual([]);
    expect(model.data.points[0].title).toContain("读了 2 小时");
  });

  it("没有 days 时不崩，判空", () => {
    expect(buildReadingTrend([]).empty).toBe(true);
  });
});

describe("14 导入累计曲线", () => {
  it("逐日累加：曲线只增不减，斜率就是当天导入量", () => {
    const model = buildImportCumulative(trend([2, 0, 3, 0]));
    expect(model.data.points.map((p) => p.value)).toEqual([2, 2, 5, 5]);
    expect(model.data.total).toBe(5);
    expect(model.data.points[2].title).toContain("累计 5 本（当天 +3 本）");
    expect(model.data.points.map((p) => p.label)).toEqual(["10/1", "10/2", "10/3", "10/4"]);
  });

  it("结论给累计与最后一天新增", () => {
    expect(buildImportCumulative(trend([2, 0, 3, 1])).note).toBe("近 4 天累计导入 6 本，最后一天 +1 本");
  });

  it("全 0 与空数组都判空", () => {
    expect(buildImportCumulative(trend([0, 0])).empty).toBe(true);
    expect(buildImportCumulative([]).empty).toBe(true);
  });
});

describe("占比文案（堆叠条图例）", () => {
  it("非零但不足 1% 写「<1%」；10% 以上取整，以下留一位小数", () => {
    expect(sharePctText(0)).toBe("0%");
    expect(sharePctText(0.04)).toBe("<1%");
    expect(sharePctText(0.99)).toBe("<1%");
    expect(sharePctText(3.06)).toBe("3.1%");
    expect(sharePctText(96.9)).toBe("97%");
  });
});
