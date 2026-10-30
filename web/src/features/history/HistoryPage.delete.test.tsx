import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { ReadingHistoryResult, ReadingHistoryRow } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const readingHistory = vi.fn();
const deleteReadingHistory = vi.fn();
const clearReadingHistory = vi.fn();
const showToast = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      readingHistory: (params: unknown, opts?: unknown) => readingHistory(params, opts),
      deleteReadingHistory: (id: number, opts?: unknown) => deleteReadingHistory(id, opts),
      clearReadingHistory: (opts?: unknown) => clearReadingHistory(opts),
    },
  };
});

vi.mock("../../lib/toastStore", async () => {
  const actual = await vi.importActual<typeof import("../../lib/toastStore")>("../../lib/toastStore");
  return { ...actual, showToast: (text: string, tone?: string) => showToast(text, tone) };
});

const { HistoryPage } = await import("./HistoryPage");

function row(over: Partial<ReadingHistoryRow> = {}): ReadingHistoryRow {
  return {
    bookId: 1,
    bookTitle: "示例书",
    chapterId: 11,
    chapterTitle: "第十二章 归途",
    lastReadAt: new Date(Date.now() - 3600 * 1000).toISOString(),
    totalSeconds: 600,
    days: 1,
    ...over,
  };
}

function result(from: number, count: number, total: number): ReadingHistoryResult {
  return {
    items: Array.from({ length: count }, (_, i) => row({ bookId: from + i, bookTitle: "示例书 " + (from + i) })),
    total,
  };
}

