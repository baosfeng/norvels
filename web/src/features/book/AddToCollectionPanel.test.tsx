import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookSummary, Collection, Page } from "../../api/types";
import { ToastHost } from "../../lib/toast";
import { toggleMember, type CollectionMembers } from "./collectionMembers";

const collections = vi.fn();
const books = vi.fn();
const replaceCollectionBooks = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      collections: (opts?: unknown) => collections(opts),
      books: (params: unknown, opts?: unknown) => books(params, opts),
      replaceCollectionBooks: (id: number, bookIds: number[], opts?: unknown) => replaceCollectionBooks(id, bookIds, opts),
    },
  };
});

const { AddToCollectionPanel } = await import("./AddToCollectionPanel");
const { fetchCollectionMembers } = await import("./collectionMembers");

function coll(id: number, name: string, bookCount = 0): Collection {
  return { id, name, bookCount };
}

function bookPage(ids: number[], total = ids.length): Page<BookSummary> {
  return {
    items: ids.map((id) => ({ id, title: `书 ${id}` }) as BookSummary),
    total,
    page: 1,
    size: 200,
  };
}

function renderPanel(bookId = 9) {
  return render(
    <>
      <AddToCollectionPanel bookId={bookId} bookTitle="示例书" onClose={() => {}} />
      <ToastHost />
    </>,
  );
}

beforeEach(() => {
  collections.mockReset();
  books.mockReset();
  replaceCollectionBooks.mockReset();
});

describe("书单成员换算（纯函数）", () => {
  it("加上/去掉这一本，其它成员保持不动", () => {
    const members: CollectionMembers = { ids: [1, 2], complete: true };
    expect(toggleMember(members, 9, true)).toEqual([1, 2, 9]);
    expect(toggleMember(members, 1, false)).toEqual([2]);
  });

  it("成员取不完整时拒绝改动（返回 null）", () => {
    expect(toggleMember({ ids: [1], complete: false }, 9, true)).toBeNull();
  });

  it("分页取全成员：到 total 为止", async () => {
    books.mockResolvedValueOnce(bookPage([1, 2], 2));
    await expect(fetchCollectionMembers(3)).resolves.toEqual({ ids: [1, 2], complete: true });
    expect(books.mock.calls[0][0]).toMatchObject({ collection: 3, page: 1, size: 200 });
  });
});

describe("详情页「加入书单」面板", () => {
  it("列出书单并回显当前是否已加入", async () => {
    collections.mockResolvedValue({ items: [coll(1, "想读", 1), coll(2, "经典", 0)] });
    books.mockImplementation(async (params: { collection?: number }) =>
      params.collection === 1 ? bookPage([9]) : bookPage([]),
    );
    renderPanel();

    await waitFor(() => expect(screen.getByTestId("add-to-collection-list")).toBeTruthy());
    expect((screen.getByTestId("collection-check-1") as HTMLInputElement).checked).toBe(true);
    expect((screen.getByTestId("collection-check-2") as HTMLInputElement).checked).toBe(false);
  });

  it("勾选即整份回写（已有成员 + 这一本）", async () => {
    collections.mockResolvedValue({ items: [coll(2, "经典", 1)] });
    books.mockResolvedValue(bookPage([7]));
    replaceCollectionBooks.mockResolvedValue(undefined);
    renderPanel(9);

    await waitFor(() => expect(screen.getByTestId("collection-check-2")).toBeTruthy());
    fireEvent.click(screen.getByTestId("collection-check-2"));

    await waitFor(() => expect(replaceCollectionBooks).toHaveBeenCalledTimes(1));
    expect(replaceCollectionBooks.mock.calls[0][0]).toBe(2);
    expect(replaceCollectionBooks.mock.calls[0][1].sort()).toEqual([7, 9]);
    expect((screen.getByTestId("collection-check-2") as HTMLInputElement).checked).toBe(true);
  });

  it("失败回滚勾选并提示", async () => {
    collections.mockResolvedValue({ items: [coll(2, "经典", 1)] });
    books.mockResolvedValue(bookPage([7]));
    replaceCollectionBooks.mockRejectedValueOnce(new Error("离线"));
    renderPanel(9);

    await waitFor(() => expect(screen.getByTestId("collection-check-2")).toBeTruthy());
    fireEvent.click(screen.getByTestId("collection-check-2"));

    await waitFor(() => expect(screen.getByTestId("toast").textContent).toContain("书单没能保存"));
    expect((screen.getByTestId("collection-check-2") as HTMLInputElement).checked).toBe(false);
  });

  it("一个书单都没有时给新建指引", async () => {
    collections.mockResolvedValue({ items: [] });
    renderPanel();
    await waitFor(() => expect(screen.getByTestId("add-to-collection-empty")).toBeTruthy());
    expect(screen.getByTestId("add-to-collection-empty").textContent).toContain("还没有书单");
  });
});
