import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookQuery, RequestOptions } from "../../api/client";
import type { BookSummary, CategoryTree, Page, TagSummary } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const books = vi.fn<(params: BookQuery, opts?: RequestOptions) => Promise<Page<BookSummary>>>();
const categories = vi.fn(async (): Promise<CategoryTree> => ({
  total: 2,
  items: [{ l1: "玄幻", count: 2, items: [{ l2: "东方玄幻", count: 2 }] }],
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      books: (params: BookQuery, opts?: RequestOptions) => books(params, opts),
      categories: () => categories(),
      authors: vi.fn(async () => ({
        items: [{ id: 7, name: "示例作者", bookCount: 2 }],
        total: 1,
        page: 1,
        size: 20,
      })),
      tags: vi.fn(async (): Promise<{ items: TagSummary[] }> => ({ items: [{ id: 1, name: "示例标签", bookCount: 1 }] })),
      collections: vi.fn(async () => ({ items: [{ id: 3, name: "示例书单", bookCount: 2 }], total: 1 })),
    },
  };
});

const { ShelfPage } = await import("./ShelfPage");

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

function renderShelf(tier: "h5" | "pc" = "h5", entries: string[] = ["/"]) {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={entries}>
        <ShelfPage />
      </MemoryRouter>
    </TierProvider>,
  );
}

beforeEach(() => {
  window.sessionStorage.clear();
  books.mockReset().mockResolvedValue(pageOf());
  categories.mockClear();
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

describe("H5 书架：筛选条直接平铺在页面上", () => {
  it("首屏就有一整条筛选分组，页头不再放「排序 / 筛选」按钮", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    expect(screen.getByTestId("shelf-filter-panel")).toBeTruthy();
    // 页头只剩书库管理动作：排序与筛选的入口就是面板本身，不再有开弹层的按钮
    expect(screen.queryByTestId("shelf-sort-trigger")).toBeNull();
    expect(screen.queryByTestId("shelf-filter-trigger")).toBeNull();
    expect(screen.queryByTestId("shelf-sort-sheet")).toBeNull();
  });

  it("排序组默认展开，选项直接可点（不需要先点开任何东西）", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    expect(screen.getByTestId("filter-group-toggle-sort").getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByTestId("filter-group-sort").getAttribute("aria-label")).toBe("排序筛选");
    expect(screen.getByTestId("sort-option-updated").getAttribute("aria-pressed")).toBe("true");
  });

  it("五类筛选分组都在页面上（作者只在已选时给清除入口）", async () => {
    renderShelf("h5", ["/?author=7"]);
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    for (const label of ["阅读状态筛选", "评分筛选", "分类筛选", "书单筛选", "标签筛选"]) {
      expect(screen.getByRole("group", { name: label })).toBeTruthy();
    }
    expect(screen.queryByRole("group", { name: "作者筛选" })).toBeNull();
    expect(screen.getByTestId("author-selected-clear")).toBeTruthy();
  });

  it("分组默认收起（排序除外），点标题才铺开选项", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    expect(screen.getByTestId("filter-group-toggle-status").getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(screen.getByTestId("filter-group-toggle-status"));
    expect(screen.getByTestId("filter-group-toggle-status").getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByTestId("read-status-option-all")).toBeTruthy();
  });

  it("带两个筛选参数的地址：面板标题报 2 项，排序组标题写当前排序", async () => {
    renderShelf("h5", ["/?status=done&rating=4&sort=title"]);
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    expect(screen.getByTestId("shelf-filter-panel").textContent).toContain("已选 2 项");
    expect(screen.getByTestId("filter-group-toggle-sort").textContent).toContain("书名");
  });
});

describe("PC 书架：仍是工具栏 + 下拉（H5 的平铺改动不影响桌面）", () => {
  it("宽视口渲染工具栏，没有平铺筛选条、没有弹层入口", async () => {
    renderShelf("pc");
    await waitFor(() => expect(screen.getByTestId("shelf-toolbar")).toBeTruthy());

    expect(screen.queryByTestId("shelf-filter-panel")).toBeNull();
    expect(screen.queryByTestId("shelf-sort-trigger")).toBeNull();
    expect(screen.queryByTestId("shelf-filter-trigger")).toBeNull();
    expect(screen.queryByTestId("shelf-sort-sheet")).toBeNull();
  });
});


describe("PC 书架：工具栏不再承载书单管理", () => {
  it("工具栏与操作区都不出现「管理书单」（书单管理已搬到 /collections 页）", async () => {
    renderShelf("pc", ["/?status=done"]);
    await waitFor(() => expect(screen.getByTestId("shelf-toolbar")).toBeTruthy());

    expect(screen.queryByTestId("collection-manage-open")).toBeNull();
    expect(screen.queryByTestId("collection-manager")).toBeNull();
    expect(within(screen.getByTestId("shelf-toolbar")).queryByText("管理书单")).toBeNull();

    // 操作区只剩作用于当前列表的「清除筛选」，不混进书单本身的维护入口
    const actions = screen.getByTestId("shelf-toolbar-actions");
    expect(within(actions).getByText("清除筛选")).toBeTruthy();
    expect(within(actions).queryAllByRole("button")).toHaveLength(1);
  });

  it("6 个筛选组都是「小标题 + 一个下拉」，书单筛选下拉保留", async () => {
    renderShelf("pc");
    await waitFor(() => expect(screen.getByTestId("shelf-toolbar")).toBeTruthy());

    const labels = [...screen.getByTestId("shelf-toolbar").querySelectorAll("label")];
    expect(labels).toHaveLength(6);
    for (const label of labels) {
      expect(label.querySelectorAll("select")).toHaveLength(1);
      expect(label.querySelectorAll("button")).toHaveLength(0);
    }
    // 书单仍是六类筛选之一：搬走的是「管理书单」，不是「书单」筛选
    expect(screen.getByTestId("filter-collection-select")).toBeTruthy();
  });

  it("没有筛选时不出现「清除筛选」，有筛选时才出现", async () => {
    const first = renderShelf("pc");
    await waitFor(() => expect(screen.getByTestId("shelf-toolbar-actions")).toBeTruthy());
    expect(screen.getByTestId("shelf-toolbar-actions").textContent).not.toContain("清除筛选");
    first.unmount();

    renderShelf("pc", ["/?status=done"]);
    await waitFor(() => expect(screen.getByTestId("shelf-toolbar-actions").textContent).toContain("清除筛选"));
  });
});
