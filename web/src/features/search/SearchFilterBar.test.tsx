import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CategoryTree, SearchResult } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const useSearch = vi.fn();
const useCategories = vi.fn();
const useTags = vi.fn();
const useAuthors = vi.fn();

vi.mock("../../api/hooks", () => ({
  // 搜索页空态接入了场景推荐位（SceneRecommendBlock），mock 必须把它用到的 hook 补齐
  useRecommendations: () => ({ data: null, loading: false, error: null, reload: () => {} }),
  useSearch: (q: string, params: unknown) => useSearch(q, params),
  useCategories: (opts?: unknown) => useCategories(opts),
  useTags: (opts?: unknown) => useTags(opts),
  useAuthors: (params: unknown, opts?: unknown) => useAuthors(params, opts),
}));

const { SearchPage } = await import("./SearchPage");

const book = {
  id: 1,
  title: "长夜书简",
  authorId: 9,
  authorName: "青川",
  chapterCount: 120,
  wordCount: 380000,
  volumeCount: 3,
  intro: "",
  status: "parsed" as const,
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-24T20:24:50.859Z",
};

const hit: SearchResult = { q: "旧城", books: { items: [book], total: 1, page: 1, size: 8 } };
const miss: SearchResult = { q: "旧城", books: { items: [], total: 0, page: 1, size: 8 } };

const categoryTree: CategoryTree = {
  items: [
    { l1: "玄幻", count: 12, items: [{ l2: "东方玄幻", count: 7 }, { l2: "", count: 5 }] },
    { l1: "都市", count: 4, items: [] },
  ],
  total: 16,
};

/** 地址栏探针：jsdom 的 window.location 不会跟着 MemoryRouter 变，读路由自己的 search。 */
function LocationProbe() {
  const location = useLocation();
  return <span data-testid="probe-search">{location.search}</span>;
}

function renderAt(path: string, tier: "pc" | "h5" = "pc") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={[path]}>
        <SearchPage />
        <LocationProbe />
      </MemoryRouter>
    </TierProvider>,
  );
}

/** 页面最后一次请求参数（筛选默认值不下发，所以断言用 toMatchObject / undefined）。 */
function lastParams(): Record<string, unknown> {
  const calls = useSearch.mock.calls;
  return calls[calls.length - 1][1] as Record<string, unknown>;
}

