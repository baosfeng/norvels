import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReadingHistoryResult, ReadingHistoryRow } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const readingHistory = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: { ...actual.api, readingHistory: (params: unknown, opts?: unknown) => readingHistory(params, opts) },
  };
});

const { HistoryPage } = await import("./HistoryPage");

function row(over: Partial<ReadingHistoryRow> = {}): ReadingHistoryRow {
  return {
    bookId: 1,
    bookTitle: "示例书",
    chapterId: 11,
    chapterTitle: "第十二章 归途",
    lastReadAt: new Date(Date.now() - 2 * 3600 * 1000).toISOString(),
    totalSeconds: 4800,
    days: 3,
    ...over,
  };
}

function result(from: number, count: number, total: number): ReadingHistoryResult {
  return {
    items: Array.from({ length: count }, (_, i) => row({ bookId: from + i, bookTitle: `示例书 ${from + i}` })),
    total,
  };
}

function renderHistory(tier: "pc" | "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter>
        <HistoryPage />
      </MemoryRouter>
    </TierProvider>,
  );
}

beforeEach(() => {
  readingHistory.mockReset();
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

describe("阅读历史：H5 用卡片列表（PC 仍是表格）", () => {
  it("H5 不渲染表格，用卡片承载同样的五项信息", async () => {
    readingHistory.mockResolvedValue(result(1, 2, 2));
    renderHistory("h5");

    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    expect(document.querySelector("table")).toBeNull();
    const card = screen.getByTestId("history-row-1");
    expect(card.className).toContain("history-card");
    // 封面 + 书名 + 最后读到第几章 + 最后阅读时间 + 累计时长 + 阅读天数
    expect(card.textContent).toContain("示例书 1");
    expect(card.textContent).toContain("第十二章 归途");
    expect(card.textContent).toContain("1 小时 20 分");
    expect(card.textContent).toContain("3 天");
    expect(card.querySelector(".cover, [data-testid='cover-placeholder']")).not.toBeNull();
  });

  it("H5 卡片整张可点，落到最后读到的那一章", async () => {
    readingHistory.mockResolvedValue(result(1, 1, 1));
    renderHistory("h5");
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    const link = screen.getByRole("link", { name: /示例书 1/ }) as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/read/1/11");
  });

  it("H5 也有分页，翻页重新取数", async () => {
    readingHistory.mockResolvedValue(result(1, 20, 45));
    renderHistory("h5");
    await waitFor(() => expect(screen.getByTestId("history-pagination")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    await waitFor(() => expect(readingHistory).toHaveBeenCalledTimes(2));
    expect(readingHistory.mock.calls[1][0]).toMatchObject({ page: 2, size: 20 });
  });

  it("H5 空态有「去书架挑一本」出口", async () => {
    readingHistory.mockResolvedValue({ items: [], total: 0 });
    renderHistory("h5");
    const empty = await screen.findByTestId("history-empty");
    expect(empty.textContent).toContain("还没有阅读记录");
    expect(screen.getByRole("link", { name: "去书架挑一本" }).getAttribute("href")).toBe("/shelf");
  });

  it("PC 仍是表格（一行一本，五列）", async () => {
    readingHistory.mockResolvedValue(result(1, 2, 2));
    renderHistory("pc");
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    expect(document.querySelector("table")).not.toBeNull();
    expect(document.querySelectorAll("thead th")).toHaveLength(5);
    expect(document.querySelector(".history-card")).toBeNull();
  });
});
