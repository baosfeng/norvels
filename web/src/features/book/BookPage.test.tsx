import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, BookMetaPatch, ReadingProgress } from "../../api/types";
import { ToastHost } from "../../lib/toast";
import { TierProvider } from "../../lib/tier";

const book: BookDetail = {
  id: 7,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: 2,
  wordCount: 34000,
  volumeCount: 1,
  intro: "一段示例简介。",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  hasCover: true,
  series: "示例系列",
  seriesIndex: 2,
  tags: ["武侠", "连载"],
  serialStatus: "ongoing",
  readStatus: "",
  lastChapterTitle: "第二章",
  lastChapterIdx: 1,
  volumes: [
    {
      id: 1,
      idx: 0,
      title: "正文",
      chapters: [
        { id: 10, idx: 0, title: "第一章", volumeId: 1, wordCount: 2000 },
        { id: 11, idx: 1, title: "第二章", volumeId: 1, wordCount: 2100 },
      ],
    },
  ],
};

/** 阅读进度（默认空）：用例按需塞一条已读进度来验「打开即定位当前章」。 */
const progressState: { items: ReadingProgress[] } = { items: [] };

/** jsdom 里没有真实滚动容器高度，useVirtualizer 会一行都不渲染；这里换成「全部行都渲染」的桩。 */
const scrollToIndexSpy = vi.hoisted(() => vi.fn());

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (options: { count: number; estimateSize: () => number }) => {
    const items = Array.from({ length: options.count }, (_, index) => ({
      index,
      key: index,
      start: index * options.estimateSize(),
      size: options.estimateSize(),
    }));
    return {
      getTotalSize: () => options.count * options.estimateSize(),
      getVirtualItems: () => items,
      scrollToIndex: scrollToIndexSpy,
    };
  },
}));

/** 多卷样书：卷层级只在 ≥2 卷时才有意义，用它对照单卷形态。 */
const multiVolumeBook: BookDetail = {
  ...book,
  volumeCount: 2,
  chapterCount: 4,
  volumes: [
    {
      id: 1,
      idx: 0,
      title: "上卷",
      chapters: [
        { id: 20, idx: 0, title: "第一章", volumeId: 1, wordCount: 2000 },
        { id: 21, idx: 1, title: "第二章", volumeId: 1, wordCount: 2100 },
      ],
    },
    {
      id: 2,
      idx: 1,
      title: "下卷",
      chapters: [
        { id: 30, idx: 2, title: "第三章", volumeId: 2, wordCount: 2200 },
        { id: 31, idx: 3, title: "第四章", volumeId: 2, wordCount: 2300 },
      ],
    },
  ],
};

const bookState = {
  data: book as BookDetail | null,
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const updateBook = vi.fn<(id: number, patch: BookMetaPatch) => Promise<BookDetail>>();

vi.mock("../../api/hooks", () => ({
  useBook: () => bookState,
  useReadingProgress: () => ({ data: { items: progressState.items }, loading: false, error: null, reload: vi.fn() }),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { updateBook } };
});

const { BookPage } = await import("./BookPage");
const { ApiError } = await import("../../api/client");

/** 档位要用 Provider 显式给：jsdom 没有 matchMedia，默认档位是 PC。 */
function renderPage(tier: "pc" | "h5" = "pc") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={["/book/7"]}>
        <Routes>
          <Route path="/book/:bookId" element={<BookPage />} />
          <Route path="/shelf" element={<p>书架页</p>} />
          <Route path="/admin/books/:bookId" element={<p>单书管理页</p>} />
        </Routes>
        <ToastHost />
      </MemoryRouter>
    </TierProvider>,
  );
}

/** jsdom 默认 innerWidth=1024；目录列数按窗口宽度算，用例要能换成 1280/390。 */
function setWidth(width: number) {
  Object.defineProperty(window, "innerWidth", { configurable: true, writable: true, value: width });
}

const DEFAULT_WIDTH = window.innerWidth;

afterEach(() => {
  setWidth(DEFAULT_WIDTH);
  progressState.items = [];
  scrollToIndexSpy.mockReset();
});

const coverVersion = String(Date.parse(book.updatedAt));

