import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import type { HealthReport } from "../../api/types";

vi.mock("../../api/hooks", () => ({
  useHealthIssues: () => ({ data: null, loading: false, error: null, reload: vi.fn() }),
}));

const { HealthCard } = await import("./HealthCard");

function report(over: Partial<HealthReport> = {}): HealthReport {
  return {
    totalBooks: 100,
    thinBooks: 0,
    missingAuthor: 0,
    missingIntro: 0,
    hugeChapters: 0,
    chapterBuckets: [],
    chapterLengthBuckets: [],
    warnings: [],
    ...over,
  };
}

function renderCard(over: Partial<HealthReport> = {}) {
  const onRetry = vi.fn();
  render(
    <MemoryRouter>
      <HealthCard report={report(over)} loading={false} error={null} onRetry={onRetry} lastImportAt="2026-10-24T20:25:26.409Z" />
    </MemoryRouter>,
  );
  return onRetry;
}

describe("健康卡入口", () => {
  it("四类问题都 0 时，四个入口都禁用，且各自一句不同的说明（不共用同一句话）", () => {
    renderCard();
    const texts = ["missing_author", "missing_intro", "thin_book", "huge_chapter"].map((kind) => {
      const entry = screen.getByTestId(`health-entry-${kind}`) as HTMLButtonElement;
      expect(entry.disabled).toBe(true);
      expect(entry.textContent).toBeTruthy();
      return entry.textContent ?? "";
    });
    expect(new Set(texts).size).toBe(4);
    expect(texts[0]).toBe("没有缺作者的书");
    expect(texts[3]).toBe("没有超长章节");
  });

  it("有问题时入口可点，文案是「查看 N 本/个」（章节级用「个」）", () => {
    renderCard({ thinBooks: 3, missingAuthor: 2, missingIntro: 7, hugeChapters: 5 });
    for (const [kind, text] of [
      ["thin_book", "查看 3 本"],
      ["missing_author", "查看 2 本"],
      ["missing_intro", "查看 7 本"],
      ["huge_chapter", "查看 5 个"],
    ] as const) {
      const entry = screen.getByTestId(`health-entry-${kind}`) as HTMLButtonElement;
      expect(entry.disabled).toBe(false);
      expect(entry.textContent).toContain(text);
    }
  });

  it("数字与口径按报告显示：章节完整显示「完整本数 / 总数」", () => {
    renderCard({ totalBooks: 838, thinBooks: 3 });
    expect(screen.getByRole("heading", { name: "库健康状况" }).closest(".dash-block")?.textContent).toContain(
      "835 / 838",
    );
  });

  it("点入口打开对应明细抽屉，并聚焦「关闭」", () => {
    renderCard({ hugeChapters: 5 });
    fireEvent.click(screen.getByTestId("health-entry-huge_chapter"));
    expect(screen.getByTestId("health-drawer").getAttribute("data-kind")).toBe("huge_chapter");
    expect(screen.getByTestId("health-drawer-title").textContent).toBe("超长章节");
    expect(document.activeElement).toBe(screen.getByTestId("health-drawer-close"));
  });

  it("关闭抽屉后焦点回到打开它的入口按钮", () => {
    renderCard({ hugeChapters: 5 });
    const trigger = screen.getByTestId("health-entry-huge_chapter") as HTMLButtonElement;
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTestId("health-drawer-close"));
    expect(document.activeElement).toBe(trigger);
  });
});
