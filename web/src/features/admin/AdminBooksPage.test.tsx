import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookSummary } from "../../api/types";

const { booksState, tagsState, seenParams, updateBook, deleteBook } = vi.hoisted(() => {
  const book = (id: number, title: string, serial: "" | "ongoing" | "completed"): unknown => ({
    id,
    title,
    authorId: 1,
    authorName: "示例作者",
    chapterCount: 12,
    wordCount: 34000,
    volumeCount: 1,
    intro: "",
    status: "parsed",
    serialStatus: serial,
    importedAt: "2026-10-24T20:24:50.859Z",
    updatedAt: "2026-10-25T02:00:00.000Z",
    hasCover: false,
  });
  return {
    booksState: {
      data: { items: [book(7, "示例书简", "ongoing"), book(8, "另一本书", "")], total: 2, page: 1, size: 20 },
      loading: false,
      error: null as unknown,
      reload: () => undefined,
    },
    tagsState: { data: { items: [{ id: 1, name: "武侠", bookCount: 2 }] }, loading: false, error: null, reload: () => undefined },
    seenParams: { last: null as Record<string, unknown> | null },
    updateBook: vi.fn(),
    deleteBook: vi.fn(),
  };
});

vi.mock("../../api/hooks", () => ({
  useBooks: (params: Record<string, unknown>) => {
    seenParams.last = params;
    return booksState;
  },
  useTags: () => tagsState,
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { updateBook, deleteBook } };
});

const { AdminBooksPage } = await import("./AdminBooksPage");

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/admin"]}>
      <Routes>
        <Route path="/admin" element={<AdminBooksPage />} />
        <Route path="/admin/books/:bookId" element={<p>单书管理页</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

const items = () => booksState.data.items as BookSummary[];

describe("书籍管理列表", () => {
  beforeEach(() => {
    booksState.data = {
      items: [
        { ...items()[0], title: "示例书简", serialStatus: "ongoing" },
        { ...items()[1], title: "另一本书", serialStatus: "" },
      ],
      total: 2,
      page: 1,
      size: 20,
    } as typeof booksState.data;
    updateBook.mockReset();
    deleteBook.mockReset();
  });

  it("表格列出书名、作者与连载状态", () => {
    renderPage();
    expect(screen.getByTestId("admin-book-open-7").textContent).toBe("示例书简");
    expect(screen.getAllByText("示例作者")).toHaveLength(2);
    expect(screen.getByTestId("admin-book-row-7").textContent).toContain("连载中");
    expect(screen.getByTestId("admin-book-row-8").textContent).toContain("未知");
  });

  it("书名链接进单书管理页，而不是阅读详情页", async () => {
    renderPage();
    const link = screen.getByTestId("admin-book-open-7") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/admin/books/7");
    fireEvent.click(link);
    await waitFor(() => expect(screen.getByText("单书管理页")).toBeTruthy());
  });

  it("勾选一本书后出现批量条", () => {
    renderPage();
    expect(screen.queryByTestId("admin-batch-bar")).toBeNull();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    expect(screen.getByTestId("admin-batch-bar").textContent).toContain("已选");
    fireEvent.click(screen.getByTestId("admin-batch-clear"));
    expect(screen.queryByTestId("admin-batch-bar")).toBeNull();
  });

  it("批量设作者：逐本下发 PATCH authors，完成后提示并刷新", async () => {
    updateBook.mockResolvedValue({});
    renderPage();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    fireEvent.click(screen.getByTestId("admin-batch-author"));
    fireEvent.change(screen.getByTestId("admin-batch-input"), { target: { value: "新作者" } });
    fireEvent.click(screen.getByTestId("admin-batch-confirm"));
    await waitFor(() => expect(updateBook).toHaveBeenCalledWith(7, { authors: ["新作者"] }));
    await waitFor(() => expect(screen.getByTestId("admin-list-notice").textContent).toContain("作者"));
  });

  it("批量删除走 DELETE，并在完成后清空选择", async () => {
    deleteBook.mockResolvedValue(undefined);
    renderPage();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    fireEvent.click(screen.getByTestId("admin-batch-delete"));
    expect(screen.getByTestId("admin-batch-warning").textContent).toContain("无法恢复");
    fireEvent.click(screen.getByTestId("admin-batch-confirm"));
    await waitFor(() => expect(deleteBook).toHaveBeenCalledWith(7));
    await waitFor(() => expect(screen.queryByTestId("admin-batch-bar")).toBeNull());
  });

  it("改连载状态筛选会把参数带进查询", () => {
    renderPage();
    fireEvent.change(screen.getByTestId("admin-filter-serial"), { target: { value: "ongoing" } });
    expect(seenParams.last?.serialStatus).toBe("ongoing");
  });

  it("空库给出去导入的出口", () => {
    booksState.data = { items: [], total: 0, page: 1, size: 20 } as typeof booksState.data;
    renderPage();
    expect(screen.getByTestId("admin-list-empty").textContent).toContain("书库还没有书");
  });

  it("有筛选但没命中时提示清除筛选", () => {
    booksState.data = { items: [], total: 0, page: 1, size: 20 } as typeof booksState.data;
    renderPage();
    fireEvent.change(screen.getByTestId("admin-filter-serial"), { target: { value: "completed" } });
    expect(screen.getByTestId("admin-list-nomatch")).toBeTruthy();
  });
});
