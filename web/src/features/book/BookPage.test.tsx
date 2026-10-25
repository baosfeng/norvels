import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, BookMetaPatch } from "../../api/types";
import { ToastHost } from "../../lib/toast";

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

const bookState = {
  data: book as BookDetail | null,
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const updateBook = vi.fn<(id: number, patch: BookMetaPatch) => Promise<BookDetail>>();

vi.mock("../../api/hooks", () => ({
  useBook: () => bookState,
  useReadingProgress: () => ({ data: { items: [] }, loading: false, error: null, reload: vi.fn() }),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { updateBook } };
});

const { BookPage } = await import("./BookPage");
const { ApiError } = await import("../../api/client");

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/book/7"]}>
      <Routes>
        <Route path="/book/:bookId" element={<BookPage />} />
        <Route path="/shelf" element={<p>书架页</p>} />
        <Route path="/admin/books/:bookId" element={<p>单书管理页</p>} />
      </Routes>
      <ToastHost />
    </MemoryRouter>,
  );
}

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
