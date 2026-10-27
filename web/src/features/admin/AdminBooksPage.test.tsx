import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookSummary } from "../../api/types";

const { booksState, tagsState, seenParams, updateBook, deleteBook, batchUpdateBooks } = vi.hoisted(() => {
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
    batchUpdateBooks: vi.fn(),
  };
});

vi.mock("../../api/hooks", () => ({
  useBooks: (params: Record<string, unknown>) => {
    seenParams.last = params;
    return booksState;
  },
  useTags: () => tagsState,
  useCategories: () => ({ data: { items: [{ l1: "玄幻", count: 2, items: [{ l2: "东方玄幻", count: 2 }] }], total: 1 }, loading: false, error: null, reload: () => undefined }),
  useAuthors: () => ({ data: { items: [{ id: 1, name: "示例作者", bookCount: 2, wordCount: 34000 }], total: 1, page: 1, size: 50 }, loading: false, error: null, reload: () => undefined }),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { updateBook, deleteBook, batchUpdateBooks } };
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
    batchUpdateBooks.mockReset();
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

  it("批量设作者走一次 batchUpdateBooks，不再逐本 PATCH", async () => {
    batchUpdateBooks.mockResolvedValue({ updated: [7, 8], failed: [] });
    renderPage();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    fireEvent.click(screen.getByTestId("admin-book-pick-8"));
    fireEvent.click(screen.getByTestId("admin-batch-author"));
    fireEvent.change(screen.getByTestId("admin-batch-input"), { target: { value: "新作者" } });
    fireEvent.click(screen.getByTestId("admin-batch-confirm"));

    await waitFor(() => expect(batchUpdateBooks).toHaveBeenCalledTimes(1));
    expect(batchUpdateBooks).toHaveBeenCalledWith({ ids: [7, 8], patch: { authors: ["新作者"] } });
    expect(updateBook).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByTestId("admin-list-notice").textContent).toContain("新作者"));
    // 全部成功才关弹窗并清空选择
    expect(screen.queryByTestId("admin-batch-dialog")).toBeNull();
    expect(screen.queryByTestId("admin-batch-bar")).toBeNull();
  });

  it("批量设标签与设状态都走同一次 batch 端点，patch 只带对应字段", async () => {
    batchUpdateBooks.mockResolvedValue({ updated: [7], failed: [] });
    renderPage();

    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    fireEvent.click(screen.getByTestId("admin-batch-tag"));
    fireEvent.change(screen.getByTestId("admin-batch-input"), { target: { value: "武侠, 群像" } });
    fireEvent.click(screen.getByTestId("admin-batch-confirm"));
    await waitFor(() => expect(batchUpdateBooks).toHaveBeenCalledWith({ ids: [7], patch: { tags: ["武侠", "群像"] } }));

    batchUpdateBooks.mockClear();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    fireEvent.click(screen.getByTestId("admin-batch-serial"));
    fireEvent.change(screen.getByTestId("admin-batch-serial-input"), { target: { value: "completed" } });
    fireEvent.click(screen.getByTestId("admin-batch-confirm"));
    await waitFor(() =>
      expect(batchUpdateBooks).toHaveBeenCalledWith({ ids: [7], patch: { serialStatus: "completed" } }),
    );
  });

  it("部分失败：失败清单带后端原因，可只重试那一本", async () => {
    batchUpdateBooks
      .mockResolvedValueOnce({ updated: [7], failed: [{ id: 8, message: "作者太长" }] })
      .mockResolvedValueOnce({ updated: [8], failed: [] });
    renderPage();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    fireEvent.click(screen.getByTestId("admin-book-pick-8"));
    fireEvent.click(screen.getByTestId("admin-batch-author"));
    fireEvent.change(screen.getByTestId("admin-batch-input"), { target: { value: "新作者" } });
    fireEvent.click(screen.getByTestId("admin-batch-confirm"));

    await waitFor(() => expect(screen.getByTestId("admin-batch-failed-8").textContent).toContain("作者太长"));
    expect(screen.getByTestId("admin-batch-failed-8").textContent).toContain("另一本书");
    // 部分失败不关弹窗、不清选择：先让用户看到哪几本没成
    expect(screen.getByTestId("admin-batch-dialog")).toBeTruthy();

    fireEvent.click(screen.getByTestId("admin-batch-retry-8"));
    await waitFor(() => expect(batchUpdateBooks).toHaveBeenCalledTimes(2));
    expect(batchUpdateBooks.mock.calls[1][0]).toEqual({ ids: [8], patch: { authors: ["新作者"] } });
    await waitFor(() => expect(screen.queryByTestId("admin-batch-failures")).toBeNull());
  });

  it("作者留空在本地就挡下，不打一次注定 400 的请求", async () => {
    renderPage();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    fireEvent.click(screen.getByTestId("admin-batch-author"));
    fireEvent.click(screen.getByTestId("admin-batch-confirm"));

    expect(batchUpdateBooks).not.toHaveBeenCalled();
    expect(screen.getByTestId("admin-batch-error").textContent).toContain("作者不能为空");
  });

  it("整批请求失败时把原因说清楚，弹窗不关", async () => {
    const { ApiError } = await import("../../api/client");
    batchUpdateBooks.mockRejectedValueOnce(new ApiError("http", "请求超时", 408, "timeout"));
    renderPage();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    fireEvent.click(screen.getByTestId("admin-batch-serial"));
    fireEvent.click(screen.getByTestId("admin-batch-confirm"));

    await waitFor(() => expect(screen.getByTestId("admin-batch-error").textContent).toContain("批量更新没能完成"));
    expect(screen.getByTestId("admin-batch-dialog")).toBeTruthy();
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

  it("筛选条旁能打开标签管理弹窗，列表里的标签就在里面", () => {
    renderPage();
    expect(screen.queryByTestId("tag-manage-dialog")).toBeNull();
    fireEvent.click(screen.getByTestId("admin-tag-manage"));
    expect(screen.getByTestId("tag-manage-dialog").textContent).toContain("武侠");
    fireEvent.click(screen.getByTestId("tag-manage-close"));
    expect(screen.queryByTestId("tag-manage-dialog")).toBeNull();
  });

  it("C-42：取消选择后焦点回到表头全选，不是掉到 body", async () => {
    renderPage();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    expect(screen.getByTestId("admin-batch-bar").textContent).toContain("已选 1 本");
    fireEvent.click(screen.getByTestId("admin-batch-clear"));
    await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("admin-select-page")));
  });

  it("C-34：关闭标签管理弹层后焦点回入口按钮", async () => {
    renderPage();
    const trigger = screen.getByTestId("admin-tag-manage");
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTestId("tag-manage-close"));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it("批量条按钮文案按契约（批量改作者 / 批量改标签 / 批量改连载状态 / 批量删除）", () => {
    renderPage();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    const bar = screen.getByTestId("admin-batch-bar").textContent ?? "";
    expect(bar).toContain("批量改作者");
    expect(bar).toContain("批量改标签");
    expect(bar).toContain("批量改连载状态");
    expect(bar).toContain("批量删除");
  });

  it("勾选的行带 is-picked 类（行底色反馈）", () => {
    renderPage();
    fireEvent.click(screen.getByTestId("admin-book-pick-7"));
    expect(screen.getByTestId("admin-book-row-7").className).toContain("is-picked");
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
