import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ImportTrendPoint, Stats } from "../../api/types";
import {
  ChapterBucketsChart,
  ChapterLengthChart,
  ImportTrendChart,
  ReadingDistChart,
  StorageChart,
  TopAuthorsChart,
  type ChartState,
} from "./Charts";

const idle: ChartState = { loading: false, failed: false, onRetry: () => {} };

const points: ImportTrendPoint[] = [
  { date: "2026-10-01", books: 0, chapters: 0 },
  { date: "2026-10-02", books: 0, chapters: 0 },
  { date: "2026-10-03", books: 5, chapters: 50 },
];

const MB = 1024 * 1024;
/** 三段 60% / 30% / 10%：读数里的百分比由真实字节算出。 */
const stats: Stats = {
  books: 2,
  chapters: 10,
  authors: 1,
  words: 200,
  dbBytes: 1000 * MB,
  lastImportAt: null,
  textBytes: 600 * MB,
  indexBytes: 300 * MB,
  otherBytes: 100 * MB,
};

/** 取卡片里第一个浮层读数的文案（每张图各有一层，同时最多一层有内容）。 */
function tipText(container: HTMLElement): string | null {
  const tip = container.querySelector('[data-testid="chart-tip"]');
  return tip?.textContent ?? null;
}

describe("图表读数：悬停与键盘焦点都能读出数值", () => {
  it("导入趋势：整列可聚焦，零值那天也说明「没有导入」", () => {
    const { container } = render(<ImportTrendChart points={points} {...idle} />);
    const cols = [...container.querySelectorAll<HTMLElement>(".bar-col")];
    expect(cols).toHaveLength(3);
    expect(cols[0].getAttribute("tabindex")).toBe("0");
    expect(cols[0].getAttribute("aria-label")).toBe("10 月 1 日：没有导入");
    expect(cols[2].getAttribute("aria-label")).toBe("10 月 3 日：导入 5 本、50 章");
  });

  it("聚焦 = 悬停：键盘 Tab 到数据点就浮出同一句读数", () => {
    const { container } = render(<ImportTrendChart points={points} {...idle} />);
    const last = [...container.querySelectorAll<HTMLElement>(".bar-col")][2];
    fireEvent.focus(last);
    expect(tipText(container)).toBe("10 月 3 日：导入 5 本、50 章");
    fireEvent.blur(last);
    expect(tipText(container)).toBeNull();
  });

  it("滚动不收起读数：跟着锚点走（否则 Tab 到屏幕外的数据点时浮层会被那次滚动关掉）", () => {
    const { container } = render(<ImportTrendChart points={points} {...idle} />);
    const last = [...container.querySelectorAll<HTMLElement>(".bar-col")][2];
    fireEvent.focus(last);
    expect(tipText(container)).toBe("10 月 3 日：导入 5 本、50 章");
    fireEvent.scroll(window);
    expect(tipText(container)).toBe("10 月 3 日：导入 5 本、50 章");
  });

  it("章节数分布：每桶可读，空桶不假装有数", () => {
    const { container } = render(
      <ChapterBucketsChart
        buckets={[
          { label: "1", count: 0 },
          { label: "2-9", count: 3 },
        ]}
        {...idle}
      />,
    );
    const cols = [...container.querySelectorAll<HTMLElement>(".bar-col")];
    expect(cols[0].getAttribute("aria-label")).toBe("1 章：没有这样的书");
    expect(cols[1].getAttribute("aria-label")).toBe("2-9 章：3 本");
    fireEvent.mouseEnter(cols[1]);
    expect(tipText(container)).toBe("2-9 章：3 本");
  });

  it("Top 作者：读数带字数与占全库比例", () => {
    const { container } = render(
      <TopAuthorsChart
        authors={[
          { name: "甲", wordCount: 100 },
          { name: "乙", wordCount: 60 },
        ]}
        totalWords={200}
        {...idle}
      />,
    );
    const rows = [...container.querySelectorAll<HTMLElement>(".hbar-row")];
    expect(rows[0].getAttribute("aria-label")).toBe("甲：100 字，占全库 50%");
    fireEvent.focus(rows[1]);
    expect(tipText(container)).toBe("乙：60 字，占全库 30%");
  });

  it("阅读三态：每段可读，带本数与占比", () => {
    const { container } = render(
      <ReadingDistChart
        progress={[]}
        totalBooks={19}
        limitReached={false}
        buckets={{ finished: 5, reading: 4, unread: 10 }}
        {...idle}
      />,
    );
    const segs = [...container.querySelectorAll<SVGCircleElement>(".donut-seg")];
    expect(segs.length).toBe(3);
    expect(segs[0].getAttribute("aria-label")).toBe("已读完 5 本，占全库 26%");
    fireEvent.focus(segs[0]);
    expect(tipText(container)).toBe("已读完 5 本，占全库 26%");
  });

  it("占比不足 1% 时写「<1」，不四舍五入成 0%（真实库：在读 2 本 / 838 本）", () => {
    const { container } = render(
      <ReadingDistChart
        progress={[]}
        totalBooks={838}
        limitReached={false}
        buckets={{ finished: 5, reading: 2, unread: 831 }}
        {...idle}
      />,
    );
    const segs = [...container.querySelectorAll<SVGCircleElement>(".donut-seg")];
    expect(segs[1].getAttribute("aria-label")).toBe("在读 2 本，占全库 <1%");
  });

  it("库体积构成：三段各自可读", () => {
    const { container } = render(<StorageChart stats={stats} {...idle} />);
    const segs = [...container.querySelectorAll<HTMLElement>(".stack-seg")];
    expect(segs[0].getAttribute("aria-label")).toBe("正文 600 MB，占 60.0%");
    fireEvent.mouseEnter(segs[1]);
    expect(tipText(container)).toBe("书目信息 300 MB，占 30.0%");
  });

  it("章节长度分布：每档可读，带章数与占比", () => {
    const { container } = render(
      <ChapterLengthChart
        buckets={[
          { label: "<1千", count: 10 },
          { label: "1千-3千", count: 30 },
        ]}
        {...idle}
      />,
    );
    const cols = [...container.querySelectorAll<HTMLElement>(".col-item")];
    expect(cols[0].getAttribute("aria-label")).toBe("<1千：10 章，占 25%");
    fireEvent.mouseEnter(cols[1]);
    expect(tipText(container)).toBe("1千-3千：30 章，占 75%");
  });

  it("空图不渲染读数浮层（没有数据点可指）", () => {
    const { container } = render(<TopAuthorsChart authors={[]} totalWords={0} {...idle} />);
    expect(container.querySelector('[data-testid="chart-tip"]')).toBeNull();
  });
});
