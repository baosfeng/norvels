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
  textBytes: 101563175,
  indexBytes: 73533895,
  otherBytes: 151456530,
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

  it("每张图的结论都由数据实算（不是固定文案）", () => {
    const { unmount } = render(<ImportTrendChart points={points} {...idle} />);
    expect(screen.getByText("近 14 天导入 12 本，日均 1 本")).toBeTruthy();
    unmount();

    render(<StorageChart stats={stats} {...idle} />);
    expect(screen.getByText(/全库约 .*其中正文占 \d+%/)).toBeTruthy();
  });
});