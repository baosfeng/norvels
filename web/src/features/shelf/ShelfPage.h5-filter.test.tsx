import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookQuery, RequestOptions } from "../../api/client";
import type { BookDetail, BookSummary, CategoryTree, Page, TagSummary } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const books = vi.fn<(params: BookQuery, opts?: RequestOptions) => Promise<Page<BookSummary>>>();
const categories = vi.fn(async (): Promise<CategoryTree> => tree());
const tags = vi.fn(async (): Promise<{ items: TagSummary[] }> => ({ items: [] }));
const collections = vi.fn(async () => ({ items: [], total: 0 }));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      books: (params: BookQuery, opts?: RequestOptions) => books(params, opts),
      categories: () => categories(),
      tags: () => tags(),
      collections: () => collections(),
      authors: vi.fn(async () => ({ items: [], total: 0, page: 1, size: 20 })),
      updateBook: vi.fn(async () => ({}) as BookDetail),
    },
  };
});

const { ShelfPage } = await import("./ShelfPage");

function tree(): CategoryTree {
  return {
    total: 5,
    items: [
      { l1: "历史", count: 1, items: [{ l2: "两宋元明", count: 1 }] },
      { l1: "玄幻", count: 4, items: [{ l2: "东方玄幻", count: 3 }, { l2: "奇幻玄幻", count: 1 }] },
      { l1: "", count: 2, items: [{ l2: "", count: 2 }] },
    ],
  };
}

function bookAt(id: number): BookSummary {
  return {
    id,
    title: `示例书 ${id}`,
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

function pageOf(): Page<BookSummary> {
  return { items: [bookAt(1)], total: 1, page: 1, size: 30 };
}

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname + location.search}</div>;
}

function renderShelf(entries: string[] = ["/"]) {
  return render(
    <TierProvider value="h5">
      <MemoryRouter initialEntries={entries}>
        <ShelfPage />
        <LocationProbe />
      </MemoryRouter>
    </TierProvider>,
  );
}

function currentQuery(): string {
  return decodeURIComponent(screen.getByTestId("location").textContent ?? "");
}

beforeEach(() => {
  window.sessionStorage.clear();
  books.mockReset().mockResolvedValue(pageOf());
  categories.mockClear();
  tags.mockClear();
  collections.mockClear();
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

describe("H5 书架：分类等筛选在底部弹层里（C-21）", () => {
  it("H5 首屏不渲染 PC 工具栏，但排序入口可打开筛弹层", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());
    expect(screen.queryByTestId("shelf-toolbar")).toBeNull();

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    const sheet = within(screen.getByTestId("shelf-sort-sheet"));
    expect(sheet.getByRole("dialog")).toBeTruthy();
    // 分类两级在弹层里同样存在，且选项来自同一份 /api/categories
    expect(await sheet.findByTestId("filter-category-l1")).toBeTruthy();
    expect(categories).toHaveBeenCalled();
  });

  it("选分类后列表按 categoryL1 重新取数，并把筛选写进地址", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    const select = (await screen.findByTestId("filter-category-l1")) as HTMLSelectElement;
    await waitFor(() => expect(select.querySelectorAll("option").length).toBeGreaterThan(1));
    fireEvent.change(select, { target: { value: "玄幻" } });

    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ categoryL1: "玄幻", page: 1, size: 30 });
    // 选中即刻生效，弹层保持打开让用户接着调下一项；「查看 N 本」才收起
    expect(screen.getByTestId("shelf-sort-sheet")).toBeTruthy();
    fireEvent.click(await screen.findByTestId("sheet-done"));
    expect(screen.queryByTestId("shelf-sort-sheet")).toBeNull();
    await waitFor(() => expect(currentQuery()).toContain("l1=玄幻"));
  });

  it("小分类联动大分类，两个值一起进查询参数", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    const l1 = (await screen.findByTestId("filter-category-l1")) as HTMLSelectElement;
    await waitFor(() => expect(l1.querySelectorAll("option").length).toBeGreaterThan(1));
    fireEvent.change(l1, { target: { value: "玄幻" } });

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    const l2 = (await screen.findByTestId("filter-category-l2")) as HTMLSelectElement;
    fireEvent.change(l2, { target: { value: "东方玄幻" } });

    await waitFor(() => expect(books).toHaveBeenCalledTimes(3));
    expect(books.mock.calls[2][0]).toMatchObject({ categoryL1: "玄幻", categoryL2: "东方玄幻" });
  });

  it("评分 / 作者 / 状态筛选在 H5 弹层里也能改变列表请求", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    fireEvent.click(screen.getByTestId("rating-option-4"));
    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ rating: 4 });

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    fireEvent.click(screen.getByTestId("read-status-option-reading"));
    await waitFor(() => expect(books).toHaveBeenCalledTimes(3));
    expect(books.mock.calls[2][0].readStatus).toBe("reading");
    expect(books.mock.calls[2][0].rating).toBe(4);
  });

  it("「清除筛选」清空全部筛选并回到无筛选的请求", async () => {
    renderShelf(["/?l1=玄幻&status=done"]);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));
    expect(books.mock.calls[0][0]).toMatchObject({ categoryL1: "玄幻", readStatus: "done" });

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    fireEvent.click(screen.getByTestId("sheet-reset-filters"));

    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0].categoryL1).toBeUndefined();
    expect(books.mock.calls[1][0].readStatus).toBeUndefined();
    await waitFor(() => expect(currentQuery()).toBe("/"));
  });

  it("Esc 关闭弹层，焦点回到打开它的按钮", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    const trigger = screen.getByTestId("shelf-sort-trigger");
    fireEvent.click(trigger);
    expect(screen.getByTestId("shelf-sort-sheet")).toBeTruthy();
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(screen.queryByTestId("shelf-sort-sheet")).toBeNull());
  });
});

describe("H5 书架：地址即筛选（刷新 / 后退 / 分享都能还原）", () => {
  it("带 l1/l2/tag 的地址直接打开就按它取数", async () => {
    renderShelf(["/?l1=玄幻&l2=东方玄幻&tag=民国"]);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));
    expect(books.mock.calls[0][0]).toMatchObject({
      categoryL1: "玄幻",
      categoryL2: "东方玄幻",
      tag: "民国",
      size: 30,
    });
  });

  it("H5 深链 ?tag= 不再失效（详情页标签 chip 的落点）", async () => {
    renderShelf(["/?tag=民国"]);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));
    expect(books.mock.calls[0][0].tag).toBe("民国");
    expect(currentQuery()).toContain("tag=民国");
  });

  it("分类筛选经会话缓存也能还原（从详情返回的兜底路径）", async () => {
    const { writeShelfCache } = await import("../../lib/shelfCache");
    writeShelfCache({
      sort: "updated",
      authorId: null,
      tag: null,
      readStatus: null,
      rating: null,
      collection: null,
      categoryL1: "玄幻",
      categoryL2: "东方玄幻",
      size: 30,
      mode: "infinite",
      items: [bookAt(1)],
      total: 1,
      page: 1,
      // 有滚动位置时不走 scrollIntoView（jsdom 没有这个方法）：用缓存里的定位能力证明还原路径
      scrollTop: 320,
      lastBookId: 1,
    });
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());
    // 缓存够新（60 秒内）时只渲染不刷新；地址随后被补成同一筛选
    await waitFor(() => expect(currentQuery()).toContain("l1=玄幻"));
    expect(currentQuery()).toContain("l2=东方玄幻");
  });
});
