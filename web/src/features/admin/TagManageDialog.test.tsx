import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TagSummary } from "../../api/types";

const { createTag, renameTag, deleteTag } = vi.hoisted(() => ({
  createTag: vi.fn(),
  renameTag: vi.fn(),
  deleteTag: vi.fn(),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { createTag, renameTag, deleteTag } };
});

const { ApiError } = await import("../../api/client");
const { TagManageDialog } = await import("./TagManageDialog");

const tags: TagSummary[] = [
  { id: 1, name: "武侠", bookCount: 2 },
  { id: 2, name: "群像", bookCount: 0 },
];

function renderDialog(overrides: Partial<Parameters<typeof TagManageDialog>[0]> = {}) {
  const props = {
    tags,
    loading: false,
    error: null as InstanceType<typeof ApiError> | null,
    onReload: vi.fn(),
    onClose: vi.fn(),
    onChanged: vi.fn(),
    ...overrides,
  };
  render(<TagManageDialog {...props} />);
  return props;
}

describe("标签管理弹窗", () => {
  beforeEach(() => {
    createTag.mockReset();
    renameTag.mockReset();
    deleteTag.mockReset();
  });

  it("列出标签与各自的书数；一个都没有时说清下一步", () => {
    renderDialog();
    expect(screen.getByTestId("tag-row-1").textContent).toContain("武侠");
    expect(screen.getByTestId("tag-row-1").textContent).toContain("2 本");
    expect(screen.getByTestId("tag-row-2").textContent).toContain("0 本");
  });

  it("没有标签时给空态", () => {
    renderDialog({ tags: [] });
    expect(screen.getByTestId("tag-manage-empty")).toBeTruthy();
  });

  it("新建：空名不发请求，填了名字就调 createTag 并刷新列表", async () => {
    createTag.mockResolvedValue({ id: 3, name: "奇幻", bookCount: 0 });
    const props = renderDialog();

    fireEvent.click(screen.getByTestId("tag-create"));
    expect(createTag).not.toHaveBeenCalled();
    expect(screen.getByTestId("tag-manage-error").textContent).toContain("先填一个标签名");

    fireEvent.change(screen.getByTestId("tag-new"), { target: { value: " 奇幻 " } });
    fireEvent.click(screen.getByTestId("tag-create"));

    await waitFor(() => expect(createTag).toHaveBeenCalledWith("奇幻"));
    await waitFor(() => expect(screen.getByTestId("tag-manage-note").textContent).toContain("奇幻"));
    expect(props.onReload).toHaveBeenCalled();
    expect(props.onChanged).toHaveBeenCalledWith(expect.stringContaining("已创建"));
    // 输入框清空，方便接着加下一个
    expect((screen.getByTestId("tag-new") as HTMLInputElement).value).toBe("");
  });

  it("改名：原地编辑后调 renameTag；重名 409 翻成人话", async () => {
    renameTag.mockResolvedValueOnce({ id: 1, name: "武侠小说", bookCount: 2 });
    const props = renderDialog();

    fireEvent.click(screen.getByTestId("tag-rename-1"));
    const input = screen.getByTestId("tag-rename-input") as HTMLInputElement;
    expect(input.value).toBe("武侠");
    fireEvent.change(input, { target: { value: "武侠小说" } });
    fireEvent.click(screen.getByTestId("tag-rename-save"));

    await waitFor(() => expect(renameTag).toHaveBeenCalledWith(1, "武侠小说"));
    await waitFor(() => expect(screen.getByTestId("tag-manage-note").textContent).toContain("武侠小说"));
    expect(props.onReload).toHaveBeenCalled();

    renameTag.mockRejectedValueOnce(new ApiError("http", "conflict", 409, "conflict"));
    fireEvent.click(screen.getByTestId("tag-rename-2"));
    fireEvent.change(screen.getByTestId("tag-rename-input"), { target: { value: "群像集" } });
    fireEvent.click(screen.getByTestId("tag-rename-save"));
    await waitFor(() => expect(screen.getByTestId("tag-manage-error").textContent).toContain("已经有同名标签"));
  });

  it("删除先确认：文案写明从 N 本书解绑、书不会被删，确认后才调接口", async () => {
    deleteTag.mockResolvedValue(undefined);
    const props = renderDialog();

    fireEvent.click(screen.getByTestId("tag-delete-1"));
    const warning = screen.getByTestId("tag-delete-warning").textContent ?? "";
    expect(warning).toContain("2 本书");
    expect(warning).toContain("不会被删除");
    expect(deleteTag).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("tag-delete-cancel"));
    expect(deleteTag).not.toHaveBeenCalled();
    expect(screen.queryByTestId("tag-delete-warning")).toBeNull();

    fireEvent.click(screen.getByTestId("tag-delete-1"));
    fireEvent.click(screen.getByTestId("tag-delete-confirm"));
    await waitFor(() => expect(deleteTag).toHaveBeenCalledWith(1));
    await waitFor(() => expect(screen.getByTestId("tag-manage-note").textContent).toContain("解绑"));
    expect(props.onChanged).toHaveBeenCalledWith(expect.stringContaining("书本身都还在"));
    expect(props.onReload).toHaveBeenCalled();
  });

  it("删除失败把「标签已不存在」说清楚，不静默", async () => {
    deleteTag.mockRejectedValueOnce(new ApiError("http", "not found", 404, "not_found"));
    const props = renderDialog();

    fireEvent.click(screen.getByTestId("tag-delete-1"));
    fireEvent.click(screen.getByTestId("tag-delete-confirm"));

    await waitFor(() => expect(screen.getByTestId("tag-manage-error").textContent).toContain("已经不在了"));
    expect(props.onChanged).not.toHaveBeenCalled();
  });

  it("标签列表的加载态与错误态各有可读状态", () => {
    renderDialog({ loading: true });
    expect(screen.getByTestId("tag-manage-loading")).toBeTruthy();
    renderDialog({ loading: false, error: new ApiError("http", "boom", 500) });
    expect(screen.getByTestId("tag-manage-load-error")).toBeTruthy();
  });
});
