import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookQuery, RequestOptions } from "../../api/client";
import type { BookDetail, BookSummary, CategoryTree, Page, ShelfItem, TagSummary } from "../../api/types";
import { AuthContext, type AuthState } from "../../lib/auth";
import { TierProvider } from "../../lib/tier";

const books = vi.fn<(params: BookQuery, opts?: RequestOptions) => Promise<Page<BookSummary>>>();
/** 分组计数：与列表分开的取数路径（api.bookCount），所以这里单独桩 */
const bookCount = vi.fn<(params: BookQuery) => Promise<number>>();
const categories = vi.fn(async (): Promise<CategoryTree> => tree());
const tags = vi.fn(async (): Promise<{ items: TagSummary[] }> => ({ items: [{ id: 9, name: "民国", bookCount: 2 }] }));
const collections = vi.fn(async () => ({ items: [], total: 0 }));
const authors = vi.fn(async () => ({
  items: [{ id: 1, name: "示例作者", bookCount: 3 }],
  total: 1,
  page: 1,
  size: 20,
}));
/** 「我的书架」取数：默认空书架。已登录 + 无筛选时才会走它，作者筛选的用例里断言它没被调用。 */
const listShelf = vi.fn<(params: { page?: number; size?: number }, opts?: RequestOptions) => Promise<Page<ShelfItem>>>();
/** 单个作者详情：作者筛选回显用（候选表只有 id=1）。 */
const author = vi.fn<(id: number, opts?: RequestOptions) => Promise<{ id: number; name: string; bookCount: number; wordCount: number; books: [] }>>();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      books: (params: BookQuery, opts?: RequestOptions) => books(params, opts),
      bookCount: (params: BookQuery) => bookCount(params),
      categories: () => categories(),
      tags: () => tags(),
      collections: () => collections(),
      authors: () => authors(),
      listShelf: (params: { page?: number; size?: number }, opts?: RequestOptions) => listShelf(params, opts),
      author: (id: number, opts?: RequestOptions) => author(id, opts),
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
      // 没有细分的大分类：照样是大分类 chip，选中后那一行报「该分类暂无细分」
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

function renderShelf(entries: string[] = ["/"], auth?: AuthState) {
  const tree = (
    <TierProvider value="h5">
      <MemoryRouter initialEntries={entries}>
        <ShelfPage />
        <LocationProbe />
      </MemoryRouter>
    </TierProvider>
  );
  return render(auth ? <AuthContext.Provider value={auth}>{tree}</AuthContext.Provider> : tree);
}

/** 已登录态：默认视图是「我的书架」，用来复现「点作者却停在空书架」的原始路径。 */
function loggedInAuth(): AuthState {
  const user = { id: 2, name: "示例读者", createdAt: "2026-10-01T00:00:00.000Z", lastSeenAt: "" };
  return {
    user,
    loggedIn: true,
    loading: false,
    enter: async () => user,
    register: async () => user,
    login: async () => user,
    deleteAccount: async () => undefined,
    logout: async () => undefined,
    subscribe: () => () => undefined,
    clearLocal: () => undefined,
  };
}

function currentQuery(): string {
  return decodeURIComponent(screen.getByTestId("location").textContent ?? "");
}

/** 展开某个筛选分组（默认只有排序组是展开的） */
function openGroup(id: string) {
  const toggle = screen.getByTestId(`filter-group-toggle-${id}`);
  if (toggle.getAttribute("aria-expanded") !== "true") fireEvent.click(toggle);
  return screen.getByTestId(`filter-group-${id}`);
}

/**
 * 分组计数的默认桩：按「阅读状态 / 评分」给出**独立于列表**的数字，
 * 这样断言能证明数字来自计数接口，而不是碰巧等于列表总数。
 */
const STATUS_COUNTS: Record<string, number> = { want: 12, reading: 37, done: 5 };