describe("书籍详情：真实封面与占位回退", () => {
  beforeEach(() => {
    bookState.data = book;
    bookState.reload = vi.fn();
    updateBook.mockReset();
  });

  it("有封面时用带版本参数的图片，并延迟加载", () => {
    renderPage();
    const img = screen.getByTestId("cover-image") as HTMLImageElement;
    expect(img.getAttribute("src")).toBe(`/api/books/7/cover?v=${coverVersion}`);
    expect(img.getAttribute("loading")).toBe("lazy");
  });

  it("图片加载失败时回落到书名首字占位，而不是留一格空白", () => {
    renderPage();
    fireEvent.error(screen.getByTestId("cover-image"));
    expect(screen.queryByTestId("cover-image")).toBeNull();
    expect(screen.getByText("示")).toBeTruthy();
  });

  it("没有封面的书直接用占位", () => {
    bookState.data = { ...book, hasCover: false };
    renderPage();
    expect(screen.queryByTestId("cover-image")).toBeNull();
  });
});

describe("书籍详情：只读展示", () => {
  beforeEach(() => {
    bookState.data = book;
    updateBook.mockReset();
  });

  it("页头用顿号连起多作者，并展示连载状态与最新章节", () => {
    bookState.data = { ...book, authors: ["示例作者", "另一位"], authorName: "示例作者" };
    renderPage();
    expect(screen.getByTestId("book-byline").textContent).toContain("示例作者、另一位");
    expect(screen.getByTestId("book-serial").textContent).toBe("连载中");
    expect(screen.getByTestId("book-last-chapter").textContent).toContain("第 2 章 · 第二章");
  });

  it("还没有章节时元信息行说清「暂无章节」", () => {
    bookState.data = { ...book, volumes: [], lastChapterIdx: -1, lastChapterTitle: "" };
    renderPage();
    expect(screen.getByTestId("book-last-chapter").textContent).toContain("暂无章节");
  });

  it("展示标签；标签为空时不渲染标签行", () => {
    renderPage();
    expect(screen.getByTestId("book-tags").textContent).toContain("武侠");
  });

  it("标签为空时不渲染标签行", () => {
    bookState.data = { ...book, tags: [] };
    renderPage();
    expect(screen.queryByTestId("book-tags")).toBeNull();
  });

  it("阅读状态下拉回填当前值，改一次只提交 readStatus 并即时回显", async () => {
    updateBook.mockResolvedValue({ ...book, readStatus: "reading" });
    renderPage();
    fireEvent.change(screen.getByTestId("book-read-status"), { target: { value: "reading" } });
    await waitFor(() => expect(updateBook).toHaveBeenCalledWith(7, { readStatus: "reading" }));
    await waitFor(() => expect((screen.getByTestId("book-read-status") as HTMLSelectElement).value).toBe("reading"));
  });

  it("阅读状态写失败时退回原值，不用后端错误原文", async () => {
    updateBook.mockRejectedValue(new ApiError("http", "store: boom", 500));
    renderPage();
    fireEvent.change(screen.getByTestId("book-read-status"), { target: { value: "done" } });
    await waitFor(() => expect((screen.getByTestId("book-read-status") as HTMLSelectElement).value).toBe(""));
    expect(screen.getByTestId("toast").textContent).not.toContain("store:");
  });

  it("有书签时元信息区显示条数", () => {
    bookState.data = { ...book, bookmarkCount: 3 };
    renderPage();
    expect(screen.getByTestId("book-bookmark-count").textContent).toContain("3");
  });

  it("没有书签时不占位", () => {
    bookState.data = { ...book, bookmarkCount: 0 };
    renderPage();
    expect(screen.queryByTestId("book-bookmark-count")).toBeNull();
  });
});

describe("书籍详情：管理入口已迁出（P1 管理页）", () => {
  beforeEach(() => {
    bookState.data = book;
    updateBook.mockReset();
  });

  it("详情页不再出现编辑与删除控件", () => {
    renderPage();
    expect(screen.queryByTestId("book-edit-form")).toBeNull();
    expect(screen.queryByText("编辑信息")).toBeNull();
    expect(screen.queryByText("删除这本书")).toBeNull();
  });

  it("PC 上给出「管理这本书」入口，指向独立的书籍管理页", async () => {
    renderPage();
    const link = screen.getByTestId("book-admin-link") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/admin/books/7");
    fireEvent.click(link);
    await waitFor(() => expect(screen.getByText("单书管理页")).toBeTruthy());
  });
});

