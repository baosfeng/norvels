import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Collection } from "../../api/types";

const books = vi.fn();
const replaceCollectionBooks = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      books: (params: unknown, opts?: unknown) => books(params, opts),
      replaceCollectionBooks: (id: number, bookIds: number[], opts?: unknown) =>
        replaceCollectionBooks(id, bookIds, opts),
    },
  };
});

const { AddBooksDialog } = await import("./AddBooksDialog");

/** 候选书的最小形状（BookSummary 的字段多，测试只关心这几个）。 */
function book(id: number) {
  return {
    id,
    title: `书${id}`,
    authorName: `作者${id}`,
    chapterCount: id * 10,
    wordCount: 100000,
    hasCover: false,
    updatedAt: "2026-10-24T20:25:26.409Z",
  };
}

function many(count: number) {
  return Array.from({ length: count }, (_, i) => book(i + 1));
}

type BookArgs = { collection?: number; q?: string; page?: number; size?: number };

const collection: Collection = { id: 1, name: "想读", bookCount: 0 };

/**
 * 书库分页桩：成员读取走 collection 分支（一次给全量），候选书按 q 过滤后按 page/size 切片。
 * failPageOnce 指定的那一页会失败一次，用来验证「行内提示 + 重试」。
 */
function stubBooks(all: ReturnType<typeof book>[], members: ReturnType<typeof book>[] = []) {
  const rejected = new Set<number>();
  const failPageOnce = (page: number) => rejected.add(page);
  books.mockImplementation(async (params: BookArgs) => {
    if (params.collection) {
      return { items: members, total: members.length, page: params.page ?? 1, size: params.size ?? 200 };
    }
    const pageNo = params.page ?? 1;
    const size = params.size ?? 20;
    if (rejected.delete(pageNo)) throw new Error("离线");
    const matched = params.q ? all.filter((item) => item.title.includes(params.q as string)) : all;
    return {
      items: matched.slice((pageNo - 1) * size, pageNo * size),
      total: matched.length,
      page: pageNo,
      size,
    };
  });
  return { failPageOnce };
}

/** 候选书的取数参数（排除成员读取）。 */
function candidateCalls(): BookArgs[] {
  return books.mock.calls.map(([params]) => params as BookArgs).filter((params) => !params.collection);
}

function renderDialog() {
  render(<AddBooksDialog collection={collection} onClose={() => {}} onAdded={() => {}} />);
}

beforeEach(() => {
  books.mockReset();
  replaceCollectionBooks.mockReset();
});

describe("添加书籍弹层：候选书分页", () => {
  it("滚到底自动取下一页，加载完显示「已全部显示（共 N 本）」", async () => {
    stubBooks(many(45));
    renderDialog();
    await waitFor(() => expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 20 / 共 45 本"));
    expect(screen.queryByTestId("add-books-more-done")).toBeNull();

    fireEvent.scroll(screen.getByTestId("add-books-body"));
    expect(screen.getByTestId("add-books-more-loading").textContent).toBe("正在加载…");

    await waitFor(() => expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 40 / 共 45 本"));
    expect(screen.queryByTestId("add-books-more-done")).toBeNull();

    fireEvent.scroll(screen.getByTestId("add-books-body"));
    await waitFor(() => expect(screen.getByTestId("add-books-more-done").textContent).toBe("已全部显示（共 45 本）"));
    expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 45 / 共 45 本");
    expect(candidateCalls().map((params) => params.page)).toEqual([1, 2, 3]);
    expect(candidateCalls().every((params) => params.size === 20)).toBe(true);
  });

  it("关键词变化回到第 1 页并清空已加载页，已勾选的书保留", async () => {
    stubBooks(many(45));
    renderDialog();
    await waitFor(() => expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 20 / 共 45 本"));

    fireEvent.scroll(screen.getByTestId("add-books-body"));
    await waitFor(() => expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 40 / 共 45 本"));

    fireEvent.click(screen.getByTestId("add-books-pick-1"));
    fireEvent.click(screen.getByTestId("add-books-pick-21"));
    expect(screen.getByTestId("add-books-picked").textContent).toBe("2");

    fireEvent.change(screen.getByTestId("add-books-search"), { target: { value: "书" } });

    await waitFor(() => expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 20 / 共 45 本"));
    expect(screen.queryByTestId("add-books-item-40")).toBeNull();
    expect(candidateCalls().at(-1)).toMatchObject({ q: "书", page: 1 });
    expect(screen.getByTestId("add-books-picked").textContent).toBe("2");
  });

  it("翻页失败给行内提示与重试，已加载的书仍可勾选", async () => {
    const { failPageOnce } = stubBooks(many(45));
    failPageOnce(2);
    renderDialog();
    await waitFor(() => expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 20 / 共 45 本"));

    fireEvent.scroll(screen.getByTestId("add-books-body"));

    await waitFor(() => expect(screen.getByTestId("add-books-more-error")).toBeTruthy());
    expect(screen.getByTestId("add-books-more-error").textContent).toContain("还有书没能读出来");
    expect(screen.getByTestId("add-books-item-20")).toBeTruthy();
    expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 20 / 共 45 本");

    fireEvent.click(screen.getByTestId("add-books-pick-20"));
    expect(screen.getByTestId("add-books-picked").textContent).toBe("1");

    fireEvent.click(screen.getByTestId("add-books-more-retry"));
    await waitFor(() => expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 40 / 共 45 本"));
    expect(screen.queryByTestId("add-books-more-error")).toBeNull();
    expect(screen.getByTestId("add-books-picked").textContent).toBe("1");
    expect(candidateCalls().map((params) => params.page)).toEqual([1, 2, 2]);
  });

  it("首屏失败同样走行内提示与重试，不整块变错误态", async () => {
    const { failPageOnce } = stubBooks(many(45));
    failPageOnce(1);
    renderDialog();

    await waitFor(() => expect(screen.getByTestId("add-books-more-error")).toBeTruthy());
    expect(screen.queryByTestId("add-books-loading")).toBeNull();
    expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 0 / 共 0 本");

    fireEvent.click(screen.getByTestId("add-books-more-retry"));
    await waitFor(() => expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 20 / 共 45 本"));
  });

  it("无匹配时顶部显示「已显示 0 / 共 0 本」并给空态", async () => {
    stubBooks(many(45));
    renderDialog();
    await waitFor(() => expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 20 / 共 45 本"));

    fireEvent.change(screen.getByTestId("add-books-search"), { target: { value: "没有这本" } });

    await waitFor(() => expect(screen.getByTestId("add-books-empty")).toBeTruthy());
    expect(screen.getByTestId("add-books-shown").textContent).toContain("已显示 0 / 共 0 本");
  });

  it("已在书单里的书标「已加入」且不可勾选", async () => {
    stubBooks(many(45), [book(2)]);
    renderDialog();

    await waitFor(() => expect(screen.getByTestId("add-books-member-2")).toBeTruthy());
    expect(screen.getByTestId("add-books-member-2").textContent).toBe("已加入");
    expect((screen.getByTestId("add-books-pick-2") as HTMLInputElement).disabled).toBe(true);

    fireEvent.click(screen.getByTestId("add-books-item-2"));
    expect(screen.getByTestId("add-books-picked").textContent).toBe("0");
  });
});
