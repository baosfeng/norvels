import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReadingHistoryResult, ReadingHistoryRow } from "../../api/types";

const readingHistory = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      readingHistory: (params: unknown, opts?: unknown) => readingHistory(params, opts),
    },
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
  return { items: Array.from({ length: count }, (_, i) => row({ bookId: from + i, bookTitle: `示例书 ${from + i}` })), total };
}

function renderHistory() {
  return render(
    <MemoryRouter>
      <HistoryPage />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  readingHistory.mockReset();
});

describe("阅读历史页", () => {
  it("正常：一行一本书，展示最后章节 / 时间 / 累计时长 / 阅读天数", async () => {
    readingHistory.mockResolvedValue(result(1, 2, 2));
    renderHistory();

    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    expect(readingHistory.mock.calls[0][0]).toMatchObject({ page: 1, size: 20 });
    const first = screen.getByTestId("history-row-1");
    expect(first.textContent).toContain("示例书 1");
    expect(first.textContent).toContain("第十二章 归途");
    expect(first.textContent).toContain("1 小时 20 分");
    expect(first.textContent).toContain("3 天");
    expect(first.textContent).toContain("2 小时前");
  });

  it("点击书名进入阅读器，跳到最后读到的章", async () => {
    readingHistory.mockResolvedValue(result(1, 1, 1));
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    expect(screen.getByRole("link", { name: "示例书 1" }).getAttribute("href")).toBe("/read/1/11");
  });

  it("分页：翻页重新取数，页码边界不越界", async () => {
    readingHistory.mockResolvedValue(result(1, 20, 45));
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-pagination")).toBeTruthy());
    expect(screen.getByTestId("history-pagination").textContent).toContain("/ 3");

    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    await waitFor(() => expect(readingHistory).toHaveBeenCalledTimes(2));
    expect(readingHistory.mock.calls[1][0]).toMatchObject({ page: 2, size: 20 });
  });

  it("空态：没有任何阅读记录", async () => {
    readingHistory.mockResolvedValue({ items: [], total: 0 });
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-empty")).toBeTruthy());
    expect(screen.getByTestId("history-empty").textContent).toContain("还没有阅读记录");
  });

  it("错误态：失败后可重试", async () => {
    readingHistory.mockRejectedValueOnce(new Error("离线")).mockResolvedValueOnce(result(1, 1, 1));
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-error")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
  });
});
