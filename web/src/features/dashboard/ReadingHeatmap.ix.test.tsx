import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ReadingHeatmapCard } from "./ReadingHeatmap";

function renderCard() {
  return render(
    <ReadingHeatmapCard
      stats={{
        totalSeconds: 7800,
        todaySeconds: 0,
        weekSeconds: 0,
        streak: 2,
        days: [
          { date: "2026-10-19", seconds: 0 },
          { date: "2026-10-20", seconds: 600 },
          { date: "2026-10-21", seconds: 7200 },
        ],
      }}
      loading={false}
      failed={false}
      onRetry={vi.fn()}
    />,
  );
}

describe("C-30（新增）热力图每格可点、可读", () => {
  it("每格一个锚点、各自独立的 aria 文案", () => {
    renderCard();
    const cells = document.querySelectorAll('[data-od-id="C-30"]');
    expect(cells).toHaveLength(3);
    expect(cells[1].getAttribute("aria-label")).toBe("10 月 20 日 · 10 分钟");
    expect(cells[2].getAttribute("aria-label")).toBe("10 月 21 日 · 2 小时");
  });

  it("点一格浮出该格读数，Esc 收起", async () => {
    renderCard();
    const cell = screen.getByTestId("heat-cell-2026-10-21");
    fireEvent.click(cell);
    const tip = await screen.findByTestId("heat-tip");
    expect(tip.textContent).toBe("10 月 21 日 · 2 小时");
    expect(tip.getAttribute("role")).toBe("status");

    fireEvent.keyDown(cell, { key: "Escape" });
    expect(screen.queryByTestId("heat-tip")).toBeNull();
  });

  it("hover 立即浮出读数（不再等 120ms），移开立即消失", async () => {
    renderCard();
    const cell = screen.getByTestId("heat-cell-2026-10-20");
    fireEvent.mouseEnter(cell);
    // 契约改为即时：11px 的小格子上再等 120ms，快速划过时根本看不到浮层
    expect(screen.getByTestId("heat-tip").textContent).toBe("10 月 20 日 · 10 分钟");
    fireEvent.mouseLeave(cell);
    await waitFor(() => expect(screen.queryByTestId("heat-tip")).toBeNull());
  });

  it("换一格读数跟着换，不会停在上一格", () => {
    renderCard();
    fireEvent.mouseEnter(screen.getByTestId("heat-cell-2026-10-20"));
    expect(screen.getByTestId("heat-tip").textContent).toBe("10 月 20 日 · 10 分钟");
    fireEvent.mouseEnter(screen.getByTestId("heat-cell-2026-10-21"));
    expect(screen.getByTestId("heat-tip").textContent).toBe("10 月 21 日 · 2 小时");
  });

  it("点开固定后再移开不收起（5 秒或 Esc 才收）", () => {
    renderCard();
    const cell = screen.getByTestId("heat-cell-2026-10-21");
    fireEvent.click(cell);
    fireEvent.mouseLeave(cell);
    expect(screen.getByTestId("heat-tip").textContent).toBe("10 月 21 日 · 2 小时");
  });

  it("键盘聚焦也显示读数（与点击等效）", () => {
    renderCard();
    fireEvent.focus(screen.getByTestId("heat-cell-2026-10-20"));
    expect(screen.getByTestId("heat-tip").textContent).toBe("10 月 20 日 · 10 分钟");
  });

  it("91 天全为 0 时整块换成空态（不画可点格子）", () => {
    render(
      <ReadingHeatmapCard
        stats={{ totalSeconds: 0, todaySeconds: 0, weekSeconds: 0, streak: 0, days: [{ date: "2026-10-19", seconds: 0 }] }}
        loading={false}
        failed={false}
        onRetry={vi.fn()}
      />,
    );
    expect(screen.getByTestId("chart:reading-time:empty")).toBeTruthy();
    expect(document.querySelectorAll('[data-od-id="C-30"]')).toHaveLength(0);
  });
});
