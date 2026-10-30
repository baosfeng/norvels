import { render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReadingHistoryRow, ReadingProgress } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const readingHistory = vi.fn();
const readingProgress = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      readingHistory: (params: unknown, opts?: unknown) => readingHistory(params, opts),
      readingProgress: (opts?: unknown, limit?: unknown) => readingProgress(opts, limit),
    },
  };
});

const { HistoryPage } = await import("./HistoryPage");

/**
 * 以「本机今天零点」为基准构造时间，而不是「N 小时前」。
 * 分组口径是日历日（见 historyGroups.ts），所以「1 小时前」在凌晨 0–1 点会落到昨天，
 * 用相对小时写用例会在跨零点后随机变红——用例要的是「今天的那一条」，不是「一小时内的那一条」。
 */
function dayStart(daysAgo = 0): number {
  const now = new Date();
  return new Date(now.getFullYear(), now.getMonth(), now.getDate() - daysAgo).getTime();
}

/** 今天第 h 小时（h 可以大于当前时刻，分组只看日历日，不看是否已过去）。 */
function todayAt(h: number): string {
  return new Date(dayStart() + h * 3600 * 1000).toISOString();
}

/** 昨天 / 更早的第 h 小时。 */
function daysAgoAt(days: number, h: number): string {
  return new Date(dayStart(days) + h * 3600 * 1000).toISOString();
}

function row(over: Partial<ReadingHistoryRow> = {}): ReadingHistoryRow {
  return {
    bookId: 1,
    bookTitle: "示例书",
    chapterId: 11,
    chapterTitle: "第十二章 归途",
    lastReadAt: todayAt(2),
    totalSeconds: 4800,
    days: 3,
    ...over,
  };
}

function progress(over: Partial<ReadingProgress> = {}): ReadingProgress {
  return {
    bookId: 1,
    bookTitle: "示例书",
    chapterId: 11,
    chapterIdx: 10,
    chapterTitle: "第十二章 归途",
    percent: 0.456,
    updatedAt: todayAt(2),
    ...over,
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
  readingProgress.mockReset();
  readingProgress.mockResolvedValue({ items: [] });
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

describe("阅读历史：分组 / 单条结构 / 骨架", () => {
  it("H5 按今天 / 昨天 / 更早分组，组头出现在各自组内", async () => {
    readingHistory.mockResolvedValue({
      items: [
        row({ bookId: 1, bookTitle: "今天读的", lastReadAt: todayAt(1) }),
        row({ bookId: 2, bookTitle: "昨天读的", lastReadAt: daysAgoAt(1, 1) }),
        row({ bookId: 3, bookTitle: "前天读的", lastReadAt: daysAgoAt(2, 1) }),
      ],
      total: 3,
    });
    renderHistory("h5");

    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    const today = screen.getByTestId("history-group-today");
    const yesterday = screen.getByTestId("history-group-yesterday");
    const earlier = screen.getByTestId("history-group-earlier");
    expect(within(today).getByText("今天")).toBeTruthy();
    expect(within(yesterday).getByText("昨天")).toBeTruthy();
    expect(within(earlier).getByText("更早")).toBeTruthy();
    // 书落在对的组里
    expect(within(today).getByText("今天读的")).toBeTruthy();
    expect(within(yesterday).getByText("昨天读的")).toBeTruthy();
    expect(within(earlier).getByText("前天读的")).toBeTruthy();
  });

  it("只有一组的日期时不渲染空组", async () => {
    readingHistory.mockResolvedValue({ items: [row({ lastReadAt: todayAt(1) })], total: 1 });
    renderHistory("h5");
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    expect(screen.queryByTestId("history-group-yesterday")).toBeNull();
    expect(screen.queryByTestId("history-group-earlier")).toBeNull();
  });

  it("单条写清「读到 第 N 章」、三项带标签的时间，不再是三个裸数字并排", async () => {
    // 这条验的是「相对时间文案」，所以这里必须真的是 2 小时前——
    // 用 todayAt(2) 会在凌晨 0–2 点变成未来时刻，页面改显示日期，和用例本意无关。
    readingHistory.mockResolvedValue({
      items: [row({ lastReadAt: new Date(Date.now() - 2 * 3600 * 1000).toISOString() })],
      total: 1,
    });
    renderHistory("h5");
    const card = await screen.findByTestId("history-row-1");
    expect(card.textContent).toContain("读到 第十二章 归途");
    expect(card.textContent).toContain("累计 1 小时 20 分");
    expect(card.textContent).toContain("读了 3 天");
    expect(card.textContent).toContain("最后 2 小时前");
  });

  it("有阅读进度时显示百分比，没有时不显示 0%", async () => {
    readingHistory.mockResolvedValue({ items: [row({ bookId: 1 }), row({ bookId: 2, bookTitle: "没进度" })], total: 2 });
    readingProgress.mockResolvedValue({ items: [progress({ bookId: 1, percent: 0.456 })] });
    renderHistory("h5");

    const withPercent = await screen.findByTestId("history-row-1");
    await waitFor(() => expect(withPercent.textContent).toContain("46%"));
    expect(screen.getByTestId("history-row-2").textContent).not.toContain("0%");
  });

  it("PC 表格也分组，五列信息 + 操作列表头 + 每本书一行", async () => {
    readingHistory.mockResolvedValue({
      items: [row({ bookId: 1, lastReadAt: todayAt(1) }), row({ bookId: 2, lastReadAt: daysAgoAt(2, 2) })],
      total: 2,
    });
    renderHistory("pc");
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    // 五列信息 + 一个操作列（行内删除）：PC 也有删除入口，不因窗口变宽消失
    expect(document.querySelectorAll("thead th")).toHaveLength(6);
    // 两组各一个 tbody（表头也是 rowgroup，所以按 tbody 数）
    expect(document.querySelectorAll("tbody")).toHaveLength(2);
    expect(screen.getByTestId("history-row-1")).toBeTruthy();
    expect(screen.getByTestId("history-row-2")).toBeTruthy();
    expect(document.querySelector(".history-card")).toBeNull();
  });

  it("加载中给骨架屏而不是转圈", async () => {
    readingHistory.mockReturnValue(new Promise(() => {}));
    renderHistory("h5");
    expect(await screen.findByTestId("history-skeleton")).toBeTruthy();
    expect(document.querySelector(".spinner")).toBeNull();
  });

  it("空态：去书架挑一本的按钮可点", async () => {
    readingHistory.mockResolvedValue({ items: [], total: 0 });
    renderHistory("h5");
    const empty = await screen.findByTestId("history-empty");
    const link = within(empty).getByRole("link", { name: "去书架挑一本" }) as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/shelf");
    // H5 触控目标：按钮类元素在 globals 里抬到 44px，这里守住它带着 btn 类
    expect(link.className).toContain("btn");
  });
});
