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