beforeEach(() => {
  useSearch.mockReset().mockReturnValue({ data: hit, loading: false, error: null, reload: () => {} });
  useCategories.mockReset().mockReturnValue({ data: categoryTree, loading: false, error: null, reload: () => {} });
  useTags
    .mockReset()
    .mockReturnValue({ data: { items: [{ id: 1, name: "武侠", bookCount: 5 }] }, loading: false, error: null, reload: () => {} });
  useAuthors.mockReset().mockReturnValue({
    data: { items: [{ id: 9, name: "青川", bookCount: 3, wordCount: 1000 }], total: 1 },
    loading: false,
    error: null,
    reload: () => {},
  });
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

/**
 * 口径：筛选区是「平铺 chips + 点一下即搜」——不是下拉、不需要再点搜索按钮。
 * jsdom 量不到宽度，横向滚动与 44px 触控区由 styles 源文件断言（searchFilters.ix.test.ts）。
 */
describe("搜索筛选：平铺展开、点一下即搜", () => {
  it("初始（还没选分类）只渲染分类一组，并提示先选分类", async () => {
    renderAt("/search?q=旧城");
    await screen.findByTestId("search-filters");
    expect(screen.getByTestId("search-filter-category")).toBeTruthy();
    for (const id of ["rating", "status", "serial", "author", "tag"]) {
      expect(screen.queryByTestId(`search-filter-${id}`)).toBeNull();
    }
    expect(screen.getByTestId("search-filters-hint").textContent).toContain("先选一个分类");
    // 一个 <select> 都没有：全是 chip
    expect(screen.queryAllByRole("combobox")).toHaveLength(0);
    expect(screen.getByTestId("search-filter-category-玄幻").textContent).toContain("12");
  });

  it("选了分类后其余五组才出现，顺序为 分类 → 细分 → 评分 → 阅读 → 连载 → 作者 → 标签", async () => {
    renderAt("/search?q=旧城&categoryL1=玄幻");
    await screen.findByTestId("search-filters");
    for (const id of ["category", "subcategory", "rating", "status", "serial", "author", "tag"]) {
      expect(screen.getByTestId(`search-filter-${id}`)).toBeTruthy();
    }
    const labels = Array.from(document.querySelectorAll(".filter-row-label")).map((el) => el.textContent);
    expect(labels).toEqual(["分类", "细分", "评分", "阅读", "连载", "作者", "标签"]);
    // 展开后不再显示「先选分类」提示
    expect(screen.queryByTestId("search-filters-hint")).toBeNull();
    expect(screen.getByTestId("search-filter-author-9").textContent).toContain("青川");
    expect(screen.getByTestId("search-filter-tag-武侠").textContent).toContain("武侠");
  });

  it("每个维度只有一颗「全部」，不重复（阅读状态不再借候选里的 null）", async () => {
    renderAt("/search?q=旧城&categoryL1=玄幻");
    await screen.findByTestId("search-filters");
    for (const id of ["category", "rating", "status", "serial", "author", "tag"]) {
      const row = screen.getByTestId(`search-filter-${id}`);
      const allChips = Array.from(row.querySelectorAll("button")).filter((b) => b.textContent === "全部");
      expect(allChips).toHaveLength(1);
    }
  });

  it("点大分类立刻重查、写进地址、页码回第 1 页（不等「搜索」按钮）", async () => {
    renderAt("/search?q=旧城&page=2");
    await screen.findByTestId("search-filters");
    fireEvent.click(screen.getByTestId("search-filter-category-玄幻"));

    await waitFor(() => expect(lastParams()).toMatchObject({ categoryL1: "玄幻", page: 1 }));
    expect(screen.getByTestId("search-filter-category-玄幻").getAttribute("aria-pressed")).toBe("true");
  });

  it("选了大分类就展开该分类的细分；空 l2（未细分）不算一项", async () => {
    renderAt("/search?q=旧城");
    await screen.findByTestId("search-filters");
    expect(screen.queryByTestId("search-filter-subcategory")).toBeNull();

    fireEvent.click(screen.getByTestId("search-filter-category-玄幻"));
    const sub = await screen.findByTestId("search-filter-subcategory");
    // 全部 + 东方玄幻（空 l2 的「未细分」不渲染成一个可选小分类）
    expect(sub.querySelectorAll("button")).toHaveLength(2);

    fireEvent.click(screen.getByTestId("search-filter-subcategory-东方玄幻"));
    await waitFor(() => expect(lastParams()).toMatchObject({ categoryL1: "玄幻", categoryL2: "东方玄幻" }));
  });

  it("评分再点一次＝取消该项（回到全部）", async () => {
    renderAt("/search?q=旧城&categoryL1=玄幻");
    await screen.findByTestId("search-filters");

    fireEvent.click(screen.getByTestId("search-filter-rating-5"));
    await waitFor(() => expect(lastParams()).toMatchObject({ rating: 5 }));
    expect(screen.getByTestId("search-filter-rating-5").getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(screen.getByTestId("search-filter-rating-5"));
    await waitFor(() => expect(lastParams().rating).toBeUndefined());
    expect(screen.getByTestId("search-filter-rating-all").getAttribute("aria-pressed")).toBe("true");
  });

  it("结果计数旁显示「已选 N 项」，清除筛选后请求回到无筛选", async () => {
    renderAt("/search?q=旧城&categoryL1=玄幻&rating=5");
    expect((await screen.findByTestId("search-active-filters")).textContent).toBe("已选 2 项");

    fireEvent.click(screen.getByTestId("search-filters-clear"));
    await waitFor(() => expect(lastParams()).toEqual({ type: "book", page: 1, size: 8, limit: 8, offset: 0 }));
    expect(screen.queryByTestId("search-active-filters")).toBeNull();
  });

  it("无结果 + 有筛选：说清是筛选排除了，并给「清除筛选」", async () => {
    useSearch.mockReturnValue({ data: miss, loading: false, error: null, reload: () => {} });
    renderAt("/search?q=旧城&categoryL1=玄幻");

    const empty = await screen.findByTestId("search-empty");
    expect(empty.textContent).toContain("没有符合条件的书");
    fireEvent.click(screen.getByTestId("search-empty-clear-filters"));
    await waitFor(() => expect(lastParams().categoryL1).toBeUndefined());
  });

  it("没有关键词：不显示筛选区，也不去取候选值", async () => {
    renderAt("/search");
    expect(screen.queryByTestId("search-filters")).toBeNull();
    expect(useCategories).toHaveBeenLastCalledWith({ enabled: false });
    expect(useTags).toHaveBeenLastCalledWith({ enabled: false });
  });

  it("点分类「全部」= 收起并清空其余筛选（URL 只剩 q，不会留下隐形筛选）", async () => {
    renderAt("/search?q=旧城&categoryL1=玄幻&rating=5");
    await screen.findByTestId("search-filters");
    expect(screen.getByTestId("search-filter-rating-5").getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(screen.getByTestId("search-filter-category-all"));
    await waitFor(() => expect(lastParams()).toEqual({ type: "book", page: 1, size: 8, limit: 8, offset: 0 }));
    // 其余组收起
    expect(screen.queryByTestId("search-filter-rating")).toBeNull();
    expect(screen.getByTestId("search-filters-hint")).toBeTruthy();
    // 地址里也不再有 rating
    await waitFor(() => expect(screen.getByTestId("probe-search").textContent).toBe("?q=%E6%97%A7%E5%9F%8E"));
  });

  it("URL 带 categoryL1&rating：刷新后展开态与选中态都还原", async () => {
    renderAt("/search?q=旧城&categoryL1=玄幻&rating=5");
    await screen.findByTestId("search-filters");
    expect(screen.getByTestId("search-filter-category-玄幻").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByTestId("search-filter-rating-5").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByTestId("search-filter-rating")).toBeTruthy();
    expect(lastParams()).toMatchObject({ categoryL1: "玄幻", rating: 5 });
  });

  it("地址里只有 rating 没有分类：按未筛选处理（界面上看不见的筛选不生效）", async () => {
    renderAt("/search?q=旧城&rating=5");
    await screen.findByTestId("search-filters");
    // 组没展开，也就不该有「已选」与 rating 参数
    expect(screen.queryByTestId("search-filter-rating")).toBeNull();
    expect(screen.queryByTestId("search-active-filters")).toBeNull();
    expect(lastParams().rating).toBeUndefined();
  });

  it("H5 同样渐进展开：初始只有分类，点了分类后其余五组出现", async () => {
    renderAt("/search?q=旧城", "h5");
    await screen.findByTestId("search-filters");
    expect(screen.queryByTestId("search-filter-rating")).toBeNull();

    fireEvent.click(screen.getByTestId("search-filter-category-玄幻"));
    await waitFor(() => expect(screen.getByTestId("search-filter-rating-5")).toBeTruthy());
    expect(screen.getByTestId("search-filter-status-reading")).toBeTruthy();
    // 没有「展开筛选」这类二次入口：展开由选分类驱动
    expect(screen.queryByRole("button", { name: /展开筛选|更多筛选/ })).toBeNull();
  });
});