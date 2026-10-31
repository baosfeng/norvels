import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Collection } from "../../api/types";
import { CollectionFilter } from "./CollectionFilter";

function items(): Collection[] {
  return [
    { id: 1, name: "想读", bookCount: 3 },
    { id: 2, name: "经典", bookCount: 0 },
  ];
}

/** 下拉的候选由书架页给（同一份 /api/collections），这里不自己取数 */
function renderFilter(list: Collection[] = items(), value: number | null = null, onChange = vi.fn()) {
  const view = render(<CollectionFilter value={value} onChange={onChange} items={list} />);
  return { onChange, view };
}

describe("书架书单筛选", () => {
  it("下拉列出全部书单，选中把 id 交给书架", async () => {
    const { onChange } = renderFilter();
    await waitFor(() =>
      expect((screen.getByTestId("filter-collection-select") as HTMLSelectElement).options).toHaveLength(3),
    );

    fireEvent.change(screen.getByTestId("filter-collection-select"), { target: { value: "2" } });
    expect(onChange).toHaveBeenCalledWith(2);

    fireEvent.change(screen.getByTestId("filter-collection-select"), { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith(null);
  });

  it("候选换一份就跟着换（父级刷新后不留在旧列表上）", async () => {
    const { view } = renderFilter();
    await waitFor(() => expect(screen.getByTestId("filter-collection-select").children).toHaveLength(3));

    view.rerender(<CollectionFilter value={null} onChange={vi.fn()} items={[{ id: 9, name: "新书单", bookCount: 1 }]} />);
    await waitFor(() => expect(screen.getByTestId("filter-collection-select").children).toHaveLength(2));
    expect(screen.getByTestId("filter-collection-select").textContent).toContain("新书单");
  });
});