beforeEach(() => {
  window.sessionStorage.clear();
  books.mockReset().mockResolvedValue(pageOf());
  bookCount.mockReset().mockImplementation(async (params: BookQuery) => {
    if (params.readStatus) return STATUS_COUNTS[params.readStatus] ?? 0;
    if (params.rating !== undefined) return params.rating * 10;
    return 838;
  });
  categories.mockClear();
  tags.mockClear();
  collections.mockClear();
  authors.mockClear();
  listShelf.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, size: 30 });
  author.mockReset().mockImplementation(async (id: number) => ({ id, name: "作者" + id, bookCount: 1, wordCount: 1000, books: [] }));
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

describe("H5 书架：筛选平铺在最外层（不再藏进底部弹层）", () => {
  it("首屏就能看到筛选条与分组标题，没有任何弹层与弹层按钮", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    expect(screen.getByTestId("shelf-filter-panel")).toBeTruthy();
    // 五个分组标题一眼可见；排序组默认展开，其余收起但标题在
    for (const id of ["sort", "status", "rating", "category", "tag"]) {
      expect(screen.getByTestId(`filter-group-${id}`)).toBeTruthy();
    }
    expect(screen.getByTestId("filter-group-toggle-sort").getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByTestId("filter-group-toggle-category").getAttribute("aria-expanded")).toBe("false");
    // 旧形态整体不存在：没有弹层、没有开弹层的入口、没有 PC 工具栏
    expect(screen.queryByTestId("shelf-sort-sheet")).toBeNull();
    expect(screen.queryByTestId("shelf-sort-trigger")).toBeNull();
    expect(screen.queryByTestId("shelf-filter-trigger")).toBeNull();
    expect(screen.queryByTestId("shelf-toolbar")).toBeNull();
  });

  it("排序默认展开且选项在页面上直接可点，选中即生效", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("sort-option-title"));
    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ sort: "title", page: 1, size: 30 });
    // 选中状态与分组标题右侧的「当前值」同步更新，收起也能读出选了什么
    expect(screen.getByTestId("sort-option-title").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByTestId("filter-group-toggle-sort").textContent).toContain("书名");
  });

  it("分组可折叠：点标题展开选项，再点收起；收起时当前值仍显示在标题行", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    const toggle = screen.getByTestId("filter-group-toggle-category");
    fireEvent.click(toggle);
    expect(screen.getByTestId("category-option-l1-玄幻")).toBeTruthy();
    expect(toggle.textContent).toContain("全部");

    fireEvent.click(await screen.findByTestId("category-option-l1-玄幻"));
    expect(toggle.textContent).toContain("玄幻");
    fireEvent.click(toggle);
    // 收起后选项离开无障碍树，但「当前值＝玄幻」还在标题行上
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(toggle.textContent).toContain("玄幻");
  });

  it("大分类全量铺开：chip 数 = 接口节点数 + 「全部」，未分类不进候选、计数直接写在 chip 上", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    const box = openGroup("category").querySelector("[data-testid=\"category-l1-options\"]") as HTMLElement;
    await waitFor(() => expect(box.querySelectorAll(".shelf-filter-chip")).toHaveLength(4)); // 全部 + 历史 + 玄幻 + 悬疑
    expect(box.textContent).toContain("玄幻（4）");
    expect(box.textContent).not.toContain("未分类");
    // 页面上没有原生下拉可展开（分组标题是按钮，不是 select）
    expect(document.querySelector("[data-testid=\"shelf-filter-panel\"] select")).toBeNull();
  });

  it("选分类后列表按 categoryL1 重新取数，并把筛选写进地址", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    openGroup("category");
    fireEvent.click(await screen.findByTestId("category-option-l1-玄幻"));

    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ categoryL1: "玄幻", page: 1, size: 30 });
    await waitFor(() => expect(currentQuery()).toContain("l1=玄幻"));
    // 平铺面板选完不收起（连续调几项才是一次完整筛选）
    expect(screen.getByTestId("shelf-filter-panel")).toBeTruthy();
  });

  it("小分类联动大分类，两个值一起进查询参数", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    openGroup("category");
    fireEvent.click(await screen.findByTestId("category-option-l1-玄幻"));
    const l2 = await screen.findByTestId("category-l2-options");
    expect(l2.querySelectorAll(".shelf-filter-chip")).toHaveLength(3); // 全部 + 东方玄幻 + 奇幻玄幻
    fireEvent.click(screen.getByTestId("category-option-l2-东方玄幻"));

    await waitFor(() => expect(books).toHaveBeenCalledTimes(3));
    expect(books.mock.calls[2][0]).toMatchObject({ categoryL1: "玄幻", categoryL2: "东方玄幻" });
    expect(screen.getByTestId("category-option-l2-东方玄幻").getAttribute("aria-pressed")).toBe("true");
  });

  it("换大分类时小分类跟着回到「全部」", async () => {
    renderShelf(["/?l1=玄幻&l2=东方玄幻"]);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));

    openGroup("category");
    fireEvent.click(await screen.findByTestId("category-option-l1-历史"));

    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ categoryL1: "历史" });
    expect(books.mock.calls[1][0].categoryL2).toBeUndefined();
  });

  it("没有细分的大分类：那一行说「该分类暂无细分」", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    openGroup("category");
    fireEvent.click(await screen.findByTestId("category-option-l1-悬疑"));

    expect((await screen.findByTestId("category-l2-empty")).textContent).toBe("该分类暂无细分");
    expect(screen.queryByTestId("category-l2-options")).toBeNull();
  });

  it("阅读状态与评分都带真实计数：数字来自计数接口，不是编的", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    openGroup("status");
    // 三个状态各自一个数字，且互不相同——证明是分别问出来的，不是拿总数糊上去
    await waitFor(() => expect(screen.getByTestId("read-status-option-reading").textContent).toBe("在读 37"));
    expect(screen.getByTestId("read-status-option-want").textContent).toBe("想读 12");
    expect(screen.getByTestId("read-status-option-done").textContent).toBe("已读 5");
    // 「全部」不显示数字：那就是底部「查看 N 本」的职责，同一处不写两遍
    expect(screen.getByTestId("read-status-option-all").textContent).toBe("全部");

    openGroup("rating");
    await waitFor(() => expect(screen.getByTestId("rating-option-4").textContent).toBe("4 星 40"));
    expect(screen.getByTestId("rating-option-5").textContent).toBe("5 星 50");
  });

  it("分组标签上的数字 == 选中它之后「查看 N 本」的数字（同一个查询）", async () => {
    // 计数与列表走同一套后端过滤：把两边都接到同一份假数据上，「在读」就是 37
    bookCount.mockImplementation(async (params: BookQuery) => (params.readStatus === "reading" ? 37 : 1));
    books.mockImplementation(async (params: BookQuery) => ({
      items: [bookAt(1)],
      total: params.readStatus === "reading" ? 37 : 1,
      page: 1,
      size: 30,
    }));
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    openGroup("status");
    await waitFor(() => expect(screen.getByTestId("read-status-option-reading").textContent).toBe("在读 37"));
    const labelCount = Number(screen.getByTestId("read-status-option-reading").textContent!.replace(/\D+/g, ""));

    fireEvent.click(screen.getByTestId("read-status-option-reading"));
    // 选中之后底部按钮报的就是同一个数：标签与结果数出自同一个查询条件
    await waitFor(() => expect(screen.getByTestId("panel-view-results").textContent).toBe(`查看 ${labelCount} 本`));
    expect(labelCount).toBe(37);
    expect(books.mock.calls.map((call) => call[0].readStatus)).toContain("reading");
  });

  it("计数接口失败时标签退回裸文字，不拿 0 冒充「没有」", async () => {
    bookCount.mockRejectedValue(new Error("离线"));
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    openGroup("status");
    // 取不到就不写数字；列表本身照常渲染，不因为计数失败而变成错误页
    expect(screen.getByTestId("read-status-option-reading").textContent).toBe("在读");
    expect(screen.getByTestId("shelf-live")).toBeTruthy();
  });

  it("作者只留「已选作者 ×」的清除入口，不再铺一屏作者 chip", async () => {
    renderShelf(["/?author=1"]);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));

    const clear = await screen.findByTestId("author-selected-clear");
    // 名字来自候选表；没有候选时退成「已选作者」，不编名字
    expect(clear.textContent).toBe("作者：示例作者 ×");
    expect(screen.queryByTestId("author-option-all")).toBeNull();

    fireEvent.click(clear);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0].authorId).toBeUndefined();
  });

  it("评分 / 状态筛选在平铺面板里也能改变列表请求", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    openGroup("rating");
    fireEvent.click(screen.getByTestId("rating-option-4"));
    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ rating: 4 });

    openGroup("status");
    fireEvent.click(screen.getByTestId("read-status-option-reading"));
    await waitFor(() => expect(books).toHaveBeenCalledTimes(3));
    expect(books.mock.calls[2][0].readStatus).toBe("reading");
    expect(books.mock.calls[2][0].rating).toBe(4);
  });

  it("「清除筛选」常驻且清空全部筛选并回到无筛选的请求", async () => {
    renderShelf(["/?l1=玄幻&status=done"]);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));
    expect(books.mock.calls[0][0]).toMatchObject({ categoryL1: "玄幻", readStatus: "done" });

    const clear = screen.getByTestId("panel-clear-filters");
    expect(clear.textContent).toBe("清除筛选");
    fireEvent.click(clear);

    // 七项筛选一起清：多项状态在同一个更新批次里落地，取数只认最后那次查询
    await waitFor(() => expect(books.mock.calls.length).toBeGreaterThan(1));
    const last = books.mock.calls[books.mock.calls.length - 1][0];
    expect(last.categoryL1).toBeUndefined();
    expect(last.readStatus).toBeUndefined();
    await waitFor(() => expect(currentQuery()).toBe("/"));
  });

  it("没有筛选时「清除筛选」仍在原位但禁用（不是消失）", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    const clear = screen.getByTestId("panel-clear-filters") as HTMLButtonElement;
    expect(clear.disabled).toBe(true);
    expect(screen.getByTestId("shelf-filter-panel").textContent).not.toContain("已选");
  });

  it("底部「查看 N 本」报出当前命中数，点它把列表带回顶部", async () => {
    books.mockResolvedValue(pageOf());
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    const view = screen.getByTestId("panel-view-results");
    expect(view.textContent).toBe("查看 1 本");
    fireEvent.click(view);
    expect(window.scrollTo).toHaveBeenCalled();
  });

  it("面板标题报出已选筛选项数，逐项 chip 可单独清除", async () => {
    renderShelf(["/?status=done&rating=4"]);
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    expect(screen.getByTestId("shelf-filter-panel").textContent).toContain("已选 2 项");
    const chips = await screen.findByTestId("shelf-filter-chips");
    expect(chips.textContent).toContain("已读");
    expect(chips.textContent).toContain("4 星");

    fireEvent.click(screen.getByTestId("filter-chip-rating"));
    await waitFor(() => expect(currentQuery()).not.toContain("rating=4"));
    expect(currentQuery()).toContain("status=done");
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

  it("已登录点作者昵称（?author=8）落在全部书：读 /api/books，不读空的我的书架", async () => {
    renderShelf(["/?author=8"], loggedInAuth());
    await waitFor(() => expect(books).toHaveBeenCalled());
    expect(books.mock.calls[0][0]).toMatchObject({ authorId: 8, size: 30 });
    expect(listShelf).not.toHaveBeenCalled();
    expect(screen.getByTestId("shelf-view-all").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByTestId("shelf-view-mine").getAttribute("aria-pressed")).toBe("false");
    // 作者不在候选前 50 位：名字由详情接口补，回显不能退化成「已选作者」
    await waitFor(() => expect(screen.getByTestId("author-selected-clear").textContent).toContain("作者：作者8"));
  });

  it("从带筛选的地址切到「我的书架」会清掉筛选参数（否则又被拉回全部书）", async () => {
    renderShelf(["/?author=8"]);
    await waitFor(() => expect(books).toHaveBeenCalled());
    fireEvent.click(screen.getByTestId("shelf-view-mine"));
    await waitFor(() => expect(currentQuery()).not.toContain("author"));
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