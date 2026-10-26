import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ReadingProgress } from "../../api/types";
import { ReadingDistChart, ImportTrendChart, type ChartState } from "./Charts";

const idle: ChartState = { loading: false, failed: false, onRetry: () => {} };

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

describe("C-26（新增）环形图图例过滤", () => {
  it("图例是按钮：点一下 aria-pressed 翻转、该段变淡，并说明总数不变", () => {
    const { container } = render(
      <ReadingDistChart
        progress={progress}
        totalBooks={4}
        limitReached={false}
        buckets={{ finished: 5, reading: 4, unread: 10 }}
        {...idle}
      />,
    );
    const legend = screen.getByTestId("legend-finished");
    expect(legend.getAttribute("data-od-id")).toBe("C-26");
    expect(legend.getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(legend);
    expect(screen.getByTestId("legend-finished").getAttribute("aria-pressed")).toBe("false");
    // 第一段就是「已读完」（finished → reading → unread 的固定顺序）
    const seg = container.querySelector(".donut-seg") as SVGCircleElement;
    expect(seg.getAttribute("opacity")).toBe("0.16");
    expect(screen.getByTestId("legend-hidden-note").textContent).toContain("总数不变");
    // 数字一字不动：过滤只改可见性
    expect(screen.getByTestId("legend-finished").textContent).toContain("已读完");

    fireEvent.click(screen.getByTestId("legend-finished"));
    expect(screen.getByTestId("legend-finished").getAttribute("aria-pressed")).toBe("true");
    expect(screen.queryByTestId("legend-hidden-note")).toBeNull();
  });

  it("三段各自一个可聚焦按钮（读屏仍能读到本数）", () => {
    render(<ReadingDistChart progress={progress} totalBooks={4} limitReached={false} {...idle} />);
    const buttons = document.querySelectorAll('[data-od-id="C-26"]');
    expect(buttons).toHaveLength(3);
    expect(buttons[0].getAttribute("aria-label")).toContain("本");
  });
});

describe("C-31（新增）图表卡重试", () => {
  it("失败态按钮挂 C-31，点后变「重试中…」并禁用、显示骨架块，最短 240ms 后还原", async () => {
    const onRetry = vi.fn();
    render(<ImportTrendChart points={[]} loading={false} failed onRetry={onRetry} />);
    const button = screen.getByRole("button", { name: "重试" });
    expect(button.getAttribute("data-od-id")).toBe("C-31");

    fireEvent.click(button);
    expect(onRetry).toHaveBeenCalledOnce();
    await waitFor(() => expect(screen.getByTestId("chart:trend:loading")).toBeTruthy());
    expect(screen.getByTestId("chart-retry-skeleton")).toBeTruthy();

    await waitFor(() => expect(screen.getByRole("button", { name: "重试" })).toBeTruthy(), { timeout: 2000 });
  });
});
