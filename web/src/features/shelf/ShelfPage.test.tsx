import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookQuery, RequestOptions } from "../../api/client";
import type { BookSummary, Page } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const books = vi.fn<(params: BookQuery, opts?: RequestOptions) => Promise<Page<BookSummary>>>();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      books: (params: BookQuery, opts?: RequestOptions) => books(params, opts),
      authors: vi.fn(async () => ({ items: [], total: 0, page: 1, size: 20 })),
      tags: vi.fn(async () => ({ items: [] })),
    },
  };
});

const { ShelfPage } = await import("./ShelfPage");

type IOCallback = (entries: IntersectionObserverEntry[], observer: IntersectionObserver) => void;
let ioCallbacks: IOCallback[] = [];

class MockIntersectionObserver {
  constructor(callback: IOCallback) {
    ioCallbacks.push(callback);
  }
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return [];
  }
}

function bookAt(index: number): BookSummary {
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
  };
}

function pageOf(from: number, count: number, total: number): Page<BookSummary> {
  return { items: Array.from({ length: count }, (_, i) => bookAt(from + i)), total, page: 1, size: 30 };
}

function cardCount(): number {
  return screen.getByTestId("shelf-live").querySelectorAll(".book-card").length;
}

function renderShelf(tier: "h5" | "pc" = "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter>
        <ShelfPage />
      </MemoryRouter>
    </TierProvider>,
  );
}

/** 模拟「滚动到底」：手动触发最近一次 IntersectionObserver 的回调 */
async function reachBottom() {
  await act(async () => {
    for (const callback of [...ioCallbacks]) {
      callback([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver);
    }
  });
}

describe("书架：H5 无限滚动与排序", () => {
  beforeEach(() => {
    books.mockReset();
    ioCallbacks = [];
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  it("首屏只取第一页 30 本，底部还没有「到底」文案", async () => {
    books.mockResolvedValueOnce(pageOf(1, 30, 45));
    renderShelf();

    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());
    expect(books).toHaveBeenCalledTimes(1);
    expect(books.mock.calls[0][0]).toMatchObject({ sort: "updated", page: 1, size: 30 });
    expect(cardCount()).toBe(30);
    expect(screen.getByTestId("shelf-sentinel")).toBeTruthy();
    expect(screen.queryByTestId("shelf-end")).toBeNull();
    // H5 不放页码分页，PC 工具栏也不渲染
    expect(screen.queryByRole("navigation", { name: "分页" })).toBeNull();
    expect(screen.queryByTestId("shelf-toolbar")).toBeNull();
  });

  it("滚到底自动追加下一页，取完显示「没有更多了」", async () => {
    books.mockResolvedValueOnce(pageOf(1, 30, 45));
    books.mockResolvedValueOnce(pageOf(31, 15, 45));
    renderShelf();
    await waitFor(() => expect(cardCount()).toBe(30));

    await reachBottom();
    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ page: 2, size: 30 });

    await waitFor(() => expect(cardCount()).toBe(45));
    expect(screen.getByTestId("shelf-end").textContent).toContain("没有更多了");
    expect(screen.queryByTestId("shelf-sentinel")).toBeNull();
  });

  it("换排序后回到第 1 页重新取数，结果替换而不是追加", async () => {
    books.mockResolvedValueOnce(pageOf(1, 30, 45));
    books.mockResolvedValueOnce(pageOf(100, 5, 5));
    renderShelf();
    await waitFor(() => expect(cardCount()).toBe(30));

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    expect(within(screen.getByTestId("shelf-sort-sheet")).getByRole("dialog")).toBeTruthy();
    fireEvent.click(screen.getByTestId("sort-option-title"));

    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ sort: "title", page: 1, size: 30 });
    await waitFor(() => expect(cardCount()).toBe(5));
    expect(screen.queryByTestId("shelf-sort-sheet")).toBeNull();
    expect(window.scrollTo).toHaveBeenCalled();
  });

  it("排序切换时丢弃在途响应：旧排序晚到的结果不落库", async () => {
    let resolveOld: (value: Page<BookSummary>) => void = () => {};
    let resolveNew: (value: Page<BookSummary>) => void = () => {};
    books
      .mockImplementationOnce(() => new Promise<Page<BookSummary>>((resolve) => (resolveOld = resolve)))
      .mockImplementationOnce(() => new Promise<Page<BookSummary>>((resolve) => (resolveNew = resolve)));
    renderShelf();
    expect(books).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    fireEvent.click(screen.getByTestId("sort-option-words"));
    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));

    // 新排序先回，旧排序后回：旧结果必须被丢弃
    await act(async () => resolveNew(pageOf(200, 3, 3)));
    await waitFor(() => expect(cardCount()).toBe(3));

    await act(async () => resolveOld(pageOf(1, 30, 45)));
    expect(cardCount()).toBe(3);
    expect(screen.queryByText("示例书 1")).toBeNull();
    expect(screen.getByText("示例书 200")).toBeTruthy();
  });

  it("PC 仍是工具栏 + 页码分页，没有排序弹层", async () => {
    books.mockResolvedValueOnce(pageOf(1, 24, 48));
    renderShelf("pc");

    await waitFor(() => expect(cardCount()).toBe(24));
    expect(screen.getByTestId("shelf-toolbar")).toBeTruthy();
    expect(screen.queryByTestId("shelf-sort-trigger")).toBeNull();
    expect(screen.queryByTestId("shelf-sentinel")).toBeNull();
    expect(screen.getByRole("navigation", { name: "分页" })).toBeTruthy();
  });
});

describe("视口底子：100vh 只作 100dvh 的回退", () => {
  // vitest 默认把 CSS 当空模块，import.meta.glob("?raw") 拿不到正文，这里直接读源码
  const STYLE_FILES = [
    "../../styles/globals.css",
    "../../styles/prototype.css",
    "../../styles/tokens.css",
    "../../styles/motion.css",
  ];

  async function readStyle(relative: string): Promise<string> {
    const fs = (await import("node:fs" as string)) as { readFileSync: (file: URL, encoding: string) => string };
    return fs.readFileSync(new URL(relative, import.meta.url), "utf8");
  }

  it("每个 100vh 声明都紧跟 100dvh 回退", async () => {
    for (const file of STYLE_FILES) {
      const lines = (await readStyle(file)).split("\n");
      lines.forEach((line, index) => {
        if (!line.includes("100vh")) return;
        const hasFallback = line.includes("100dvh") || (lines[index + 1] ?? "").includes("100dvh");
        expect(hasFallback, `${file}:${index + 1} 的 100vh 缺少 100dvh 回退`).toBe(true);
      });
    }
  });

  it("补了安全区与触屏底子", async () => {
    const css = (await Promise.all(STYLE_FILES.map(readStyle))).join("\n");
    expect(css).toContain("env(safe-area-inset-bottom)");
    expect(css).toContain("touch-action: manipulation");
    expect(css).toContain("-webkit-tap-highlight-color: transparent");
    expect(css).toContain("user-select: none");
  });
});
