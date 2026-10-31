import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Collection } from "../../api/types";

const collections = vi.fn<() => Promise<{ items: Collection[] }>>();
const createCollection = vi.fn();
const renameCollection = vi.fn();
const deleteCollection = vi.fn();
const books = vi.fn();
const replaceCollectionBooks = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      collections: () => collections(),
      createCollection: (name: string, opts?: unknown) => createCollection(name, opts),
      renameCollection: (id: number, name: string, opts?: unknown) => renameCollection(id, name, opts),
      deleteCollection: (id: number, opts?: unknown) => deleteCollection(id, opts),
      books: (params: unknown, opts?: unknown) => books(params, opts),
      replaceCollectionBooks: (id: number, bookIds: number[], opts?: unknown) =>
        replaceCollectionBooks(id, bookIds, opts),
    },
  };
});

/** 成员/候选书的最小形状（BookSummary 的字段多，测试只关心这几个）。 */
function book(id: number, title: string) {
  return {
    id,
    title,
    authorName: `作者${id}`,
    chapterCount: id * 10,
    wordCount: 100000,
    hasCover: false,
    updatedAt: "2026-10-24T20:25:26.409Z",
  };
}

/** 书库分页返回。 */
function page(items: ReturnType<typeof book>[], total = items.length) {
  return { items, total, page: 1, size: 24 };
}

const { CollectionsPage } = await import("./CollectionsPage");

function items(): Collection[] {
  return [
    { id: 1, name: "想读", bookCount: 3 },
    { id: 2, name: "经典", bookCount: 0 },
  ];
}

beforeEach(() => {
  collections.mockReset();
  createCollection.mockReset();
  renameCollection.mockReset();
  deleteCollection.mockReset();
  books.mockReset();
  replaceCollectionBooks.mockReset();
  collections.mockResolvedValue({ items: items() });
  books.mockResolvedValue(page([]));
  replaceCollectionBooks.mockResolvedValue(undefined);
});

describe("书单页：书单本身的管理面", () => {
  it("页标题「书单」+ 一句扁平收藏夹的说明", async () => {
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("书单");
    expect(document.body.textContent).toContain("书单是扁平收藏夹，一本书可以同时放在多个书单里");
  });

  it("没有书单时给空态与新建指引", async () => {
    collections.mockResolvedValue({ items: [] });
    render(<CollectionsPage />);

    await waitFor(() => expect(screen.getByTestId("collections-empty")).toBeTruthy());
    expect(screen.getByTestId("collections-empty").textContent).toContain("还没有书单");
    expect(screen.queryByTestId("collections-list")).toBeNull();
  });

  it("列表每行给出名字与本数", async () => {
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    expect(screen.getByTestId("collections-count-1").textContent).toBe("3 本");
    const row = screen.getByTestId("collections-row-2");
    expect(row.textContent).toContain("经典");
    expect(row.textContent).toContain("0 本");
  });

  it("新建：去掉首尾空白后下发名字，成功后清空输入并重拉列表", async () => {
    createCollection.mockResolvedValue({ id: 3, name: "今年想读", bookCount: 0 });
    collections
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValue({ items: [{ id: 3, name: "今年想读", bookCount: 0 }] });
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-empty")).toBeTruthy());

    fireEvent.change(screen.getByTestId("collections-new-input"), { target: { value: "  今年想读  " } });
    fireEvent.click(screen.getByTestId("collections-create"));

    await waitFor(() => expect(createCollection).toHaveBeenCalledWith("今年想读", undefined));
    await waitFor(() => expect(screen.getByTestId("collections-row-3")).toBeTruthy());
    expect((screen.getByTestId("collections-new-input") as HTMLInputElement).value).toBe("");
  });

  it("重名（409）给人话提示，不静默失败", async () => {
    const { ApiError } = await vi.importActual<typeof import("../../api/client")>("../../api/client");
    createCollection.mockRejectedValue(new ApiError("http", "conflict", 409, "conflict"));
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    fireEvent.change(screen.getByTestId("collections-new-input"), { target: { value: "想读" } });
    fireEvent.click(screen.getByTestId("collections-create"));

    await waitFor(() =>
      expect(screen.getByTestId("collections-create-error").textContent).toContain("已经有同名书单"),
    );
  });

  it("改名：保存后下发新名字并重拉列表", async () => {
    renameCollection.mockResolvedValue(undefined);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collections-rename-1"));
    fireEvent.change(screen.getByTestId("collections-rename-input-1"), { target: { value: "待读" } });
    fireEvent.click(screen.getByTestId("collections-rename-save-1"));

    await waitFor(() => expect(renameCollection).toHaveBeenCalledWith(1, "待读", undefined));
    await waitFor(() => expect(collections).toHaveBeenCalledTimes(2));
  });

  it("删除要先确认：点「删除」只进确认态，确认后才发请求", async () => {
    deleteCollection.mockResolvedValue(undefined);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collections-delete-1"));
    expect(deleteCollection).not.toHaveBeenCalled();
    expect(within(screen.getByTestId("collections-row-1")).getByText("确认删除")).toBeTruthy();

    fireEvent.click(screen.getByTestId("collections-delete-confirm-1"));
    await waitFor(() => expect(deleteCollection).toHaveBeenCalledWith(1, undefined));
    await waitFor(() => expect(collections).toHaveBeenCalledTimes(2));
  });

  it("确认态点「取消」不发请求，回到普通行", async () => {
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collections-delete-2"));
    fireEvent.click(within(screen.getByTestId("collections-row-2")).getByText("取消"));

    expect(deleteCollection).not.toHaveBeenCalled();
    expect(screen.getByTestId("collections-delete-2")).toBeTruthy();
  });
});

