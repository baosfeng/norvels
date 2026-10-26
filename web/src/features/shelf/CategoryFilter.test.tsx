import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CategoryTree } from "../../api/types";
import { CategoryFilter } from "./CategoryFilter";

const categories = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      categories: (opts?: unknown) => categories(opts),
    },
  };
});

function tree(): CategoryTree {
  return {
    total: 5,
    items: [
      { l1: "历史", count: 1, items: [{ l2: "两宋元明", count: 1 }] },
      {
        l1: "玄幻",
        count: 4,
        items: [
          { l2: "东方玄幻", count: 3 },
          { l2: "奇幻玄幻", count: 1 },
        ],
      },
      { l1: "", count: 2, items: [{ l2: "", count: 2 }] },
    ],
  };
}

function renderFilter(value = { l1: null, l2: null } as { l1: string | null; l2: string | null }, onChange = vi.fn()) {
  render(<CategoryFilter value={value} onChange={onChange} />);
  return onChange;
}

beforeEach(() => {
  categories.mockReset();
  categories.mockResolvedValue(tree());
});

describe("书架分类筛选", () => {
  it("大分类下拉跳过「未分类」，选中后把小分类清空", async () => {
    const onChange = renderFilter({ l1: "玄幻", l2: "东方玄幻" });
    const l1 = screen.getByTestId("filter-category-l1") as HTMLSelectElement;
    await waitFor(() => expect(l1.options).toHaveLength(3)); // 全部分类 + 历史 + 玄幻

    fireEvent.change(l1, { target: { value: "历史" } });
    expect(onChange).toHaveBeenCalledWith({ l1: "历史", l2: null });

    fireEvent.change(l1, { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith({ l1: null, l2: null });
  });

  it("选中有多个小分类的大分类时出现第二级下拉", async () => {
    const onChange = renderFilter({ l1: "玄幻", l2: null });
    const l2 = (await waitFor(() => screen.getByTestId("filter-category-l2"))) as HTMLSelectElement;
    expect(l2.options).toHaveLength(3); // 全部小分类 + 东方玄幻 + 奇幻玄幻

    fireEvent.change(l2, { target: { value: "奇幻玄幻" } });
    expect(onChange).toHaveBeenCalledWith({ l1: "玄幻", l2: "奇幻玄幻" });
  });

  it("只有一个小分类时仍渲染第二级：选它能排除该大分类下未细分的书", async () => {
    renderFilter({ l1: "历史", l2: null });
    const l2 = (await waitFor(() => screen.getByTestId("filter-category-l2"))) as HTMLSelectElement;
    expect(l2.options).toHaveLength(2); // 全部小分类 + 两宋元明
  });

  it("分类接口失败时给出提示而不是静默", async () => {
    categories.mockRejectedValue(new Error("boom"));
    renderFilter();
    await waitFor(() => expect(screen.getByTestId("category-hint")).toBeTruthy());
  });
});
