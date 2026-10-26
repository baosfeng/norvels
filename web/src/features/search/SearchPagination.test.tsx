import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SearchResult } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const useSearch = vi.fn();

vi.mock("../../api/hooks", () => ({
  useSearch: (q: string, type: string, page: number, size: number) => useSearch(q, type, page, size),
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

function result(total: number, page = 1): SearchResult {
  return {
    q: "旧城",
    books: { items: page === 1 ? [book] : [book], total, page, size: 8 },
    chapters: { items: [], total: 0, page, size: 8 },
  };
}

function renderAt(path: string) {
  return render(
    <TierProvider value="pc">
      <MemoryRouter initialEntries={[path]}>
        <SearchPage />
      </MemoryRouter>
    </TierProvider>,
  );
}

describe("搜索分页（新增控件 C-54 / C-55）", () => {
  beforeEach(() => {
    useSearch.mockReset();
    useSearch.mockReturnValue({ data: result(20), loading: false, error: null, reload: () => {} });
  });

  it("命中超过一页时出现分页；第 1 页「上一页」禁用", async () => {
    renderAt("/search?q=旧城");
    await waitFor(() => expect(screen.getByText("长夜书简")).toBeTruthy());
    const prev = screen.getByRole("button", { name: "上一页" }) as HTMLButtonElement;
    const next = screen.getByRole("button", { name: "下一页" }) as HTMLButtonElement;
    expect(prev.disabled).toBe(true);
    expect(next.disabled).toBe(false);
    expect(useSearch).toHaveBeenCalledWith("旧城", "all", 1, 8);
  });

  it("点「下一页」按第 2 页重查，且「下一页」在末页禁用", async () => {
    renderAt("/search?q=旧城");
    await waitFor(() => expect(screen.getByRole("button", { name: "下一页" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    // 第 3 页是末页（20 条 / 每页 8 条）
    await waitFor(() => expect(useSearch).toHaveBeenLastCalledWith("旧城", "all", 2, 8));
    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    await waitFor(() => expect(useSearch).toHaveBeenLastCalledWith("旧城", "all", 3, 8));
    await waitFor(() => expect((screen.getByRole("button", { name: "下一页" }) as HTMLButtonElement).disabled).toBe(true));
  });

  it("一页装得下时整个分页条不出现", async () => {
    useSearch.mockReturnValue({ data: result(5), loading: false, error: null, reload: () => {} });
    renderAt("/search?q=旧城");
    await waitFor(() => expect(screen.getByText("长夜书简")).toBeTruthy());
    expect(screen.queryByTestId("search-pagination")).toBeNull();
  });

  it("输入框按 Esc 清空、焦点留在输入框", async () => {
    renderAt("/search?q=旧城");
    const input = screen.getByLabelText("搜索关键词") as HTMLInputElement;
    input.focus();
    fireEvent.change(input, { target: { value: "旧城雨" } });
    fireEvent.keyDown(input, { key: "Escape" });
    expect(input.value).toBe("");
    expect(document.activeElement).toBe(input);
  });

  it("分栏支持左右方向键切换（C-50）", async () => {
    renderAt("/search?q=旧城");
    await waitFor(() => expect(screen.getByRole("tablist")).toBeTruthy());
    const all = screen.getByRole("tab", { name: "全部" });
    fireEvent.keyDown(all, { key: "ArrowRight" });
    await waitFor(() =>
      expect(screen.getByRole("tab", { name: /书名和作者/ }).getAttribute("aria-selected")).toBe("true"),
    );
  });
});
