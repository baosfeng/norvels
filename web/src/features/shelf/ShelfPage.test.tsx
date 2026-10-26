import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookQuery, RequestOptions } from "../../api/client";
import type { BookDetail, BookMetaPatch, BookSummary, Page } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const books = vi.fn<(params: BookQuery, opts?: RequestOptions) => Promise<Page<BookSummary>>>();
const updateBook = vi.fn<(id: number, patch: BookMetaPatch) => Promise<BookDetail>>();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      books: (params: BookQuery, opts?: RequestOptions) => books(params, opts),
      updateBook: (id: number, patch: BookMetaPatch) => updateBook(id, patch),
      authors: vi.fn(async () => ({ items: [], total: 0, page: 1, size: 20 })),
      tags: vi.fn(async () => ({ items: [] })),
    },
  };
});

const { ShelfPage } = await import("./ShelfPage");
const { writeShelfCache, readShelfCache } = await import("../../lib/shelfCache");

// sessionStorage 在同一个测试文件里跨用例共享：不清就会拿上一个用例的缓存渲染
beforeEach(() => {
  window.sessionStorage.clear();
});

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
    updateBook.mockReset();
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
    // 弹层打开时锁住页面滚动，选完即解锁
    expect(document.body.classList.contains("h5-sheet-open")).toBe(true);
    fireEvent.click(screen.getByTestId("sort-option-title"));
    expect(document.body.classList.contains("h5-sheet-open")).toBe(false);

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