function renderHistory(tier: "pc" | "h5" = "h5") {
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
  deleteReadingHistory.mockReset();
  clearReadingHistory.mockReset();
  showToast.mockReset();
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

describe("阅读记录变更后的跨组件刷新", () => {
  // 回归：删掉阅读记录后，后端已经不算这段时间了，首页「今天已读」与驾驶舱时长卡
  // 却还挂着旧数字——删除只刷新了历史列表自己。
  it("删除成功后广播一次失效，别处的阅读统计会重取", async () => {
    const { readingRevision } = await import("../../lib/readingRevision");
    readingHistory.mockResolvedValue(result(1, 2, 2));
    deleteReadingHistory.mockResolvedValue(undefined);
    const before = readingRevision();

    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    fireEvent.click(screen.getByTestId("history-delete-1"));
    fireEvent.click(screen.getByTestId("history-confirm-1"));

    await waitFor(() => expect(readingRevision()).toBe(before + 1));
  });

  it("删除失败不广播（数据没变，别处不该白重取一次）", async () => {
    const { readingRevision } = await import("../../lib/readingRevision");
    readingHistory.mockResolvedValue(result(1, 2, 2));
    deleteReadingHistory.mockRejectedValue(new ApiError("network", "没能连上服务"));
    const before = readingRevision();

    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    fireEvent.click(screen.getByTestId("history-delete-1"));
    fireEvent.click(screen.getByTestId("history-confirm-1"));

    await waitFor(() => expect(showToast.mock.calls[0][1]).toBe("error"));
    expect(readingRevision()).toBe(before);
  });
});

describe("阅读历史：单条删除（H5 卡片）", () => {
  it("删除入口是链接之外的按钮，点开先要二次确认，确认前不发请求", async () => {
    readingHistory.mockResolvedValue(result(1, 2, 2));
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());

    const entry = screen.getByTestId("history-delete-1");
    expect(entry.closest("a")).toBeNull(); // 不能嵌在续读链接里，否则点删除会先跳走
    fireEvent.click(entry);

    expect(screen.getByTestId("history-confirm-1")).toBeTruthy();
    expect(deleteReadingHistory).not.toHaveBeenCalled();
  });

  it("确认后本地移除该行、total 减一（不等后端刷新）", async () => {
    readingHistory.mockResolvedValue(result(1, 3, 3));
    deleteReadingHistory.mockResolvedValue(undefined);
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("history-delete-2"));
    fireEvent.click(screen.getByTestId("history-confirm-2"));

    await waitFor(() => expect(deleteReadingHistory).toHaveBeenCalledWith(2, undefined));
    await waitFor(() => expect(screen.queryByTestId("history-row-2")).toBeNull());
    expect(screen.getByTestId("history-row-1")).toBeTruthy();
    expect(screen.getByTestId("history-row-3")).toBeTruthy();
    expect(screen.getByText(/2 本/)).toBeTruthy();
    expect(showToast).toHaveBeenCalledWith("已删除这条阅读记录。", undefined);
  });

  it("取消后行还在，不发请求", async () => {
    readingHistory.mockResolvedValue(result(1, 2, 2));
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("history-delete-1"));
    fireEvent.click(screen.getByTestId("history-cancel-1"));

    await waitFor(() => expect(screen.queryByTestId("history-confirm-1")).toBeNull());
    expect(screen.getByTestId("history-row-1")).toBeTruthy();
    expect(deleteReadingHistory).not.toHaveBeenCalled();
  });

  it("后端失败必须回滚：行回到列表，并给 error 提示", async () => {
    readingHistory.mockResolvedValue(result(1, 2, 2));
    deleteReadingHistory.mockRejectedValue(new ApiError("http", "服务开小差了", 500));
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("history-delete-1"));
    fireEvent.click(screen.getByTestId("history-confirm-1"));

    await waitFor(() => expect(showToast).toHaveBeenCalled());
    expect(showToast.mock.calls[0][1]).toBe("error");
    // 回滚：整份快照放回去，不是只加回这一行
    await waitFor(() => expect(screen.getByTestId("history-row-1")).toBeTruthy());
    expect(screen.getByTestId("history-row-2")).toBeTruthy();
  });

  it("删除中禁用其余入口（避免两次删除的乐观状态互相覆盖）", async () => {
    readingHistory.mockResolvedValue(result(1, 3, 3));
    let release: (v: undefined) => void = () => undefined;
    deleteReadingHistory.mockImplementation(() => new Promise<undefined>((resolve) => (release = resolve)));
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("history-delete-1"));
    fireEvent.click(screen.getByTestId("history-confirm-1"));

    // 被删的那行已乐观移除；剩下两行的入口在请求回来之前一律禁用
    await waitFor(() => expect(screen.queryByTestId("history-row-1")).toBeNull());
    expect((screen.getByTestId("history-delete-2") as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId("history-delete-3") as HTMLButtonElement).disabled).toBe(true);
    // 点禁用入口不会展开第二个确认，也就不会有并发删除
    fireEvent.click(screen.getByTestId("history-delete-2"));
    expect(screen.queryByTestId("history-confirm-2")).toBeNull();
    expect(deleteReadingHistory).toHaveBeenCalledTimes(1);

    release(undefined);
    await waitFor(() => expect((screen.getByTestId("history-delete-2") as HTMLButtonElement).disabled).toBe(false));
  });

  it("删除成功后以服务端新数据为准（乐观视图只是临时真相）", async () => {
    readingHistory
      .mockResolvedValueOnce(result(1, 2, 2))
      // 服务端删完后的真实列表：只剩 1 本（模拟别处也删了一条，本地猜不出来）
      .mockResolvedValueOnce({ items: [row({ bookId: 1, bookTitle: "示例书 1" })], total: 1 });
    deleteReadingHistory.mockResolvedValue(undefined);
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("history-delete-2"));
    fireEvent.click(screen.getByTestId("history-confirm-2"));

    await waitFor(() => expect(readingHistory).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.getByText(/1 本/)).toBeTruthy());
    expect(screen.getByTestId("history-row-1")).toBeTruthy();
    expect(screen.queryByTestId("history-row-2")).toBeNull();
  });

  it("404 not_found 按成功处理：移除该行、刷新列表，不弹错误", async () => {
    readingHistory
      .mockResolvedValueOnce(result(1, 2, 2))
      .mockResolvedValueOnce({ items: [row({ bookId: 1, bookTitle: "示例书 1" })], total: 1 });
    // 列表过时：后端说这本早就没有记录了
    deleteReadingHistory.mockRejectedValue(new ApiError("http", "记录不存在", 404, "not_found"));
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("history-delete-2"));
    fireEvent.click(screen.getByTestId("history-confirm-2"));

    await waitFor(() => expect(deleteReadingHistory).toHaveBeenCalledWith(2, undefined));
    // 不等后端：行先消失
    await waitFor(() => expect(screen.queryByTestId("history-row-2")).toBeNull());
    // 并且刷新到事实
    await waitFor(() => expect(readingHistory).toHaveBeenCalledTimes(2));
    // 不弹 error 提示
    const tones = showToast.mock.calls.map((c) => c[1]);
    expect(tones).not.toContain("error");
    expect(showToast.mock.calls[0][0]).toBe("这条记录已经不在了。");
    expect(screen.getByTestId("history-row-1")).toBeTruthy();
  });

  it("本页删空且不在第一页 → 回退一页重新取数", async () => {
    readingHistory.mockImplementation((params: { page?: number }) =>
      Promise.resolve(params.page === 2 ? result(21, 1, 21) : result(1, 20, 21)),
    );
    deleteReadingHistory.mockResolvedValue(undefined);
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    await waitFor(() => expect(readingHistory).toHaveBeenCalledTimes(2));

    fireEvent.click(screen.getByTestId("history-delete-21"));
    fireEvent.click(screen.getByTestId("history-confirm-21"));

    await waitFor(() => expect(readingHistory).toHaveBeenCalledTimes(3));
    expect(readingHistory.mock.calls[2][0]).toMatchObject({ page: 1 });
  });
});

