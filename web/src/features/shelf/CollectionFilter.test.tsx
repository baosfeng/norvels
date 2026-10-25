import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Collection } from "../../api/types";
import { CollectionFilter } from "./CollectionFilter";

const collections = vi.fn();
const createCollection = vi.fn();
const renameCollection = vi.fn();
const deleteCollection = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      collections: (opts?: unknown) => collections(opts),
      createCollection: (name: string, opts?: unknown) => createCollection(name, opts),
      renameCollection: (id: number, name: string, opts?: unknown) => renameCollection(id, name, opts),
      deleteCollection: (id: number, opts?: unknown) => deleteCollection(id, opts),
    },
  };
});

function items(): { items: Collection[] } {
  return { items: [{ id: 1, name: "想读", bookCount: 3 }, { id: 2, name: "经典", bookCount: 0 }] };
}

function renderFilter(value: number | null = null, onChange = vi.fn()) {
  render(<CollectionFilter value={value} onChange={onChange} />);
  return onChange;
}

beforeEach(() => {
  collections.mockReset();
  createCollection.mockReset();
  renameCollection.mockReset();
  deleteCollection.mockReset();
  collections.mockResolvedValue(items());
});

describe("书架书单筛选", () => {
  it("下拉列出全部书单，选中把 id 交给书架", async () => {
    const onChange = renderFilter();
    await waitFor(() => expect((screen.getByTestId("filter-collection-select") as HTMLSelectElement).options).toHaveLength(3));

    fireEvent.change(screen.getByTestId("filter-collection-select"), { target: { value: "2" } });
    expect(onChange).toHaveBeenCalledWith(2);

    fireEvent.change(screen.getByTestId("filter-collection-select"), { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith(null);
  });
});

describe("书单管理弹层", () => {
  async function open() {
    await waitFor(() => expect(screen.getByTestId("filter-collection-select")).toBeTruthy());
    fireEvent.click(screen.getByTestId("collection-manage-open"));
    await waitFor(() => expect(screen.getByTestId("collection-manager")).toBeTruthy());
  }

  it("新建：去掉首尾空白后下发名字并刷新列表", async () => {
    createCollection.mockResolvedValue({ id: 3, name: "今年想读", bookCount: 0 });
    renderFilter();
    await open();

    fireEvent.change(screen.getByTestId("collection-new-input"), { target: { value: "  今年想读  " } });
    fireEvent.click(screen.getByTestId("collection-create"));

    await waitFor(() => expect(createCollection).toHaveBeenCalledWith("今年想读", undefined));
    await waitFor(() => expect(collections).toHaveBeenCalledTimes(2));
  });

  it("重名（409）给人话提示，不静默失败", async () => {
    const { ApiError } = await vi.importActual<typeof import("../../api/client")>("../../api/client");
    createCollection.mockRejectedValue(new ApiError("http", "conflict", 409, "conflict"));
    renderFilter();
    await open();

    fireEvent.change(screen.getByTestId("collection-new-input"), { target: { value: "想读" } });
    fireEvent.click(screen.getByTestId("collection-create"));

    await waitFor(() => expect(screen.getByTestId("collection-manager-error").textContent).toContain("已经有同名书单"));
  });

  it("改名：保存后刷新列表", async () => {
    renameCollection.mockResolvedValue(undefined);
    renderFilter();
    await open();

    fireEvent.click(screen.getByTestId("collection-rename-1"));
    fireEvent.change(screen.getByTestId("collection-rename-input-1"), { target: { value: "待读" } });
    fireEvent.click(screen.getByTestId("collection-rename-save-1"));

    await waitFor(() => expect(renameCollection).toHaveBeenCalledWith(1, "待读", undefined));
    await waitFor(() => expect(collections).toHaveBeenCalledTimes(2));
  });

  it("删除：先确认再下发，删掉的正好是当前筛选时清空筛选", async () => {
    deleteCollection.mockResolvedValue(undefined);
    const onChange = renderFilter(1);
    await open();

    fireEvent.click(screen.getByTestId("collection-delete-1"));
    // 点「删除」只进入确认态，不发请求
    expect(deleteCollection).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("collection-delete-confirm-1"));

    await waitFor(() => expect(deleteCollection).toHaveBeenCalledWith(1, undefined));
    expect(onChange).toHaveBeenCalledWith(null);
  });

  it("没有任何书单时给新建指引", async () => {
    collections.mockResolvedValue({ items: [] });
    renderFilter();
    await open();
    expect(screen.getByTestId("collection-manager-empty").textContent).toContain("还没有书单");
  });
});