describe("书籍详情：目录形态（单卷 / 多卷）", () => {
  beforeEach(() => {
    bookState.data = book;
    updateBook.mockReset();
  });

  it("单卷书不渲染卷跳转与卷头，章节直接铺出来", () => {
    renderPage();
    expect(screen.queryByRole("group", { name: "卷跳转" })).toBeNull();
    expect(document.querySelector(".vol-jump")).toBeNull();
    expect(document.querySelector(".vol-head")).toBeNull();
    // 章节仍然在里面：2 章都在
    expect(screen.getByTestId("chapter-scroll").querySelectorAll(".cv-row")).toHaveLength(2);
  });

  it("多卷书照旧：卷跳转胶囊 + 可折叠卷头 + 每卷一个列表", () => {
    bookState.data = multiVolumeBook;
    renderPage();
    const jump = screen.getByRole("group", { name: "卷跳转" });
    expect(jump.querySelectorAll("button")).toHaveLength(2);
    const heads = document.querySelectorAll(".vol-head");
    expect(heads).toHaveLength(2);
    // 默认只展开第一卷：折叠的卷不渲染列表
    expect(screen.getAllByTestId("chapter-scroll")).toHaveLength(1);
    fireEvent.click(heads[1]);
    expect(screen.getAllByTestId("chapter-scroll")).toHaveLength(2);
  });

  it("目录不再向内嵌滚动块要高度：既不传 maxHeight 也不占固定高度", () => {
    renderPage();
    const scroller = screen.getByTestId("chapter-scroll");
    expect(scroller.style.maxHeight).toBe("");
    expect(scroller.style.height).toBe("");
    expect(scroller.className).not.toContain("is-fill");
  });
});

describe("书籍详情：目录列数与行高", () => {
  beforeEach(() => {
    bookState.data = book;
    updateBook.mockReset();
    scrollToIndexSpy.mockReset();
  });

  it("PC 宽 1280 三列 + 32px 紧凑行高", () => {
    setWidth(1280);
    renderPage();
    const scroller = screen.getByTestId("chapter-scroll");
    expect(scroller.getAttribute("data-columns")).toBe("3");
    expect(scroller.className).toContain("is-dense");
    const row = screen.getAllByTestId("chapter-word")[0].parentElement as HTMLElement;
    expect(row.style.height).toBe("32px");
  });

  it("PC 宽 1000 两列", () => {
    setWidth(1000);
    renderPage();
    expect(screen.getByTestId("chapter-scroll").getAttribute("data-columns")).toBe("2");
  });

  it("H5 恒一列、44px 触控行高，不使用紧凑行", () => {
    setWidth(390);
    renderPage("h5");
    const scroller = screen.getByTestId("chapter-scroll");
    expect(scroller.getAttribute("data-columns")).toBe("1");
    expect(scroller.className).not.toContain("is-dense");
    const row = screen.getAllByTestId("chapter-word")[0].parentElement as HTMLElement;
    expect(row.style.height).toBe("44px");
  });

  it("窗口从 1280 拖到 1000 时列数跟着变", () => {
    setWidth(1280);
    renderPage();
    expect(screen.getByTestId("chapter-scroll").getAttribute("data-columns")).toBe("3");
    setWidth(1000);
    fireEvent(window, new Event("resize"));
    expect(screen.getByTestId("chapter-scroll").getAttribute("data-columns")).toBe("2");
  });

  it("打开就定位到当前章所在行", () => {
    setWidth(1280);
    progressState.items = [
      {
        bookId: 7,
        bookTitle: "示例书简",
        chapterId: 11,
        chapterIdx: 1,
        chapterTitle: "第二章",
        percent: 12,
        updatedAt: "2026-10-25T00:00:00.000Z",
      },
    ];
    renderPage();
    // 2 章、3 列 → 当前章（下标 1）在第 0 行
    expect(scrollToIndexSpy).toHaveBeenCalledWith(0, { align: "center" });
  });
});