describe("阅读历史：清空（页头入口，两档都有）", () => {
  it("清空前要二次确认，确认后列表变空态", async () => {
    readingHistory.mockResolvedValue(result(1, 2, 2));
    clearReadingHistory.mockResolvedValue(undefined);
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("history-clear"));
    expect(clearReadingHistory).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("history-clear-confirm"));

    await waitFor(() => expect(clearReadingHistory).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByTestId("history-empty")).toBeTruthy());
    expect(showToast).toHaveBeenCalledWith("已清空阅读历史。", undefined);
  });

  it("清空失败同样回滚，列表恢复", async () => {
    readingHistory.mockResolvedValue(result(1, 2, 2));
    clearReadingHistory.mockRejectedValue(new ApiError("network", "没能连上服务"));
    renderHistory();
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("history-clear"));
    fireEvent.click(screen.getByTestId("history-clear-confirm"));

    await waitFor(() => expect(showToast.mock.calls[0][1]).toBe("error"));
    await waitFor(() => expect(screen.getByTestId("history-row-1")).toBeTruthy());
  });

  it("PC 表格下也能清空（清空是整页动作，与承载形态无关）", async () => {
    readingHistory.mockResolvedValue(result(1, 2, 2));
    clearReadingHistory.mockResolvedValue(undefined);
    renderHistory("pc");
    await waitFor(() => expect(document.querySelector("table")).not.toBeNull());
    expect(screen.getByTestId("history-clear")).toBeTruthy();
    fireEvent.click(screen.getByTestId("history-clear"));
    fireEvent.click(screen.getByTestId("history-clear-confirm"));
    await waitFor(() => expect(clearReadingHistory).toHaveBeenCalledTimes(1));
  });

  // 同一份数据上的「删掉这条」不该因为窗口变宽就消失：PC 表格也有行内删除，
  // 且与 H5 卡片共用同一套二段式确认（testid 也是同一套）。
  it("PC 表格的行内删除：确认前不发请求，确认后调接口并移除该行", async () => {
    readingHistory.mockResolvedValue(result(1, 2, 2));
    deleteReadingHistory.mockResolvedValue(undefined);
    renderHistory("pc");
    await waitFor(() => expect(document.querySelector("table")).not.toBeNull());

    const entry = screen.getByTestId("history-delete-1");
    expect(entry.closest("a")).toBeNull();
    fireEvent.click(entry);
    expect(screen.getByTestId("history-confirm-1")).toBeTruthy();
    expect(deleteReadingHistory).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("history-confirm-1"));
    await waitFor(() => expect(deleteReadingHistory.mock.calls[0][0]).toBe(1));
    await waitFor(() => expect(screen.queryByTestId("history-row-1")).toBeNull());
  });
});