describe("书架：阅读状态筛选与书卡角标", () => {
  beforeEach(() => {
    books.mockReset();
    updateBook.mockReset();
    ioCallbacks = [];
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  it("PC 工具栏选「在读」只请求在读的书", async () => {
    books.mockResolvedValueOnce(pageOf(1, 2, 2));
    books.mockResolvedValueOnce({ items: [bookAt(5)], total: 1, page: 1, size: 24 });
    renderShelf("pc");
    await waitFor(() => expect(cardCount()).toBe(2));

    fireEvent.change(screen.getByTestId("filter-read-status"), { target: { value: "reading" } });

    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ readStatus: "reading", page: 1 });
    await waitFor(() => expect(cardCount()).toBe(1));
  });

  it("H5 把阅读状态收进底部弹层，选完即收起并重新取数", async () => {
    books.mockResolvedValueOnce(pageOf(1, 30, 30));
    books.mockResolvedValueOnce({ items: [bookAt(9)], total: 1, page: 1, size: 30 });
    renderShelf();
    await waitFor(() => expect(cardCount()).toBe(30));

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    fireEvent.click(screen.getByTestId("read-status-option-done"));

    expect(screen.queryByTestId("shelf-sort-sheet")).toBeNull();
    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ readStatus: "done", page: 1, size: 30 });
  });

  it("书卡角标点一下循环：想读 → 在读 → 已读 → 未标记，PATCH 只带 readStatus", async () => {
    books.mockResolvedValueOnce({
      items: [{ ...bookAt(1), readStatus: "want", bookmarkCount: 2 }],
      total: 1,
      page: 1,
      size: 24,
    });
    updateBook.mockResolvedValue({} as BookDetail);
    renderShelf("pc");
    await waitFor(() => expect(cardCount()).toBe(1));

    const badge = screen.getByTestId("book-status-1");
    expect(badge.textContent).toBe("想读");

    fireEvent.click(badge);
    expect(badge.textContent).toBe("在读");
    fireEvent.click(badge);
    expect(badge.textContent).toBe("已读");
    fireEvent.click(badge);
    expect(badge.textContent).toBe("未标记");

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(3));
    expect(updateBook.mock.calls.map((call) => call[1])).toEqual([
      { readStatus: "reading" },
      { readStatus: "done" },
      { readStatus: "" },
    ]);
    expect(updateBook.mock.calls[0][0]).toBe(1);
  });

  it("PATCH 失败时角标退回原值", async () => {
    books.mockResolvedValueOnce({ items: [{ ...bookAt(1), readStatus: "want" }], total: 1, page: 1, size: 24 });
    updateBook.mockRejectedValueOnce(new Error("离线"));
    renderShelf("pc");
    await waitFor(() => expect(cardCount()).toBe(1));

    const badge = screen.getByTestId("book-status-1");
    fireEvent.click(badge);
    expect(badge.textContent).toBe("在读");

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(badge.textContent).toBe("想读"));
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
describe("书架：多选与批量设作者", () => {
  beforeEach(() => {
    books.mockReset();
    updateBook.mockReset();
    ioCallbacks = [];
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  async function selectTwo(tier: "pc" | "h5" = "pc") {
    // 批量成功后书架会 reload：用可重复的返回值，别让第二次取数拿到 undefined
    books.mockResolvedValue({ items: [bookAt(1), bookAt(2)], total: 2, page: 1, size: 24 });
    renderShelf(tier);
    await waitFor(() => expect(cardCount()).toBe(2));
    fireEvent.click(screen.getByTestId("shelf-select-toggle"));
    fireEvent.click(screen.getByTestId("book-select-1"));
    fireEvent.click(screen.getByTestId("book-select-2"));
  }

  it("选中 2 本后循环 PATCH，全部成功后收工并退出选择模式", async () => {
    updateBook.mockResolvedValue({} as BookDetail);
    await selectTwo();
    expect(screen.getByTestId("shelf-batch-author").textContent).toContain("（2）");

    fireEvent.click(screen.getByTestId("shelf-batch-author"));
    fireEvent.change(screen.getByLabelText("作者（多位用「、」或逗号分隔）"), { target: { value: "张三、李四" } });
    fireEvent.click(screen.getByTestId("batch-author-confirm"));

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(2));
    expect(updateBook.mock.calls.map((call) => call[0])).toEqual([1, 2]);
    expect(updateBook.mock.calls[0][1]).toEqual({ authors: ["张三", "李四"] });

    await waitFor(() => expect(screen.queryByTestId("batch-author-dialog")).toBeNull());
    expect(screen.queryByTestId("book-select-1")).toBeNull();
  });

  it("有失败项时显示「已完成 x / N」，可只重试失败的那本", async () => {
    // 第 2 本第一次失败、重试时成功：用实现而不是一次性队列，避免次数对不上
    let failBook2 = true;
    updateBook.mockImplementation(async (id: number) => {
      if (id === 2 && failBook2) {
        failBook2 = false;
        throw new Error("离线");
      }
      return {} as BookDetail;
    });
    await selectTwo();

    fireEvent.click(screen.getByTestId("shelf-batch-author"));
    fireEvent.change(screen.getByLabelText("作者（多位用「、」或逗号分隔）"), { target: { value: "张三" } });
    fireEvent.click(screen.getByTestId("batch-author-confirm"));

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.getByTestId("batch-author-progress").textContent).toContain("1 / 2"));
    expect(screen.getByTestId("batch-author-failures").textContent).toContain("示例书 2");
    expect(screen.getByTestId("batch-author-dialog")).toBeTruthy();

    fireEvent.click(screen.getByTestId("batch-author-retry"));
    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(3));
    // 只重试失败的那本
    expect(updateBook.mock.calls[2][0]).toBe(2);
    // 重试成功后整批收工：弹层关闭、退出选择模式
    await waitFor(() => expect(screen.queryByTestId("batch-author-dialog")).toBeNull());
    expect(screen.queryByTestId("book-select-1")).toBeNull();
  });

  it("作者留空即清空，下发空数组（按契约）", async () => {
    updateBook.mockResolvedValue({} as BookDetail);
    books.mockResolvedValue({ items: [bookAt(1)], total: 1, page: 1, size: 24 });
    renderShelf("h5");
    await waitFor(() => expect(cardCount()).toBe(1));

    fireEvent.click(screen.getByTestId("shelf-select-toggle"));
    fireEvent.click(screen.getByTestId("book-select-1"));
    fireEvent.click(screen.getByTestId("shelf-batch-author"));
    fireEvent.change(screen.getByLabelText("作者（多位用「、」或逗号分隔）"), { target: { value: "  " } });
    expect(screen.getByTestId("batch-author-confirm").textContent).toContain("清空 1 本作者");
    fireEvent.click(screen.getByTestId("batch-author-confirm"));

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(1));
    expect(updateBook.mock.calls[0][1]).toEqual({ authors: [] });
  });
});
describe("书架：返回定位与缓存恢复", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    books.mockReset();
    updateBook.mockReset();
    ioCallbacks = [];
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    vi.spyOn(window, "scrollY", "get").mockReturnValue(673);
  });

  function cachedState(items: BookSummary[], overrides: Record<string, unknown> = {}) {
    writeShelfCache({
      sort: "updated",
      authorId: null,
      tag: null,
      readStatus: null,
      size: 30,
      mode: "infinite",
      items,
      total: 64,
      page: 2,
      scrollTop: 673,
      lastBookId: items[items.length - 1].id,
      ...overrides,
    });
  }

  function manyBooks(count: number): BookSummary[] {
    return Array.from({ length: count }, (_, i) => bookAt(i + 1));
  }

  it("命中缓存时先用缓存渲染：不发第 1 页请求，项数保留并恢复滚动位置", async () => {
    cachedState(manyBooks(60));
    renderShelf();

    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());
    expect(cardCount()).toBe(60);
    // 60 秒内的缓存只渲染不刷新：返回书架不该再拉一次第 1 页
    expect(books).not.toHaveBeenCalled();
    expect(window.scrollTo).toHaveBeenCalledWith({ top: 673 });
  });

  it("缓存过期（>30 分钟）不恢复，照常从第 1 页取", async () => {
    const now = Date.now();
    writeShelfCache(
      {
        sort: "updated",
        authorId: null,
        tag: null,
        readStatus: null,
        size: 30,
        mode: "infinite",
        items: manyBooks(60),
        total: 64,
        page: 2,
        scrollTop: 673,
        lastBookId: 60,
      },
      now - 31 * 60 * 1000,
    );
    books.mockResolvedValueOnce(pageOf(1, 30, 64));
    renderShelf();

    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));
    expect(books.mock.calls[0][0]).toMatchObject({ page: 1, size: 30 });
    await waitFor(() => expect(cardCount()).toBe(30));
  });

  it("点卡片进书前把「最近打开的书」与滚动位置写进缓存，供返回时定位", async () => {
    books.mockResolvedValueOnce(pageOf(1, 2, 2));
    renderShelf("pc");
    await waitFor(() => expect(cardCount()).toBe(2));

    fireEvent.click(screen.getAllByRole("link", { name: /示例书 2/ })[0]);

    const cached = readShelfCache();
    expect(cached?.lastBookId).toBe(2);
    expect(cached?.scrollTop).toBe(673);
    expect(cached?.items).toHaveLength(2);
  });

  it("返回后最近打开的那本被标成 is-recent 高亮 1.5 秒", async () => {
    cachedState(manyBooks(4), { lastBookId: 3 });
    renderShelf();

    await waitFor(() => expect(cardCount()).toBe(4));
    const recent = document.querySelector(".book-card.is-recent");
    expect(recent).not.toBeNull();
    expect(recent?.getAttribute("data-book-id")).toBe("3");
  });
});

