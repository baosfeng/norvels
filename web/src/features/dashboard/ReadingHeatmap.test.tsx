import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ReadingStats } from "../../api/types";
import { ReadingHeatmapCard } from "./ReadingHeatmap";

function stats(over: Partial<ReadingStats> = {}): ReadingStats {
  return {
    totalSeconds: 36_000,
    todaySeconds: 600,
    weekSeconds: 5400,
    streak: 3,
    days: [
      { date: "2026-10-19", seconds: 0 },
      { date: "2026-10-20", seconds: 600 },
      { date: "2026-10-21", seconds: 7200 },
    ],
    ...over,
  };
}

function renderCard(over: Partial<ReadingStats> = {}, state: Partial<{ loading: boolean; failed: boolean }> = {}) {
  const onRetry = vi.fn();
  render(
    <ReadingHeatmapCard
      stats={stats(over)}
      loading={state.loading ?? false}
      failed={state.failed ?? false}
      onRetry={onRetry}
    />,
  );
  return onRetry;
}

describe("驾驶舱阅读时长卡", () => {
  it("四项数值：今日 / 本周 / 累计 / 连续天数", () => {
    renderCard();
    expect(screen.getByTestId("reading-time-today").textContent).toBe("10 分");
    expect(screen.getByTestId("reading-time-week").textContent).toBe("1 小时 30 分");
    expect(screen.getByTestId("reading-time-total").textContent).toBe("10 小时");
    expect(screen.getByTestId("reading-time-streak").textContent).toBe("3 天");
  });

  it("热力图按天出格子，档位跟着秒数走", () => {
    renderCard();
    expect(screen.getByTestId("heat-cell-2026-10-19").getAttribute("data-level")).toBe("0");
    expect(screen.getByTestId("heat-cell-2026-10-20").getAttribute("data-level")).toBe("1");
    expect(screen.getByTestId("heat-cell-2026-10-21").getAttribute("data-level")).toBe("4");
    // 格子带可读文案（悬浮与读屏共用）
    expect(screen.getByTestId("heat-cell-2026-10-20").getAttribute("aria-label")).toBe("10 月 20 日 · 10 分钟");
  });

  it("四项数值 2×2 排，数值本身不换行（原 81px 一格会把「2 小时 4 分」折成两行）", () => {
    renderCard({ totalSeconds: 7440 });
    const figures = screen.getByTestId("chart:reading-time:figures");
    expect(figures.querySelectorAll(".rt-figure")).toHaveLength(4);
    expect(screen.getByTestId("reading-time-total").textContent).toBe("2 小时 4 分");
    for (const testId of ["reading-time-today", "reading-time-week", "reading-time-total", "reading-time-streak"]) {
      expect(screen.getByTestId(testId).textContent).not.toContain("\n");
    }
  });

  it("周几标签与格子共用同一张网格：标签占 1 / 3 / 5 行，格子覆盖 1–7 行（严格对齐）", () => {
    renderCard();
    const grid = screen.getByTestId("chart:reading-time:grid");
    const labels = [...grid.querySelectorAll(".heat-wd")];
    expect(labels.map((el) => el.textContent)).toEqual(["一", "三", "五"]);
    // 行高就是格子边长：标签与被点亮的格子落在同一行上（原先两列各自 flex，会错开半个行距）
    expect(grid.style.gridAutoRows).toBe("12px");
    for (const label of labels) {
      expect((label as HTMLElement).style.gridColumn).toBe("1");
    }
    expect(labels.map((label) => (label as HTMLElement).style.gridRow)).toEqual(["1", "3", "5"]);
    const cells = [...grid.querySelectorAll(".heat-cell")];
    expect(cells.length).toBeGreaterThan(0);
    for (const cell of cells) {
      const gridRow = Number((cell as HTMLElement).style.gridRow);
      const gridColumn = Number((cell as HTMLElement).style.gridColumn);
      expect(gridRow).toBeGreaterThanOrEqual(1);
      expect(gridRow).toBeLessThanOrEqual(7);
      expect(gridColumn).toBeGreaterThanOrEqual(2);
    }
  });

  it("一天都没读时给空态，不画 90 个空格子", () => {
    renderCard({ days: [{ date: "2026-10-19", seconds: 0 }] });
    expect(screen.getByTestId("chart:reading-time:empty")).toBeTruthy();
    expect(screen.queryByTestId("chart:reading-time:grid")).toBeNull();
    expect(screen.getByTestId("chart:reading-time:note").textContent).toContain("近 90 天还没有阅读记录");
  });

  it("读取中与失败态", () => {
    renderCard({}, { loading: true });
    expect(screen.getByTestId("chart:reading-time:loading")).toBeTruthy();
  });

  it("失败态可以重试", () => {
    const onRetry = renderCard({}, { failed: true });
    const button = screen.getByRole("button", { name: "重试" });
    button.click();
    expect(onRetry).toHaveBeenCalled();
  });
});
