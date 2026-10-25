import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookQuery, RequestOptions } from "../../api/client";
import type { BookSummary, Page, ReadingProgress } from "../../api/types";
import { TierProvider } from "../../lib/tier";
import { ToastHost } from "../../lib/toast";

const books = vi.fn<(params: BookQuery, opts?: RequestOptions) => Promise<Page<BookSummary>>>();
const readingProgress = vi.fn();
const randomBook = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      books: (params: BookQuery, opts?: RequestOptions) => books(params, opts),
      readingProgress: (opts?: unknown) => readingProgress(opts),
      randomBook: (params: unknown, opts?: unknown) => randomBook(params, opts),
      authors: vi.fn(async () => ({ items: [], total: 0, page: 1, size: 20 })),
      tags: vi.fn(async () => ({ items: [] })),
      collections: vi.fn(async () => ({ items: [] })),
    },
  };
});

const { ShelfPage } = await import("./ShelfPage");

function book(over: Partial<BookSummary> = {}): BookSummary {
  return {
    id: 1,
    title: "示例书",
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

function progress(over: Partial<ReadingProgress> = {}): ReadingProgress {
  return {
    bookId: 5,
    bookTitle: "在读的书",
    chapterId: 51,
    chapterIdx: 4,
    chapterTitle: "第五章 归途",
    percent: 0.42,
    updatedAt: new Date(Date.now() - 3600 * 1000).toISOString(),
    ...over,
  };
}

function renderShelf(tier: "pc" | "h5" = "pc") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={["/shelf"]}>
        <Routes>
          <Route path="/shelf" element={<ShelfPage />} />
          <Route path="/book/:bookId" element={<p>书籍详情页</p>} />
          <Route path="/read/:bookId/:chapterId" element={<p>阅读器</p>} />
        </Routes>
        <ToastHost />
      </MemoryRouter>
    </TierProvider>,
  );
}

beforeEach(() => {
  books.mockReset();
  readingProgress.mockReset();
  randomBook.mockReset();
  books.mockResolvedValue({ items: [book()], total: 1, page: 1, size: 24 });
  readingProgress.mockResolvedValue({ items: [progress()] });
  window.sessionStorage.clear();
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

describe("书架：继续阅读卡", () => {
  it("PC 顶部显示继续阅读，点进阅读器的那一章", async () => {
    renderShelf("pc");
    await waitFor(() => expect(screen.getByTestId("shelf-resume")).toBeTruthy());
    const card = screen.getByTestId("shelf-resume");
    expect(card.textContent).toContain("在读的书");
    expect(card.textContent).toContain("第五章 归途");
    expect(card.textContent).toContain("已读 42%");
    expect(card.getAttribute("href")).toBe("/read/5/51");
  });

  it("没有阅读进度时整卡不渲染，不占版面", async () => {
    readingProgress.mockResolvedValue({ items: [] });
    renderShelf("pc");
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());
    expect(screen.queryByTestId("shelf-resume")).toBeNull();
  });

  it("H5 书架不显示继续阅读卡（首页已有）", async () => {
    renderShelf("h5");
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());
    expect(screen.queryByTestId("shelf-resume")).toBeNull();
  });
});

describe("书架：随机一本", () => {
  it("挑到书就进详情页", async () => {
    randomBook.mockResolvedValue(book({ id: 42 }));
    renderShelf("pc");
    await waitFor(() => expect(screen.getByTestId("shelf-random")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-random"));
    await waitFor(() => expect(screen.getByText("书籍详情页")).toBeTruthy());
    expect(randomBook).toHaveBeenCalledTimes(1);
  });

  it("没有符合条件的书（404）给提示而不是错误页", async () => {
    const { ApiError } = await vi.importActual<typeof import("../../api/client")>("../../api/client");
    randomBook.mockRejectedValue(new ApiError("http", "not found", 404, "not_found"));
    renderShelf("pc");
    await waitFor(() => expect(screen.getByTestId("shelf-random")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-random"));
    await waitFor(() => expect(screen.getByTestId("toast").textContent).toContain("没有符合条件的书"));
    expect(screen.getByTestId("shelf-live")).toBeTruthy();
  });
});

describe("书架：追更新章角标", () => {
  it("newChapters>0 的卡片显示「新章 +N」，0 的不显示", async () => {
    books.mockResolvedValue({
      items: [book({ id: 1, newChapters: 3 }), book({ id: 2, title: "旧书", newChapters: 0 })],
      total: 2,
      page: 1,
      size: 24,
    });
    renderShelf("pc");

    await waitFor(() => expect(screen.getByTestId("book-new-chapters-1")).toBeTruthy());
    expect(screen.getByTestId("book-new-chapters-1").textContent).toBe("新章 +3");
    expect(screen.queryByTestId("book-new-chapters-2")).toBeNull();
  });
});