describe("书架：长标题与角标布局（样式契约）", () => {
  async function readCss(relative: string): Promise<string> {
    const fs = (await import("node:fs" as string)) as { readFileSync: (file: URL, encoding: string) => string };
    return fs.readFileSync(new URL(relative, import.meta.url), "utf8");
  }

  it("摘掉书名 nowrap，详情页书名最多 3 行，副标题可换行", async () => {
    const css = await readCss("../../styles/prototype.css");
    // 标题组不再把 书名/页标题/抽屉标题 钉成单行
    expect(css).not.toMatch(/\.dir-head h2,\.page-head h1,\.book-main h1,/);
    expect(css).toContain(".page-head h1,.book-main h1,.dir-head h2{white-space:normal");
    expect(css).toContain("-webkit-line-clamp:3;-webkit-box-orient:vertical;overflow:hidden");
    expect(css).not.toContain(".page-head .sub{white-space:nowrap}");
  });

  it("卡片书名 3 行、作者 2 行、角标静态排在元信息行（不绝对定位压标题）", async () => {
    const css = await readCss("../../styles/prototype.css");
    expect(css).toContain(".book-author{display:-webkit-box;-webkit-line-clamp:2");
    expect(css).toContain(".book-status {\n  margin-left: auto;");
    expect(css).not.toMatch(/\.book-status \{\n {2}position: absolute;/);
    // ≤520 单列
    expect(css).toContain("@media (max-width: 520px)");
  });

  it("书架/目录/书签条目关闭长按选中与系统菜单", async () => {
    const css = await readCss("../../styles/prototype.css");
    expect(css).toContain(".book-link,");
    expect(css).toMatch(/user-select: none;/);
    expect(css).toContain("-webkit-touch-callout: none;");
    expect(css).toContain("\n.cv-row,");
    expect(css).toContain("\n.bookmark-jump,");
  });
});
