import { render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import {
  ChapterBucketsChart,
  ChapterLengthChart,
  EventTrendChart,
  FunnelChart,
  HotBooksChart,
  ImportCumulativeChart,
  ImportTrendChart,
  InterestBarsChart,
  ReadingDistChart,
  ReadingTrendChart,
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

/** 图体的宽度：jsdom 里量不到布局，useChartWidth 退回兜底 520，段宽之和必须正好等于它。 */
function svgWidth(container: HTMLElement): number {
  return Number(container.querySelector("svg")?.getAttribute("width") ?? 0);
}

/** 柱子的像素高度（SVG rect 的 height 属性）。 */
function rectHeight(element: Element | null): number {
  return Number(element?.getAttribute("height") ?? 0);
}

describe("库概览七张图：四态一致", () => {
  it("读取中：图体换成读取占位，不画半张图", () => {
    const { container } = render(<ImportTrendChart points={points} loading failed={false} onRetry={() => {}} />);
    expect(screen.getByTestId("chart:trend:loading")).toBeTruthy();
    // 图体与结论位各一处，说明「正在读」在两个位置都传达给用户
    expect(screen.getAllByText("正在读取…")).toHaveLength(2);
    expect(container.querySelectorAll("svg")).toHaveLength(0);
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
    expect(container.querySelectorAll("svg")).toHaveLength(0);
  });

  it("有数据：七张图都能渲染出图体（图体宽度铺满容器）", () => {
    const { container, unmount } = render(<ImportTrendChart points={points} {...idle} />);
    expect(container.querySelectorAll(".col-bar")).toHaveLength(14);
    expect(svgWidth(container)).toBeGreaterThan(0);
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
    expect(buckets.container.querySelectorAll(".col-bar")).toHaveLength(2);
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
    expect(authors.container.querySelectorAll(".c-bar")).toHaveLength(2);
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
    expect(len.container.querySelectorAll(".col-bar")).toHaveLength(2);
    len.unmount();

    render(<ImportCumulativeChart points={points} {...idle} />);
    expect(screen.getByTestId("chart:cumulative:figure")).toBeTruthy();
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

  it("每张图的结论都由数据实算（不是固定文案）", () => {
    const { unmount, container } = render(<ImportTrendChart points={points} {...idle} />);
    // 只有最后一天有数据：说「只有 1 天有导入」，而不是把 12 本摊成「日均 1 本」
    expect(screen.getByText("近 14 天导入 12 本，只有 1 天有导入")).toBeTruthy();
    // 0 值那天画 1px 基线（不是不画）：13 天没有导入也要能看出「这天是 0」
    expect(container.querySelectorAll("[data-zero-baseline]")).toHaveLength(13);
    expect(rectHeight(container.querySelector(".col-bar:not(.is-zero)"))).toBeGreaterThan(20);
    unmount();

    render(<StorageChart stats={stats} {...idle} />);
    expect(screen.getByText(/全库约 .*其中正文占 \d+%/)).toBeTruthy();
  });
});

describe("库体积构成：真实占比 + 铺满图体", () => {
  it("三段段宽之和 = 图体宽度（铺满，不溢出不留缝）", () => {
    const { container } = render(<StorageChart stats={stats} {...idle} />);
    const segs = [...container.querySelectorAll(".stack-seg")];
    expect(segs).toHaveLength(3);
    const total = svgWidth(container);
    expect(total).toBeGreaterThan(0);
    const sum = segs.reduce((acc, seg) => acc + Number(seg.getAttribute("width")), 0);
    expect(Math.abs(sum - total)).toBeLessThan(0.5);
    for (const seg of segs) {
      expect(Number(seg.getAttribute("width"))).toBeGreaterThanOrEqual(0);
    }
  });

  it("段宽 = 真实占比：正文段占九成以上，不因为其他段太小而压缩它", () => {
    const { container } = render(<StorageChart stats={compressedStats} {...idle} />);
    // otherBytes 为 0 的段不画（0 宽度的矩形是纯噪音），图例也不列
    const segs = [...container.querySelectorAll(".stack-seg")];
    expect(segs).toHaveLength(2);
    const total = svgWidth(container);
    const textShare = Number(segs[0].getAttribute("width")) / total;
    // 1,690,357,569 / 1,743,683,584 = 96.9%
    expect(textShare).toBeGreaterThan(0.96);
    expect(textShare).toBeLessThan(0.98);
  });

  it("占比不足 1% 的段在图例写「<1%」，段宽不被放大", () => {
    const { container } = render(<StorageChart stats={{ ...stats, otherBytes: 8272 }} {...idle} />);
    const segs = [...container.querySelectorAll(".stack-seg")];
    const total = svgWidth(container);
    const other = Number(segs[2].getAttribute("width"));
    // 8 KB / 326 MB = 0.0025%：按真实比例画就是不到半个像素（原实现会抬到 1.2% ≈ 4px）
    expect(other / total).toBeLessThan(0.0001);
    const card = container.querySelector('[data-testid="chart:storage"]') as HTMLElement;
    expect(card.textContent).toContain("其他 <1%");
  });

  it("压缩率写进同一行结论（同一张卡不重复正文体积）", () => {
    render(<StorageChart stats={compressedStats} {...idle} />);
    const note = screen.getByTestId("chart:storage").querySelector(".chart-note") as HTMLElement;
    // 实测 34.6763% → 一位小数 34.7%；取整会显示 35%，与真实压缩率对不上
    expect(note.textContent).toContain("正文约为原始文字的 34.7%，比不压缩省下 3.0 GB");
    expect(findForbiddenTerms(note.textContent ?? "")).toEqual([]);
    expect(findForbiddenTerms(screen.getByTestId("chart:storage").textContent ?? "")).toEqual([]);
  });

  it("后端没给原始字节时不编造压缩率（图照常出）", () => {
    const { container } = render(<StorageChart stats={{ ...compressedStats, textBytesRaw: undefined }} {...idle} />);
    const note = screen.getByTestId("chart:storage").querySelector(".chart-note") as HTMLElement;
    expect(note.textContent).not.toContain("压缩");
    // 「其他」在这里是 0 字节：不画 0 宽度的段
    expect(container.querySelectorAll(".stack-seg")).toHaveLength(2);
  });

  it("正文还在统计（textBytes=0）时也说不出压缩率，且三段仍然铺满", () => {
    const { container } = render(
      <StorageChart
        stats={{ ...compressedStats, textBytes: 0, indexBytes: 0, otherBytes: 1_743_683_584 }}
        {...idle}
      />,
    );
    const note = screen.getByTestId("chart:storage").querySelector(".chart-note") as HTMLElement;
    expect(note.textContent).not.toContain("压缩");
    const segs = [...container.querySelectorAll(".stack-seg")];
    const sum = segs.reduce((acc, seg) => acc + Number(seg.getAttribute("width")), 0);
    expect(Math.abs(sum - svgWidth(container))).toBeLessThan(0.5);
  });
});

/* ── 推荐与埋点：六张图（同一套 ChartFrame 四态） ── */

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

describe("推荐与埋点六张图：四态一致", () => {
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
    const cases: [string, ReactElement][] = [
      ["events", <EventTrendChart daily={[]} {...idle} />],
      ["interest", <InterestBarsChart interests={[]} {...idle} />],
      ["hot", <HotBooksChart books={[]} {...idle} />],
      [
        "funnel",
        <FunnelChart summary={{ ...recSummary, impressions: 0, clicks: 0, reads: 0, shelfAdds: 0 }} {...idle} />,
      ],
    ];
    for (const [id, element] of cases) {
      const { container, unmount } = render(element);
      expect(screen.getByTestId(`chart:${id}:empty`)).toBeTruthy();
      expect(container.querySelectorAll("svg")).toHaveLength(0);
      unmount();
    }
  });

  it("事件趋势：上下两个 mini 面（曝光/点击 与 阅读分钟），各自独立刻度", () => {
    const { container } = render(<EventTrendChart daily={recDays} {...idle} />);
    // 两天 × 三根柱，0 值不画；两面共用一条 x 轴
    expect(container.querySelectorAll("[data-facet]")).toHaveLength(2);
    expect(container.querySelectorAll(".col-bar")).toHaveLength(6);
    // 整列可聚焦：两天两条读数
    expect(container.querySelectorAll('[data-tip="1"]')).toHaveLength(2);
    // 两面各自有 0 基线与刻度（不是三色归一后共用一把尺子）
    expect(container.querySelectorAll(".c-axis").length).toBeGreaterThanOrEqual(2);
    expect(container.textContent).toContain("曝光");
    expect(container.textContent).toContain("点击");
    expect(container.textContent).toContain("分钟");
    // 两面的绘图区左边界一致：柱子落在同一列上
    const axes = [...container.querySelectorAll(".c-axis")];
    expect(axes[0].getAttribute("x1")).toBe(axes[1].getAttribute("x1"));
  });

  it("兴趣画像：按 dim 分组渲染，作者组显示名字而不是编号", () => {
    const interest = render(<InterestBarsChart interests={recInterests} {...idle} />);
    const groups = interest.container.querySelectorAll('[data-testid^="ibars-"]');
    expect(groups).toHaveLength(3);
    expect(interest.container.querySelectorAll(".c-bar")).toHaveLength(3);
    for (const label of ["分类·大", "分类·小", "作者"]) {
      expect(interest.container.textContent).toContain(label);
    }
    expect(interest.container.textContent).toContain("演示作者甲");
    expect(interest.container.querySelector('[data-testid="ibars-author"]')?.getAttribute("data-dim")).toBe("author");
    interest.unmount();
  });

  it("热门书：条尾给「读过 · 点开」，第一本用主色高亮", () => {
    const hot = render(<HotBooksChart books={recBooks} {...idle} />);
    expect(hot.container.querySelectorAll(".c-bar")).toHaveLength(2);
    expect(hot.container.textContent).toContain("3 读 · 5 点");
    expect(hot.container.querySelector(".c-bar")?.getAttribute("fill")).toBe("var(--accent)");
    hot.unmount();
  });

  it("漏斗：四级各一条，右侧给级间转化率；0 值那级留 1px 基线", () => {
    const funnel = render(<FunnelChart summary={recSummary} {...idle} />);
    expect(funnel.container.querySelectorAll(".c-bar")).toHaveLength(4);
    // 转化率写在每级右侧：曝光→点击 9%、阅读→收藏 50%
    expect(funnel.container.textContent).toContain("9%");
    expect(funnel.container.textContent).toContain("50%");
    funnel.unmount();

    const zero = render(<FunnelChart summary={{ ...recSummary, shelfAdds: 0 }} {...idle} />);
    expect(zero.container.querySelectorAll("[data-zero-baseline]")).toHaveLength(1);
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
    expect(container.querySelectorAll('[data-testid^="ibars-"]')).toHaveLength(5);
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
    expect(container.querySelectorAll('[data-testid^="ibars-"]')).toHaveLength(1);
    expect(container.querySelector('[data-testid="ibars-author"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="ibars-category_l1"]')).toBeNull();
  });
});

describe("新增两张时间序列图", () => {
  it("近 14 天阅读时长：14 个点、0 值也画（贴基线），纵轴是分钟", () => {
    const days = Array.from({ length: 16 }, (_, i) => ({
      date: `2026-11-${String(i + 1).padStart(2, "0")}`,
      seconds: i % 3 === 0 ? 3600 : 0,
    }));
    const { container } = render(<ReadingTrendChart days={days} {...idle} />);
    // 只取最近 14 天
    expect(container.querySelectorAll(".c-dot")).toHaveLength(14);
    expect(container.querySelectorAll('[data-tip="1"]')).toHaveLength(14);
    expect(container.textContent).toContain("阅读时长 · 分钟");
    expect(screen.getByTestId("chart:reading-trend").querySelector(".chart-note")?.textContent).toContain(
      "近 14 天读了",
    );
  });

  it("导入累计：曲线只增不减，最后一个点等于窗口总量", () => {
    const { container } = render(<ImportCumulativeChart points={points} {...idle} />);
    const dots = [...container.querySelectorAll(".c-dot")];
    expect(dots).toHaveLength(14);
    const ys = dots.map((dot) => Number(dot.getAttribute("cy")));
    // 累计曲线：y 只会往上（值只增不减），最后一个点是最高的一根
    for (let i = 1; i < ys.length; i += 1) {
      expect(ys[i]).toBeLessThanOrEqual(ys[i - 1] + 0.001);
    }
    expect(screen.getByTestId("chart:cumulative").querySelector(".chart-note")?.textContent).toContain(
      "累计导入 12 本",
    );
  });

  it("阅读时长为空时说「还没有阅读记录」，不画空轴", () => {
    render(<ReadingTrendChart days={[]} {...idle} />);
    expect(screen.getByTestId("chart:reading-trend:empty")).toBeTruthy();
  });
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
