import { describe, expect, it } from "vitest";
import type { BookSummary, DaySeconds, SeriesSummary } from "../api/types";
import { formatDuration, formatDurationShort } from "./format";
import {
  buildHeatmap,
  buildSeriesCards,
  heatLevel,
  seriesCardModel,
  sortSeriesBooks,
} from "./seriesGroup";

const NOW = Date.parse("2026-10-25T12:00:00Z");

function series(over: Partial<SeriesSummary> = {}): SeriesSummary {
  return {
    name: "示例系列",
    bookCount: 3,
    wordCount: 120000,
    lastUpdatedAt: "2026-10-24T12:00:00Z",
    coverBookId: 7,
    ...over,
  };
}

function book(over: Partial<BookSummary> = {}): BookSummary {
  return {
    id: 1,
    title: "示例书",
    authorId: 1,
    authorName: "示例作者",
    chapterCount: 10,
    wordCount: 1000,
    volumeCount: 1,
    intro: "",
    status: "parsed",
    importedAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
    ...over,
  };
}

describe("系列卡片文案", () => {
  it("正常项：书数与字数走中文量级，更新时间给人话", () => {
    const card = seriesCardModel(series(), NOW);
    expect(card.bookCountLabel).toBe("3 本");
    expect(card.wordCountLabel).toBe("12 万 字");
    expect(card.updatedLabel).toBe("1 天前");
    expect(card.hasCover).toBe(true);
    expect(card.empty).toBe(false);
  });

  it("边界：没有章节的系列显示「还没有章节」，coverBookId=0 不取封面", () => {
    const card = seriesCardModel(series({ lastUpdatedAt: "", coverBookId: 0, bookCount: 1 }), NOW);
    expect(card.updatedLabel).toBe("还没有章节");
    expect(card.hasCover).toBe(false);
    expect(card.coverBookId).toBe(0);
  });

  it("异常字段：负数与 NaN 一律按 0，不把 NaN 显示给用户", () => {
    const card = seriesCardModel(
      series({ bookCount: -2, wordCount: Number.NaN, coverBookId: Number.NaN }),
      NOW,
    );
    expect(card.bookCountLabel).toBe("0 本");
    expect(card.wordCountLabel).toBe("0 字");
    expect(card.hasCover).toBe(false);
  });

  it("整页整形：空列表给 empty，有数据时合计书数", () => {
    expect(buildSeriesCards([], NOW)).toEqual({ cards: [], empty: true, bookTotal: 0 });
    const model = buildSeriesCards([series(), series({ name: "另一个系列", bookCount: 2 })], NOW);
    expect(model.empty).toBe(false);
    expect(model.cards).toHaveLength(2);
    expect(model.bookTotal).toBe(5);
  });
});

describe("系列详情书目排序", () => {
  it("系列序号升序，没标序号的排最后", () => {
    const books = [
      book({ id: 3, title: "丙", seriesIndex: 0 }),
      book({ id: 2, title: "乙", seriesIndex: 2 }),
      book({ id: 1, title: "甲", seriesIndex: 1 }),
    ];
    expect(sortSeriesBooks(books).map((b) => b.id)).toEqual([1, 2, 3]);
  });

  it("同序号比章数再比书名，且不改动入参", () => {
    const books = [
      book({ id: 1, title: "乙", seriesIndex: 1, chapterCount: 5 }),
      book({ id: 2, title: "甲", seriesIndex: 1, chapterCount: 5 }),
      book({ id: 3, title: "丙", seriesIndex: 1, chapterCount: 9 }),
    ];
    const sorted = sortSeriesBooks(books);
    expect(sorted.map((b) => b.id)).toEqual([3, 2, 1]);
    expect(books.map((b) => b.id)).toEqual([1, 2, 3]);
  });

  it("空与缺字段不抛错", () => {
    expect(sortSeriesBooks([])).toEqual([]);
    expect(sortSeriesBooks([book({ seriesIndex: undefined })])).toHaveLength(1);
  });
});

describe("阅读热力图整形", () => {
  const days: DaySeconds[] = [
    { date: "2026-10-19", seconds: 0 }, // 周一
    { date: "2026-10-20", seconds: 600 },
    { date: "2026-10-21", seconds: 1200 },
    { date: "2026-10-22", seconds: 2400 },
    { date: "2026-10-23", seconds: 7200 },
    { date: "2026-10-24", seconds: -30 },
    { date: "2026-10-25", seconds: 300 },
  ];

  it("档位：0 / 15 分 / 30 分 / 60 分四道阈值", () => {
    expect(heatLevel(0)).toBe(0);
    expect(heatLevel(-5)).toBe(0);
    expect(heatLevel(899)).toBe(1);
    expect(heatLevel(900)).toBe(2);
    expect(heatLevel(1800)).toBe(3);
    expect(heatLevel(3600)).toBe(4);
    expect(heatLevel(Number.NaN)).toBe(0);
  });

  it("按周排成 7 行一列，首列按首日周几补空", () => {
    const model = buildHeatmap(days);
    // 2026-10-19 是周一：首列顶部补 1 个空格，7 天 + 1 空格跨两列
    expect(model.columns).toHaveLength(2);
    expect(model.columns[0][0]).toBeNull();
    expect(model.columns[0][1]?.date).toBe("2026-10-19");
    expect(model.columns[0][6]?.date).toBe("2026-10-24");
    // 第 7 天溢到第二列（周一格），其余补空
    expect(model.columns[1][0]?.date).toBe("2026-10-25");
    expect(model.columns[1][1]).toBeNull();
  });

  it("悬浮文案与统计：负数归零、没有阅读的天不计数", () => {
    const model = buildHeatmap(days);
    expect(model.days[0].label).toBe("10 月 19 日 · 没有阅读");
    expect(model.days[4].label).toBe("10 月 23 日 · 2 小时");
    expect(model.days[5].seconds).toBe(0);
    expect(model.activeDays).toBe(5);
    expect(model.maxSeconds).toBe(7200);
    expect(model.empty).toBe(false);
  });

  it("空数据与全零都给 empty", () => {
    expect(buildHeatmap([])).toEqual({
      columns: [],
      days: [],
      activeDays: 0,
      maxSeconds: 0,
      empty: true,
    });
    expect(buildHeatmap([{ date: "2026-10-25", seconds: 0 }]).empty).toBe(true);
  });

  it("坏日期不抛错（周几补到 0，标签保留原文）", () => {
    const model = buildHeatmap([{ date: "not-a-date", seconds: 60 }]);
    expect(model.days[0].weekday).toBe(0);
    expect(model.days[0].label).toBe("not-a-date · 1 分钟");
  });
});

describe("时长文案", () => {
  it("分钟级与小时级", () => {
    expect(formatDuration(0)).toBe("0 分钟");
    expect(formatDuration(30)).toBe("不到 1 分钟");
    expect(formatDuration(600)).toBe("10 分钟");
    expect(formatDuration(3600)).toBe("1 小时");
    expect(formatDuration(4800)).toBe("1 小时 20 分");
    expect(formatDuration(36_000)).toBe("10 小时");
  });

  it("短口径同取整", () => {
    expect(formatDurationShort(0)).toBe("0 分");
    expect(formatDurationShort(30)).toBe("<1 分");
    expect(formatDurationShort(600)).toBe("10 分");
    expect(formatDurationShort(4800)).toBe("1 小时 20 分");
  });
});