describe("书单成员：查看 / 添加 / 移出", () => {
  /** 成员取数走 GET /api/books?collection=id；搜索与候选列表走不带 collection 的那次。 */
  function stubBooks(members: ReturnType<typeof book>[], candidates: ReturnType<typeof book>[]) {
    books.mockImplementation(async (params: { collection?: number; q?: string }) => {
      if (params.collection) return page(members);
      if (params.q) return page(candidates.filter((b) => b.title.includes(params.q ?? "")));
      return page(candidates);
    });
  }

  it("点「成员 N 本」展开列出书：封面位 + 书名 + 作者 + 章节数", async () => {
    stubBooks([book(11, "第一本"), book(22, "第二本")], []);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collections-members-toggle-1"));

    await waitFor(() => expect(screen.getByTestId("collection-members-1")).toBeTruthy());
    const row = screen.getByTestId("collection-member-1-11");
    expect(row.textContent).toContain("第一本");
    expect(row.textContent).toContain("作者11 · 110 章");
    expect(books.mock.calls.some(([params]) => params?.collection === 1)).toBe(true);
  });

  it("再点一次收起成员，不重复请求", async () => {
    stubBooks([book(11, "第一本")], []);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collections-members-toggle-1"));
    await waitFor(() => expect(screen.getByTestId("collection-members-1")).toBeTruthy());
    const calls = books.mock.calls.length;
    fireEvent.click(screen.getByTestId("collections-members-toggle-1"));
    expect(screen.queryByTestId("collection-members-1")).toBeNull();
    expect(books.mock.calls.length).toBe(calls);
  });

  it("移出一本：先取全量成员，再按「其余成员」全量写回", async () => {
    stubBooks([book(11, "第一本"), book(22, "第二本")], []);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());
    fireEvent.click(screen.getByTestId("collections-members-toggle-1"));
    await waitFor(() => expect(screen.getByTestId("collection-member-1-11")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collection-member-remove-1-11"));

    await waitFor(() => expect(replaceCollectionBooks).toHaveBeenCalledWith(1, [22], undefined));
  });

  it("空成员给「这个书单还没有书」+ 添加指引", async () => {
    stubBooks([], []);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());
    fireEvent.click(screen.getByTestId("collections-members-toggle-2"));

    await waitFor(() => expect(screen.getByTestId("collection-members-empty-2")).toBeTruthy());
    expect(screen.getByTestId("collection-members-empty-2").textContent).toContain("这个书单还没有书");
  });

  it("成员读取失败给错误面板与重试", async () => {
    books.mockRejectedValue(new Error("离线"));
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());
    fireEvent.click(screen.getByTestId("collections-members-toggle-1"));

    await waitFor(() => expect(screen.getByTestId("collection-members-error-1")).toBeTruthy());
    expect(screen.getByTestId("collection-members-error-1").textContent).toContain("这个书单的书没能读取");
  });

  it("添加书籍：勾两本后合并写回（原有成员 + 新书，按 id 去重）", async () => {
    stubBooks([book(11, "第一本")], [book(11, "第一本"), book(31, "候选甲"), book(32, "候选乙")]);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collections-add-books-1"));
    await waitFor(() => expect(screen.getByTestId("add-books-dialog")).toBeTruthy());

    fireEvent.click(screen.getByTestId("add-books-pick-31"));
    fireEvent.click(screen.getByTestId("add-books-pick-32"));
    expect(screen.getByTestId("add-books-picked").textContent).toBe("2");
    fireEvent.click(screen.getByTestId("add-books-confirm"));

    await waitFor(() => expect(replaceCollectionBooks).toHaveBeenCalledWith(1, [11, 31, 32], undefined));
    await waitFor(() => expect(screen.queryByTestId("add-books-dialog")).toBeNull());
  });

  it("添加弹层里搜索：关键词变化即按 q 重新取书", async () => {
    stubBooks([], [book(31, "候选甲")]);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collections-add-books-1"));
    await waitFor(() => expect(screen.getByTestId("add-books-list")).toBeTruthy());
    fireEvent.change(screen.getByTestId("add-books-search"), { target: { value: "甲" } });

    await waitFor(() =>
      expect(books.mock.calls.some(([params]) => params?.q === "甲")).toBe(true),
    );
  });

  it("关闭弹层后焦点回到「添加书籍」按钮", async () => {
    stubBooks([], [book(31, "候选甲")]);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());
    const trigger = screen.getByTestId("collections-add-books-2") as HTMLButtonElement;
    trigger.focus();
    fireEvent.click(trigger);
    await waitFor(() => expect(screen.getByTestId("add-books-dialog")).toBeTruthy());
    fireEvent.click(screen.getByTestId("add-books-close"));
    await waitFor(() => expect(screen.queryByTestId("add-books-dialog")).toBeNull());
    expect(document.activeElement).toBe(trigger);
  });

  it("已在书单里的书标「已加入」且不可勾选（避免重复勾选）", async () => {
    stubBooks([book(11, "第一本")], [book(11, "第一本"), book(31, "候选甲")]);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collections-add-books-1"));
    await waitFor(() => expect(screen.getByTestId("add-books-dialog")).toBeTruthy());

    await waitFor(() => expect(screen.getByTestId("add-books-member-11")).toBeTruthy());
    expect(screen.getByTestId("add-books-member-11").textContent).toBe("已加入");
    expect((screen.getByTestId("add-books-pick-11") as HTMLInputElement).disabled).toBe(true);

    fireEvent.click(screen.getByTestId("add-books-item-11"));
    expect(screen.getByTestId("add-books-picked").textContent).toBe("0");
  });

  it("现有成员读不到时给一句兜底说明，仍可勾选（保存时去重）", async () => {
    books.mockImplementation(async (params: { collection?: number }) => {
      if (params.collection) throw new Error("离线");
      return page([book(31, "候选甲")]);
    });
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());
    fireEvent.click(screen.getByTestId("collections-add-books-1"));

    await waitFor(() => expect(screen.getByTestId("add-books-members-unknown")).toBeTruthy());
    expect((screen.getByTestId("add-books-pick-31") as HTMLInputElement).disabled).toBe(false);
  });

  it("打开弹层锁背景滚动，关闭后恢复", async () => {
    stubBooks([], [book(31, "候选甲")]);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());
    expect(document.body.style.overflow).toBe("");

    fireEvent.click(screen.getByTestId("collections-add-books-2"));
    await waitFor(() => expect(screen.getByTestId("add-books-dialog")).toBeTruthy());
    expect(document.body.style.overflow).toBe("hidden");

    fireEvent.click(screen.getByTestId("add-books-close"));
    await waitFor(() => expect(screen.queryByTestId("add-books-dialog")).toBeNull());
    expect(document.body.style.overflow).toBe("");
  });

  it("没勾选时「添加」不可点", async () => {
    stubBooks([], [book(31, "候选甲")]);
    render(<CollectionsPage />);
    await waitFor(() => expect(screen.getByTestId("collections-list")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collections-add-books-1"));
    await waitFor(() => expect(screen.getByTestId("add-books-dialog")).toBeTruthy());
    expect((screen.getByTestId("add-books-confirm") as HTMLButtonElement).disabled).toBe(true);
  });
});