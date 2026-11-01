import { render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import {
  ChapterBucketsChart,
  ChapterLengthChart,
  EventTrendChart,
  FunnelChart,
  HotBooksChart,
  ImportTrendChart,
  InterestBarsChart,
  ReadingDistChart,
  StorageChart,
  TopAuthorsChart,
  type ChartState,
} from "./Charts";
import type {
  ImportTrendPoint,
  ReadingProgress,
  RecommendHotBook,
  RecommendInterest,
  RecommendStatsDay,
  RecommendStatsSummary,
  Stats,
} from "../../api/types";
import { findForbiddenTerms } from "../../lib/audit";

const idle: ChartState = { loading: false, failed: false, onRetry: () => {} };

const points: ImportTrendPoint[] = Array.from({ length: 14 }, (_, i) => ({
  date: `2026-10-${String(i + 1).padStart(2, "0")}`,
  books: i === 13 ? 12 : 0,
  chapters: i === 13 ? 1200 : 0,
}));

const stats: Stats = {
  books: 19,
  chapters: 11668,
  authors: 17,
  words: 30418864,
  dbBytes: 326553600,
  lastImportAt: "2026-10-24T20:25:26.409Z",
  // 正文：压缩后的实际占用；textBytesRaw：压缩前的原始文字字节
  textBytes: 101563175,
  textBytesRaw: 300000000,
  indexBytes: 73533895,
  otherBytes: 151456530,
};

/**
 * 真实库实测（838 本 / 42.27 万章，块压缩迁移 + VACUUM 之后），字节全部取自库里的实际值。
 * 口头常说的「正文 1.69 GB」是十进制 GB，这里按 1024 进制显示为 1.6 GB，是同一个数。
 */
const compressedStats: Stats = {
  ...stats,
  books: 838,
  chapters: 422715,
  dbBytes: 1_743_683_584,
  textBytes: 1_690_357_569,
  textBytesRaw: 4_874_681_744,
  indexBytes: 53_326_015,
  otherBytes: 0,
};

const progress: ReadingProgress[] = [
  {
    bookId: 1,
    bookTitle: "书一",
    chapterId: 2,
    chapterIdx: 1,
    chapterTitle: "第二章",
    percent: 0.2,
    updatedAt: "2026-10-24T12:00:00.000Z",
  },
];

describe("六张图都有四态", () => {
  it("读取中：图体换成读取占位，结论位也说明正在读", () => {
    const { container } = render(<ImportTrendChart points={points} loading failed={false} onRetry={() => {}} />);
    expect(screen.getByTestId("chart:trend:loading")).toBeTruthy();
    // 图体与结论位各一处，说明「正在读」在两张位置都传达给用户
    expect(screen.getAllByText("正在读取…")).toHaveLength(2);
    expect(container.querySelectorAll(".bar-col")).toHaveLength(0);
  });

  it("失败：给出可点重试，点了会调 onRetry", () => {
    const onRetry = vi.fn();
    render(<ChapterBucketsChart buckets={[]} loading={false} failed onRetry={onRetry} />);
    expect(screen.getByTestId("chart:buckets:error")).toBeTruthy();
    screen.getByRole("button", { name: "重试" }).click();
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("空数据：图体换成暂无数据，不画半张图", () => {
    const { container } = render(<TopAuthorsChart authors={[]} totalWords={0} {...idle} />);
    expect(screen.getByTestId("chart:authors:empty")).toBeTruthy();
    expect(container.querySelectorAll(".hbar-row")).toHaveLength(0);
  });

  it("有数据：六张图都能渲染出图体", () => {
    const { container, unmount } = render(<ImportTrendChart points={points} {...idle} />);
    expect(container.querySelectorAll(".bar-col")).toHaveLength(14);
    unmount();

    const buckets = render(
      <ChapterBucketsChart
        buckets={[
          { label: "1", count: 0 },
          { label: "2-9", count: 3 },
        ]}
        {...idle}
      />,
    );
    expect(buckets.container.querySelectorAll(".bar-col")).toHaveLength(2);
    buckets.unmount();

    const authors = render(
      <TopAuthorsChart
        authors={[
          { name: "甲", wordCount: 100 },
          { name: "乙", wordCount: 60 },
        ]}
        totalWords={200}
        {...idle}
      />,
    );
    expect(authors.container.querySelectorAll(".hbar-row")).toHaveLength(2);
    authors.unmount();

    const reading = render(
      <ReadingDistChart progress={progress} totalBooks={4} limitReached={false} {...idle} />,
    );
    expect(reading.container.querySelectorAll(".donut circle").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/本$/).length).toBeGreaterThanOrEqual(3);
    reading.unmount();

    const storage = render(<StorageChart stats={stats} {...idle} />);
    expect(storage.container.querySelectorAll(".stack-seg")).toHaveLength(3);
    storage.unmount();

    const len = render(
      <ChapterLengthChart
        buckets={[
          { label: "<1千字", count: 10 },
          { label: "1千-5千", count: 40 },
        ]}
        {...idle}
      />,
    );
    expect(len.container.querySelectorAll(".col-item")).toHaveLength(2);
    expect(len.container.querySelectorAll(".col-fill.is-peak")).toHaveLength(1);
  });

  it("后端统计刷新中（stale）：照常出图，只提示「数据更新中」", () => {
    const { container } = render(<StorageChart stats={stats} loading={false} failed={false} onRetry={() => {}} stale />);
    expect(screen.getByText("数据更新中")).toBeTruthy();
    expect(container.querySelectorAll(".stack-seg")).toHaveLength(3);
  });

  it("正常情况不显示「数据更新中」", () => {
    render(<StorageChart stats={stats} {...idle} />);
    expect(screen.queryByText("数据更新中")).toBeNull();
  });

  it("占比不足 0.1% 的段不出图例（避免一行「其他 0.0 MB」的噪音）", () => {
    const { container } = render(
      <StorageChart stats={{ ...stats, otherBytes: 8272 }} {...idle} />,
    );
    const legend = container.querySelector(".stack-legend") as HTMLElement;
    expect(legend.textContent).toContain("正文");
    expect(legend.textContent).toContain("书目信息");
    expect(legend.textContent).not.toContain("其他");
  });

  it("每张图的结论都由数据实算（不是固定文案）", () => {
    const { unmount, container } = render(<ImportTrendChart points={points} {...idle} />);
    // 只有最后一天有数据：说「只有 1 天有导入」，而不是把 12 本摊成「日均 1 本」
    expect(screen.getByText("近 14 天导入 12 本，只有 1 天有导入")).toBeTruthy();
    // 0 值的 13 天不画柱，只留最后一天
    expect(container.querySelectorAll(".bar-fill")).toHaveLength(1);
    unmount();

    render(<StorageChart stats={stats} {...idle} />);
    expect(screen.getByText(/全库约 .*其中正文占 \d+%/)).toBeTruthy();
  });
});

describe("库体积构成：正文改成压缩后口径", () => {
  it("三段宽度之和 = 100%（堆叠条铺满，不溢出不留缝）", () => {
    const { container } = render(<StorageChart stats={compressedStats} {...idle} />);
    const segs = [...container.querySelectorAll<HTMLElement>(".stack-seg")];
    expect(segs).toHaveLength(3);
    const sum = segs.reduce((acc, seg) => acc + Number.parseFloat(seg.style.width), 0);
    // 写进 DOM 的百分比相加必须正好是 100（残差补在最后一段上），不是「大约 100」
    expect(Math.round(sum * 100) / 100).toBe(100);
    // 每段都不为负、也不超过 100
    for (const seg of segs) {
      const width = Number.parseFloat(seg.style.width);
      expect(width).toBeGreaterThanOrEqual(0);
      expect(width).toBeLessThanOrEqual(100);
    }
  });

  it("正文压缩率用一句人话写出来，且不含实现词", () => {
    render(<StorageChart stats={compressedStats} {...idle} />);
    const detail = screen.getByTestId("chart:storage:detail");
    // 实测 34.6763% → 一位小数 34.7%；取整会显示 35%，与真实压缩率对不上
    expect(detail.textContent).toBe("正文约为原始文字的 34.7%，比不压缩省下 3.0 GB");
    expect(findForbiddenTerms(detail.textContent ?? "")).toEqual([]);
    // 同一张卡片里不重复正文体积：图例已经有一份
    const legend = document.querySelector(".stack-legend") as HTMLElement;
    expect(legend.textContent).toContain("正文 1.6 GB");
    expect(detail.textContent).not.toContain("1.6 GB");
    // 卡片全部可见文案都不出现实现词
    const card = document.querySelector('[data-testid="chart:storage"]') as HTMLElement;
    expect(findForbiddenTerms(card.textContent ?? "")).toEqual([]);
  });

  it("后端没给原始字节时不编造压缩率（只少一行说明，图照常出）", () => {
    const { container } = render(<StorageChart stats={{ ...compressedStats, textBytesRaw: undefined }} {...idle} />);
    expect(screen.queryByTestId("chart:storage:detail")).toBeNull();
    expect(container.querySelectorAll(".stack-seg")).toHaveLength(3);
  });

  it("正文还在统计（textBytes=0）时也说不出压缩率，且三段仍然铺满", () => {
    const { container } = render(
      <StorageChart
        stats={{ ...compressedStats, textBytes: 0, indexBytes: 0, otherBytes: 1_743_683_584 }}
        {...idle}
      />,
    );
    expect(screen.queryByTestId("chart:storage:detail")).toBeNull();
    const segs = [...container.querySelectorAll<HTMLElement>(".stack-seg")];
    const sum = segs.reduce((acc, seg) => acc + Number.parseFloat(seg.style.width), 0);
    expect(Math.round(sum * 100) / 100).toBe(100);
  });
});

/* ── 推荐与埋点：四张新图（同一套 ChartFrame 四态） ── */

const recDays: RecommendStatsDay[] = [
  { date: "2026-10-18", impressions: 120, clicks: 6, reads: 2, seconds: 240 },
  { date: "2026-10-19", impressions: 80, clicks: 12, reads: 4, seconds: 600 },
];

const recSummary: RecommendStatsSummary = {
  days: 14,
  impressions: 200,
  clicks: 18,
  reads: 6,
  readSeconds: 840,
  shelfAdds: 3,
};

const recInterests: RecommendInterest[] = [
  { dim: "category_l1", key: "武侠", name: "武侠", weight: 4 },
  { dim: "category_l2", key: "江湖", name: "江湖", weight: 4 },
  { dim: "author", key: "12", name: "演示作者甲", weight: 3 },
];

const recBooks: RecommendHotBook[] = [
  { bookId: 1, title: "剑影山河", authorName: "示例作者甲", reads: 3, clicks: 5, seconds: 600 },
  { bookId: 2, title: "星海拾遗", authorName: "示例作者丙", reads: 1, clicks: 2, seconds: 120 },
];

describe("推荐与埋点四张图：四态一致", () => {
  it("读取中：图体换成读取占位，不画半张图", () => {
    const cases = [
      <EventTrendChart key="events" daily={recDays} loading failed={false} onRetry={() => {}} />,
      <InterestBarsChart key="interest" interests={recInterests} loading failed={false} onRetry={() => {}} />,
      <HotBooksChart key="hot" books={recBooks} loading failed={false} onRetry={() => {}} />,
      <FunnelChart key="funnel" summary={recSummary} loading failed={false} onRetry={() => {}} />,
    ];
    for (const element of cases) {
      const { container, unmount } = render(element);
      const id = (container.querySelector(".chart-card") as HTMLElement).dataset.chart;
      expect(screen.getByTestId(`chart:${id}:loading`)).toBeTruthy();
      unmount();
    }
  });

  it("失败：四张图各自给可点重试", () => {
    const onRetry = vi.fn();
    const { unmount } = render(<EventTrendChart daily={recDays} loading={false} failed onRetry={onRetry} />);
    screen.getByTestId("chart:events:error");
    screen.getByRole("button", { name: "重试" }).click();
    expect(onRetry).toHaveBeenCalledOnce();
    unmount();

    render(<FunnelChart summary={recSummary} loading={false} failed onRetry={onRetry} />);
    expect(screen.getByTestId("chart:funnel:error")).toBeTruthy();
  });

  it("空：没有事件时四张图都说「暂无数据」，不画空轴", () => {
    const cases: [string, ReactElement, number][] = [
      [
        "events",
        <EventTrendChart daily={[]} {...idle} />,
        0,
      ],
      [
        "interest",
        <InterestBarsChart interests={[]} {...idle} />,
        0,
      ],
      [
        "hot",
        <HotBooksChart books={[]} {...idle} />,
        0,
      ],
      [
        "funnel",
        <FunnelChart summary={{ ...recSummary, impressions: 0, clicks: 0, reads: 0, shelfAdds: 0 }} {...idle} />,
        0,
      ],
    ];
    for (const [id, element, count] of cases) {
      const { container, unmount } = render(element);
      expect(screen.getByTestId(`chart:${id}:empty`)).toBeTruthy();
      expect(container.querySelectorAll(".etrend-col, .hbar-row, .funnel-row")).toHaveLength(count);
      unmount();
    }
  });

  it("有数据：四张图都画出图体，且读数可聚焦（键盘也能看浮层）", () => {
    const { container, unmount } = render(<EventTrendChart daily={recDays} {...idle} />);
    // 两天 × 三根柱，但 0 值不画：这里两天都有三种事件
    expect(container.querySelectorAll(".etrend-col")).toHaveLength(2);
    expect(container.querySelectorAll(".etrend-bar")).toHaveLength(6);
    expect(container.querySelectorAll('[data-tip="1"]')).toHaveLength(2);
    unmount();

    const interest = render(<InterestBarsChart interests={recInterests} {...idle} />);
    // 按 dim 分组渲染：这里三组有数据就画三组（大分类 / 小分类 / 作者），各一行
    expect(interest.container.querySelectorAll(".ibars-group")).toHaveLength(3);
    expect(interest.container.querySelectorAll(".hbar-row")).toHaveLength(3);
    for (const label of ["分类·大", "分类·小", "作者"]) {
      expect(interest.container.textContent).toContain(label);
    }
    // 作者组显示后端给的作者名，不是编号
    expect(interest.container.textContent).toContain("演示作者甲");
    // 组上带着 dim，窄屏/宽度规则按组取（作者名比分类名长）
    expect(interest.container.querySelector('[data-testid="ibars-author"]')?.getAttribute("data-dim")).toBe("author");
    interest.unmount();

    const hot = render(<HotBooksChart books={recBooks} {...idle} />);
    expect(hot.container.querySelectorAll(".hbar-row")).toHaveLength(2);
    hot.unmount();

    const funnel = render(<FunnelChart summary={recSummary} {...idle} />);
    expect(funnel.container.querySelectorAll(".funnel-row")).toHaveLength(4);
    // 转化率写在每级右侧：曝光→点击、点击→阅读、阅读→收藏
    expect(funnel.container.textContent).toContain("9%");
    expect(funnel.container.textContent).toContain("50%");
  });
});


  it("五维都有数据时五组都画出来：标签 / 系列不套「作者」文案", () => {
    const { container } = render(
      <InterestBarsChart
        interests={[
          ...recInterests,
          { dim: "tag", key: "武侠", name: "武侠", weight: 2 },
          { dim: "series", key: "示例系列", name: "示例系列", weight: 1 },
        ]}
        {...idle}
      />,
    );
    expect(container.querySelectorAll(".ibars-group")).toHaveLength(5);
    for (const dim of ["category_l1", "category_l2", "author", "tag", "series"]) {
      expect(container.querySelector(`[data-testid="ibars-${dim}"]`)?.getAttribute("data-dim")).toBe(dim);
    }
    // 组标题「标签」「系列」都在，且浮层文案里没有「作者 武侠」这种错误归属
    expect(container.textContent).toContain("标签");
    expect(container.textContent).toContain("系列");
    const tagGroup = container.querySelector('[data-testid="ibars-tag"]');
    expect(tagGroup?.textContent).not.toContain("作者");
  });

  it("某一组没有数据时整组不渲染（三组不一定都出现）", () => {
    const { container } = render(
      <InterestBarsChart interests={[{ dim: "author", key: "12", name: "演示作者甲", weight: 3 }]} {...idle} />,
    );
    expect(container.querySelectorAll(".ibars-group")).toHaveLength(1);
    expect(container.querySelector('[data-testid="ibars-author"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="ibars-category_l1"]')).toBeNull();
  });

describe("推荐与埋点：可见文案不含实现词", () => {
  it("四张卡的正文与结论都过一遍禁用词表", () => {
    const { container } = render(
      <>
        <EventTrendChart daily={recDays} {...idle} />
        <InterestBarsChart interests={recInterests} {...idle} />
        <HotBooksChart books={recBooks} {...idle} />
        <FunnelChart summary={recSummary} {...idle} />
      </>,
    );
    for (const card of container.querySelectorAll(".chart-card")) {
      expect(findForbiddenTerms(card.textContent ?? "")).toEqual([]);
    }
  });
});
