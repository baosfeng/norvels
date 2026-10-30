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

/** 弹层里某个分区的分组元素 */
function section(anchor: "sort" | "filters"): HTMLElement {
  return document.querySelector<HTMLElement>(`[data-sheet-section="${anchor}"]`) as HTMLElement;
}

describe("H5 书架：排序与筛选是两个独立入口", () => {
  it("两个按钮都在页头操作行里，各自带 aria-haspopup", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    const sortTrigger = screen.getByTestId("shelf-sort-trigger");
    const filterTrigger = screen.getByTestId("shelf-filter-trigger");
    expect(sortTrigger).not.toBe(filterTrigger);
    expect(sortTrigger.getAttribute("aria-haspopup")).toBe("dialog");
    expect(filterTrigger.getAttribute("aria-haspopup")).toBe("dialog");
    // 两个按钮都在 .shelf-head-actions 这一行（44px 热区由 ix-library.css 给）
    expect(sortTrigger.closest(".shelf-head-actions")).toBeTruthy();
    expect(filterTrigger.closest(".shelf-head-actions")).toBeTruthy();
  });

  it("点「排序」直接停在排序分区：焦点落在第一个排序选项", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-sort-trigger"));
    const sheet = within(screen.getByTestId("shelf-sort-sheet"));
    expect(sheet.getByRole("dialog")).toBeTruthy();

    await waitFor(() => expect(section("sort")).toBeTruthy());
    expect(section("sort").getAttribute("aria-label")).toBe("排序筛选");
    await waitFor(() => expect(document.activeElement?.getAttribute("data-testid")).toBe("sort-option-updated"));
  });

  it("点「筛选」停在第一个筛选分区，六类筛选都在弹层里", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    fireEvent.click(screen.getByTestId("shelf-filter-trigger"));
    await waitFor(() => expect(section("filters")).toBeTruthy());
    expect(section("filters").getAttribute("aria-label")).toBe("阅读状态筛选");
    await waitFor(() => expect(document.activeElement?.getAttribute("data-testid")).toBe("read-status-option-all"));

    const sheet = within(screen.getByTestId("shelf-sort-sheet"));
    // 六类：阅读状态 / 评分 / 分类 / 作者 / 书单 / 标签（排序是第七个分区，不在筛选之列）
    for (const label of ["阅读状态筛选", "评分筛选", "分类筛选", "作者筛选", "书单筛选", "标签筛选"]) {
      expect(sheet.getByRole("group", { name: label })).toBeTruthy();
    }
    expect(sheet.queryByRole("group", { name: "排序筛选" })).toBeTruthy();
  });

  it("无筛选时筛选按钮不带强调态", async () => {
    renderShelf();
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());
    expect(screen.getByTestId("shelf-filter-trigger").className).not.toContain("has-filters");
  });

  it("带两个筛选参数的地址：筛选按钮报 2 项，排序按钮报当前排序", async () => {
    renderShelf("h5", ["/?status=done&rating=4&sort=title"]);
    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());

    const filterTrigger = screen.getByTestId("shelf-filter-trigger");
    expect(filterTrigger.textContent).toBe("已选 2 项");
    expect(filterTrigger.className).toContain("has-filters");
    expect(screen.getByTestId("shelf-sort-trigger").textContent).toBe("排序 · 书名");
  });
});

describe("PC 书架：「管理书单」是工具栏右侧的独立操作", () => {
  it("管理书单不在任何筛选 label 里，与清除筛选同处操作区", async () => {
    renderShelf("pc", ["/?status=done"]);
    await waitFor(() => expect(screen.getByTestId("shelf-toolbar")).toBeTruthy());

    const manage = screen.getByTestId("collection-manage-open");
    // 以前它和书单下拉一起塞在「书单」label 里，把那一组撑成两行、整排标签错位
    expect(manage.closest("label")).toBeNull();

    const actions = screen.getByTestId("shelf-toolbar-actions");
    expect(actions.contains(manage)).toBe(true);
    expect(within(actions).getByText("清除筛选")).toBeTruthy();
  });

  it("6 个筛选组都是「小标题 + 一个下拉」，没有组里再塞按钮", async () => {
    renderShelf("pc");
    await waitFor(() => expect(screen.getByTestId("shelf-toolbar")).toBeTruthy());

    const labels = [...screen.getByTestId("shelf-toolbar").querySelectorAll("label")];
    expect(labels).toHaveLength(6);
    for (const label of labels) {
      expect(label.querySelectorAll("select")).toHaveLength(1);
      expect(label.querySelectorAll("button")).toHaveLength(0);
    }
  });

  it("点「管理书单」能打开管理弹层", async () => {
    renderShelf("pc");
    await waitFor(() => expect(screen.getByTestId("shelf-toolbar")).toBeTruthy());

    fireEvent.click(screen.getByTestId("collection-manage-open"));
    await waitFor(() => expect(screen.getByTestId("collection-manager")).toBeTruthy());
    expect(screen.getByTestId("collection-manager").textContent).toContain("管理书单");
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
