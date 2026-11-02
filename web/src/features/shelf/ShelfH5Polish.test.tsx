import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookQuery, RequestOptions } from "../../api/client";
import type { BookSummary, Page, ReadingProgress } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const books = vi.fn<(params: BookQuery, opts?: RequestOptions) => Promise<Page<BookSummary>>>();
const readingProgress = vi.fn(async (): Promise<{ items: ReadingProgress[] }> => ({ items: [] }));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      books: (params: BookQuery, opts?: RequestOptions) => books(params, opts),
      readingProgress: () => readingProgress(),
      authors: vi.fn(async () => ({
        items: [{ id: 7, name: "示例作者", bookCount: 3 }],
        total: 1,
        page: 1,
        size: 20,
      })),
      tags: vi.fn(async () => ({ items: [{ id: 1, name: "示例标签", bookCount: 2 }] })),
      collections: vi.fn(async () => ({
        items: [{ id: 3, name: "示例书单", bookCount: 2 }],
        total: 1,
      })),
    },
  };
});

const { ShelfPage } = await import("./ShelfPage");

function bookAt(index: number, over: Partial<BookSummary> = {}): BookSummary {
  return {
    id: index,
    title: `示例书 ${index}`,
    authorId: 1,
    authorName: "示例作者",
    chapterCount: 10,
    wordCount: 12000,
    volumeCount: 1,
    intro: "",
    status: "parsed",
    importedAt: "2026-10-24T20:24:50.859Z",
    updatedAt: "2026-10-24T20:24:50.859Z",
    ...over,
  };
}

function pageOf(items: BookSummary[], total = items.length): Page<BookSummary> {
  return { items, total, page: 1, size: 30 };
}

function progressAt(bookId: number, over: Partial<ReadingProgress> = {}): ReadingProgress {
  return {
    bookId,
    bookTitle: `示例书 ${bookId}`,
    chapterId: 51,
    chapterIdx: 4,
    chapterTitle: "第五章 归途",
    percent: 0.42,
    updatedAt: "2026-10-24T20:24:50.859Z",
    ...over,
  };
}

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname + location.search}</div>;
}

function renderShelf(entries: string[] = ["/"]) {
  return render(
    <TierProvider value="h5">
      <MemoryRouter initialEntries={entries}>
        <ShelfPage />
        <LocationProbe />
      </MemoryRouter>
    </TierProvider>,
  );
}

function currentQuery(): string {
  return decodeURIComponent(screen.getByTestId("location").textContent ?? "");
}

beforeEach(() => {
  window.sessionStorage.clear();
  books.mockReset().mockResolvedValue(pageOf([bookAt(1)]));
  readingProgress.mockClear().mockResolvedValue({ items: [] });
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

describe("H5 书架：书卡阅读进度", () => {
  it("读过的书显示进度条与「第 N 章 / 共 M 章」，百分比跟在进度条那一行", async () => {
    readingProgress.mockResolvedValue({ items: [progressAt(1, { percent: 0.42, chapterIdx: 4 })] });
    renderShelf();

    const line = await screen.findByTestId("book-progress-1");
    // 章节口径与详情页 / 阅读器同一句；chapterIdx 从 0 起，显示给用户要 +1
    expect(line.querySelector(".book-progress-text")?.textContent).toBe("第 5 章 / 共 10 章");
    expect(line.querySelector(".book-progress-pct")?.textContent).toBe("42%");
    expect(line.querySelector(".book-progress-bar")).not.toBeNull();
    expect(line.getAttribute("data-read")).toBe("1");
  });

  it("百分比为 0 时不画进度条（空进度等于坏了），章节口径照常报", async () => {
    readingProgress.mockResolvedValue({ items: [progressAt(1, { percent: 0 })] });
    renderShelf();

    const line = await screen.findByTestId("book-progress-1");
    expect(line.querySelector(".book-progress-text")?.textContent).toBe("第 5 章 / 共 10 章");
    expect(line.querySelector(".book-progress-bar")).toBeNull();
    expect(line.querySelector(".book-progress-pct")).toBeNull();
    expect(line.getAttribute("data-read")).toBe("0");
  });

  it("进度接口失败时列表照常渲染，只是没有进度", async () => {
    readingProgress.mockRejectedValue(new Error("离线"));
    renderShelf();

    const line = await screen.findByTestId("book-progress-1");
    expect(line.querySelector(".book-progress-text")?.textContent).toBe("共 10 章");
    expect(screen.getByTestId("shelf-live")).toBeTruthy();
  });
});

describe("H5 书架：筛选回显", () => {
  it("没有筛选时整行不出现", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());
    expect(screen.queryByTestId("shelf-filter-chips")).toBeNull();
  });

  it("带筛选的地址回显成 chip，单个 × 只清这一项", async () => {
    renderShelf(["/?status=done&rating=4"]);
    const chips = await screen.findByTestId("shelf-filter-chips");
    expect(chips.textContent).toContain("已读");
    expect(chips.textContent).toContain("4 星");

    fireEvent.click(await screen.findByTestId("filter-chip-rating"));

    await waitFor(() => expect(currentQuery()).not.toContain("rating=4"));
    // 另一个条件还在：只清被点的那一项
    expect(currentQuery()).toContain("status=done");
    expect(screen.getByTestId("shelf-filter-chips").textContent).toContain("已读");
  });

  it("作者与书单用候选表把 id 翻成名字回显", async () => {
    renderShelf(["/?author=7&collection=3"]);

    const chips = await screen.findByTestId("shelf-filter-chips");
    await waitFor(() => expect(chips.textContent).toContain("示例作者"));
    expect(chips.textContent).toContain("示例书单");
  });

  it("「清除全部」一次清空全部筛选并回到无筛选的地址", async () => {
    renderShelf(["/?l1=玄幻&status=done"]);
    await screen.findByTestId("shelf-filter-chips");

    fireEvent.click(screen.getByTestId("filter-chips-clear"));

    await waitFor(() => expect(currentQuery()).toBe("/"));
    expect(screen.queryByTestId("shelf-filter-chips")).toBeNull();
  });

  it("筛选后新请求回到第 1 页（回显只是展示，不改取数口径）", async () => {
    renderShelf(["/?status=done"]);
    await waitFor(() => expect(books).toHaveBeenCalled());
    expect(books.mock.calls[0][0]).toMatchObject({ readStatus: "done", page: 1, size: 30 });
  });
});

