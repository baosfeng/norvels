import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  ChapterBucketsChart,
  ChapterLengthChart,
  ImportTrendChart,
  ReadingDistChart,
  StorageChart,
  TopAuthorsChart,
  type ChartState,
} from "./Charts";
import type { ImportTrendPoint, ReadingProgress, Stats } from "../../api/types";
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