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
const authors = vi.fn(async () => ({
  items: [{ id: 1, name: "示例作者", bookCount: 3 }],
  total: 1,
  page: 1,
  size: 20,
}));

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
      authors: () => authors(),
      updateBook: vi.fn(async () => ({}) as BookDetail),
    },
  };
});

const { ShelfPage } = await import("./ShelfPage");

function tree(): CategoryTree {
  return {
    total: 7,
    items: [
      { l1: "历史", count: 1, items: [{ l2: "两宋元明", count: 1 }] },
      { l1: "玄幻", count: 4, items: [{ l2: "东方玄幻", count: 3 }, { l2: "奇幻玄幻", count: 1 }] },
      // 没有细分的大分类：铺开时照样是大分类 chip，选中后那一行报「该分类暂无细分」
      { l1: "悬疑", count: 2, items: [] },
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
  authors.mockClear();
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
    // 分类在弹层里是铺开的 chip，不再是「要点开才知道有什么」的原生下拉
    expect(await sheet.findByTestId("category-option-l1-玄幻")).toBeTruthy();
    expect(sheet.queryByTestId("filter-category-l1")).toBeNull();
    expect(categories).toHaveBeenCalled();
  });

  it("大分类全量铺开：chip 数 = 接口节点数 + 「全部」，未分类不进候选", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    const box = await screen.findByTestId("category-l1-options");
    await waitFor(() => expect(box.querySelectorAll(".sheet-option")).toHaveLength(4)); // 全部 + 历史 + 玄幻 + 悬疑
    // 计数直接写在 chip 上，选之前就知道各分类有几本
    expect(box.textContent).toContain("玄幻（4）");
    expect(box.textContent).not.toContain("未分类");
    // 弹层里没有原生 select 可展开
    expect(document.querySelector('[data-testid="shelf-sort-sheet"] select')).toBeNull();
  });

  it("选分类后列表按 categoryL1 重新取数，并把筛选写进地址", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    fireEvent.click(await screen.findByTestId("category-option-l1-玄幻"));

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
    // 大分类选中后小分类就铺在它下面，不用关掉弹层再打开一次
    fireEvent.click(await screen.findByTestId("category-option-l1-玄幻"));
    const l2 = await screen.findByTestId("category-l2-options");
    expect(l2.querySelectorAll(".sheet-option")).toHaveLength(3); // 全部 + 东方玄幻 + 奇幻玄幻
    fireEvent.click(screen.getByTestId("category-option-l2-东方玄幻"));

    await waitFor(() => expect(books).toHaveBeenCalledTimes(3));
    expect(books.mock.calls[2][0]).toMatchObject({ categoryL1: "玄幻", categoryL2: "东方玄幻" });
    expect(screen.getByTestId("category-option-l2-东方玄幻").getAttribute("aria-pressed")).toBe("true");
  });

  it("换大分类时小分类跟着回到「全部」", async () => {
    renderShelf(["/?l1=玄幻&l2=东方玄幻"]);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    fireEvent.click(await screen.findByTestId("category-option-l1-历史"));

    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ categoryL1: "历史" });
    expect(books.mock.calls[1][0].categoryL2).toBeUndefined();
  });

  it("没有细分的大分类：那一行说「该分类暂无细分」", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    fireEvent.click(await screen.findByTestId("category-option-l1-悬疑"));

    expect((await screen.findByTestId("category-l2-empty")).textContent).toBe("该分类暂无细分");
    expect(screen.queryByTestId("category-l2-options")).toBeNull();
  });

  it("作者只留「已选作者 ×」的清除入口，不再铺一屏作者 chip", async () => {
    renderShelf(["/?author=1"]);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    const clear = await screen.findByTestId("author-selected-clear");
    // 名字来自候选表；没有候选时退成「已选作者」，不编名字
    expect(clear.textContent).toBe("作者：示例作者 ×");
    expect(screen.queryByTestId("author-option-all")).toBeNull();

    fireEvent.click(clear);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0].authorId).toBeUndefined();
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