describe("H5 书架：加载与骨架", () => {
  it("首屏加载中给骨架而不是只有转圈", async () => {
    books.mockImplementation(() => new Promise(() => {}));
    renderShelf();

    await waitFor(() => expect(screen.getByTestId("shelf-loading")).toBeTruthy());
    // 三列两行：首屏骨架正好铺满一屏，数据回来时不跳版
    expect(screen.getByTestId("shelf-loading").querySelectorAll(".shelf-skel-card")).toHaveLength(6);
  });

  it("追加下一页时底部给骨架而不是一行转圈文字", async () => {
    let release: (value: Page<BookSummary>) => void = () => {};
    books
      .mockResolvedValueOnce(pageOf(Array.from({ length: 3 }, (_, i) => bookAt(i + 1)), 6))
      .mockImplementationOnce(() => new Promise<Page<BookSummary>>((resolve) => (release = resolve)));
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-sentinel")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-load-more"));

    await waitFor(() => expect(screen.getByTestId("shelf-more-loading")).toBeTruthy());
    expect(screen.getByTestId("shelf-more-loading").querySelectorAll(".shelf-skel-card")).toHaveLength(3);
    release(pageOf([bookAt(4)], 6));
    await waitFor(() => expect(screen.queryByTestId("shelf-more-loading")).toBeNull());
  });
});

describe("H5 书架：角标是小 chip，不抢卡片", () => {
  it("角标文字仍是状态人话，点一下循环切换", async () => {
    books.mockResolvedValue(pageOf([bookAt(1, { readStatus: "want" })]));
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    const badge = screen.getByTestId("book-status-1");
    expect(badge.textContent).toBe("想读");
    // 视觉尺寸交给 .book-status-chip，角标本身只提供 44px 热区
    expect(badge.querySelector(".book-status-chip")?.textContent).toBe("想读");
    fireEvent.click(badge);
    expect(screen.getByTestId("book-status-1").textContent).toBe("在读");
  });
});

describe("H5 书架：页头与底部入口", () => {
  it("页头只出现一次书架标题，数量与排序写进一行副标题", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    const heads = screen.getAllByRole("heading", { level: 1 });
    expect(heads).toHaveLength(1);
    expect(heads[0].textContent).toBe("书架");
    // 「书架 · N 本 / 书架」的重复文案不再出现
    expect(screen.queryByText(/^书架 · /)).toBeNull();
    // 数量只出现在副标题这一行（另一处是底部「已经到底啦 · 共 N 本」）
    expect(document.querySelector(".shelf-head-sub")?.textContent).toContain("1 本 · 按最近更新");
  });

  it("页头不再有排序 / 筛选按钮：入口就是页面上那条筛选条", async () => {
    renderShelf(["/?status=done&rating=4"]);
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    expect(screen.queryByTestId("shelf-sort-trigger")).toBeNull();
    expect(screen.queryByTestId("shelf-filter-trigger")).toBeNull();
    // 排序与已选项数都写在筛选条里：分组标题带当前排序名，面板标题带已选项数
    expect(screen.getByTestId("filter-group-toggle-sort").textContent).toContain("最近更新");
    expect(screen.getByTestId("shelf-filter-panel").textContent).toContain("已选 2 项");
    expect(screen.queryByText(/^排序：/)).toBeNull();
  });

  it("没有筛选时面板标题不报计数，分组标题仍写当前排序", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());
    expect(screen.getByTestId("shelf-filter-panel").textContent).not.toContain("已选");
    expect(screen.getByTestId("filter-group-toggle-sort").textContent).toContain("最近更新");
  });

  it("面板底部按钮报出当前命中数量，点它把列表带回顶部", async () => {
    books.mockResolvedValue(pageOf([bookAt(1), bookAt(2)]));
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    const view = screen.getByTestId("panel-view-results");
    expect(view.textContent).toBe("查看 2 本");
    fireEvent.click(view);
    expect(window.scrollTo).toHaveBeenCalled();
  });
});

