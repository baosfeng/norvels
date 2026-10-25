import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterBrief, Volume } from "../../api/types";

// vi.mock 的工厂会被提升到文件顶部，桩必须先于它存在 → 用 vi.hoisted 建
const { createVolume, renameVolume, deleteVolume, reorderVolumes, moveChapters } = vi.hoisted(() => ({
  createVolume: vi.fn(),
  renameVolume: vi.fn(),
  deleteVolume: vi.fn(),
  reorderVolumes: vi.fn(),
  moveChapters: vi.fn(),
}));

vi.mock("../../api/hooks", () => ({
  useChapter: () => ({ data: null, loading: true, error: null, reload: () => undefined }),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { createVolume, renameVolume, deleteVolume, reorderVolumes, moveChapters } };
});

const { VolumeManageDialog } = await import("./VolumeManageDialog");
const { VolumeMoveDialog } = await import("./VolumeMoveDialog");
const { AdminChapters } = await import("./AdminChapters");

function chapter(id: number, idx: number, volumeId: number, wordCount = 1000): ChapterBrief {
  return { id, idx, title: `第 ${idx + 1} 章`, volumeId, wordCount };
}

const v1: Volume = { id: 1, idx: 0, title: "卷一", chapters: [chapter(10, 0, 1), chapter(11, 1, 1)] };
const v2: Volume = { id: 2, idx: 1, title: "卷二", chapters: [chapter(20, 2, 2)] };
const v3: Volume = { id: 3, idx: 2, title: "卷三", chapters: [] };
const loose: Volume = { id: 0, idx: -1, title: "未分卷", chapters: [chapter(30, 3, 0)] };
const volumes = [v1, v2, v3, loose];

const book: BookDetail = {
  id: 7,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: 4,
  wordCount: 4000,
  volumeCount: 3,
  intro: "",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  hasCover: false,
  tags: [],
  volumes,
};

const onSaved = vi.fn();
const onClose = vi.fn();

function renderManage() {
  return render(<VolumeManageDialog bookId={7} volumes={volumes} onSaved={onSaved} onClose={onClose} />);
}

function renderMove(chapters: ChapterBrief[]) {
  return render(
    <VolumeMoveDialog bookId={7} volumes={volumes} chapters={chapters} onClose={onClose} onMoved={vi.fn()} />,
  );
}

describe("卷管理弹窗", () => {
  beforeEach(() => {
    createVolume.mockReset();
    renameVolume.mockReset();
    deleteVolume.mockReset();
    reorderVolumes.mockReset();
    onSaved.mockReset();
    onClose.mockReset();
  });

  it("逐卷列出章数与区间，未分卷不在卷管理里", () => {
    renderManage();
    expect(screen.getByTestId("volume-name-1").textContent).toBe("卷一");
    expect(screen.getByTestId("volume-meta-1").textContent).toBe("2 章 · 第 1–2 章");
    expect(screen.getByTestId("volume-meta-3").textContent).toBe("0 章 · 空卷");
    expect(screen.getByTestId("volume-manage-count").textContent).toBe("3");
    expect(screen.queryByTestId("volume-row-0")).toBeNull();
  });

  it("首卷不能上移、末卷不能下移（按钮禁用不发请求）", () => {
    renderManage();
    expect((screen.getByTestId("volume-up-1") as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId("volume-down-3") as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId("volume-down-1") as HTMLButtonElement).disabled).toBe(false);
  });

  it("新建空卷：空名与重名都在前端挡下，不发请求", () => {
    renderManage();
    fireEvent.click(screen.getByTestId("volume-create"));
    expect(screen.getByTestId("volume-error").textContent).toBe("卷名不能为空。");
    fireEvent.change(screen.getByTestId("volume-new-input"), { target: { value: " 卷一 " } });
    fireEvent.click(screen.getByTestId("volume-create"));
    expect(screen.getByTestId("volume-error").textContent).toBe("这本书已经有同名卷了");
    expect(createVolume).not.toHaveBeenCalled();
  });

  it("新建成功后清空输入并回传提示", async () => {
    createVolume.mockResolvedValue({ volume: { id: 9, idx: 3, title: "卷四", chapters: [] } });
    renderManage();
    fireEvent.change(screen.getByTestId("volume-new-input"), { target: { value: " 卷四 " } });
    fireEvent.click(screen.getByTestId("volume-create"));
    await waitFor(() => expect(createVolume).toHaveBeenCalledWith(7, "卷四"));
    expect(onSaved).toHaveBeenCalledWith("已新建空卷「卷四」");
    expect((screen.getByTestId("volume-new-input") as HTMLInputElement).value).toBe("");
  });

  it("重命名内联：回车保存，重名时提示且不发请求", async () => {
    renameVolume.mockResolvedValue({ volume: { id: 2, idx: 1, title: "卷二 · 上", chapters: [] } });
    renderManage();
    fireEvent.click(screen.getByTestId("volume-rename-open-2"));
    const input = screen.getByTestId("volume-rename-input") as HTMLInputElement;
    expect(input.value).toBe("卷二");
    fireEvent.change(input, { target: { value: "卷一" } });
    fireEvent.click(screen.getByTestId("volume-rename-save"));
    expect(screen.getByTestId("volume-error").textContent).toBe("这本书已经有同名卷了");
    expect(renameVolume).not.toHaveBeenCalled();

    fireEvent.change(screen.getByTestId("volume-rename-input"), { target: { value: "卷二 · 上" } });
    fireEvent.keyDown(screen.getByTestId("volume-rename-input"), { key: "Enter" });
    await waitFor(() => expect(renameVolume).toHaveBeenCalledWith(7, 2, "卷二 · 上"));
    expect(onSaved).toHaveBeenCalledWith("卷名已改为「卷二 · 上」");
  });

  it("后端 409 重名用同一句文案，弹窗不关闭", async () => {
    const { ApiError } = await vi.importActual<typeof import("../../api/client")>("../../api/client");
    renameVolume.mockRejectedValue(new ApiError("http", "这本书已经有同名卷了", 409, "volume_name_taken"));
    renderManage();
    fireEvent.click(screen.getByTestId("volume-rename-open-2"));
    fireEvent.change(screen.getByTestId("volume-rename-input"), { target: { value: "卷二 · 上" } });
    fireEvent.click(screen.getByTestId("volume-rename-save"));
    await waitFor(() => expect(screen.getByTestId("volume-error").textContent).toBe("这本书已经有同名卷了"));
    expect(screen.getByTestId("volume-manage-dialog")).toBeTruthy();
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("上移提交的是完整卷 id 顺序（不含未分卷）", async () => {
    reorderVolumes.mockResolvedValue(undefined);
    renderManage();
    fireEvent.click(screen.getByTestId("volume-up-2"));
    await waitFor(() => expect(reorderVolumes).toHaveBeenCalledWith(7, [2, 1, 3]));
    expect(onSaved).toHaveBeenCalledWith("「卷二」已上移。");
  });

  it("下移同理，失败时不回传成功", async () => {
    reorderVolumes.mockRejectedValue(new Error("boom"));
    renderManage();
    fireEvent.click(screen.getByTestId("volume-down-1"));
    await waitFor(() => expect(reorderVolumes).toHaveBeenCalledWith(7, [2, 1, 3]));
    expect(onSaved).not.toHaveBeenCalled();
    expect(screen.getByTestId("volume-error").textContent).toBe("操作没能完成，稍后再试。");
  });

  it("删卷确认写明卷内章数，确认后调接口并回传提示", async () => {
    deleteVolume.mockResolvedValue(undefined);
    renderManage();
    fireEvent.click(screen.getByTestId("volume-delete-1"));
    expect(screen.getByTestId("volume-delete-text").textContent).toContain("卷内 2 章会移到「未分卷」。");
    expect(document.activeElement?.getAttribute("data-testid")).toBe("volume-delete-cancel");
    fireEvent.click(screen.getByTestId("volume-delete-confirm"));
    await waitFor(() => expect(deleteVolume).toHaveBeenCalledWith(7, 1));
    expect(onSaved).toHaveBeenCalledWith("已删除「卷一」，2 章回到「未分卷」。");
  });
});

describe("移动到卷弹窗", () => {
  beforeEach(() => {
    moveChapters.mockReset();
    onClose.mockReset();
  });

  it("列出所有卷 + 未分卷，默认选第一卷", () => {
    renderMove([chapter(10, 0, 1)]);
    expect(screen.getByTestId("volume-move-opt-1")).toBeTruthy();
    expect(screen.getByTestId("volume-move-opt-2")).toBeTruthy();
    expect(screen.getByTestId("volume-move-opt-0").textContent).toContain("未分卷");
    const first = screen.getByTestId("volume-move-opt-1").querySelector("input") as HTMLInputElement;
    expect(first.checked).toBe(true);
    expect(screen.getByTestId("volume-move-sub").textContent).toContain("已经在「卷一」里了");
  });

  it("选「未分卷」后提交 volumeId=0 并回传章数", async () => {
    moveChapters.mockResolvedValue({ moved: 2 });
    const onMoved = vi.fn();
    render(
      <VolumeMoveDialog
        bookId={7}
        volumes={volumes}
        chapters={[chapter(10, 0, 1), chapter(20, 2, 2)]}
        onClose={onClose}
        onMoved={onMoved}
      />,
    );
    fireEvent.click(screen.getByTestId("volume-move-opt-0").querySelector("input") as HTMLInputElement);
    fireEvent.click(screen.getByTestId("volume-move-confirm"));
    await waitFor(() => expect(moveChapters).toHaveBeenCalledWith(7, [10, 20], 0));
    expect(onMoved).toHaveBeenCalledWith({ moved: 2, volumeId: 0, volumeTitle: "未分卷" });
  });

  it("全都在目标卷时不动库，只提示", () => {
    moveChapters.mockResolvedValue({ moved: 0 });
    renderMove([chapter(10, 0, 1), chapter(11, 1, 1)]);
    fireEvent.click(screen.getByTestId("volume-move-confirm"));
    expect(moveChapters).not.toHaveBeenCalled();
    expect(screen.getByTestId("volume-move-error").textContent).toContain("已经在这个卷里了");
  });

  it("接口失败时留在弹窗并给出人话", async () => {
    const { ApiError } = await vi.importActual<typeof import("../../api/client")>("../../api/client");
    moveChapters.mockRejectedValue(new ApiError("http", "章节不属于这本书", 404, "not_found"));
    renderMove([chapter(10, 0, 1), chapter(20, 2, 2)]);
    fireEvent.click(screen.getByTestId("volume-move-confirm"));
    await waitFor(() => expect(screen.getByTestId("volume-move-error").textContent).toContain("已经不在本书里"));
    expect(screen.getByTestId("volume-move-dialog")).toBeTruthy();
  });
});

describe("章节区的卷入口", () => {
  beforeEach(() => {
    moveChapters.mockReset();
  });

  it("工具栏「卷管理…」打开卷管理弹窗，并列出真实卷数", () => {
    render(<AdminChapters book={book} onChanged={vi.fn()} onError={vi.fn()} />);
    expect(screen.getByTestId("admin-chapters-meta").textContent).toContain("3 卷 · 未分卷 1 章");
    fireEvent.click(screen.getByTestId("admin-volume-manage-open"));
    expect(screen.getByTestId("volume-manage-dialog")).toBeTruthy();
  });

  it("未分卷分组固定排在章节表最上面", () => {
    const { container } = render(<AdminChapters book={book} onChanged={vi.fn()} onError={vi.fn()} />);
    const groups = [...container.querySelectorAll("tbody")].map((node) => node.getAttribute("data-testid"));
    expect(groups[0]).toBe("admin-volume--1");
    expect(groups.slice(1)).toEqual(["admin-volume-0", "admin-volume-1", "admin-volume-2"]);
  });

  // 回归：删卷后「新出现的」未分卷分组不在挂载时初始化的 shown 表里，
  // 旧写法 (shown[idx] ?? 0) < chapters.length 会误报「载入更多 · 本卷还有 1 章」。
  it("删卷后新出现的未分卷分组只有 1 章时不显示「载入更多」", () => {
    const before: BookDetail = { ...book, volumeCount: 3, volumes: [v1, v2, v3] };
    // 删掉卷三后，卷内 1 章回到未分卷：分组 idx=-1 是渲染期间才出现的
    const after: BookDetail = {
      ...book,
      volumeCount: 2,
      volumes: [v1, v2, { id: 0, idx: -1, title: "未分卷", chapters: [chapter(30, 3, 0)] }],
    };
    const { container, rerender } = render(<AdminChapters book={before} onChanged={vi.fn()} onError={vi.fn()} />);
    expect(container.querySelector(".more-row")).toBeNull();

    rerender(<AdminChapters book={after} onChanged={vi.fn()} onError={vi.fn()} />);
    const looseGroup = screen.getByTestId("admin-volume--1");
    expect(looseGroup.textContent).toContain("未分卷");
    expect(looseGroup.textContent).toContain("1 章 · 第 4 章");
    expect(within(looseGroup).queryByRole("button", { name: "载入更多" })).toBeNull();
    expect(looseGroup.textContent).not.toContain("本卷还有");
  });

  it("勾选后批量条出现「移动到卷…」，点开弹窗写明目标卷", () => {
    render(<AdminChapters book={book} onChanged={vi.fn()} onError={vi.fn()} />);
    expect(screen.queryByTestId("admin-ch-move")).toBeNull();
    fireEvent.click(screen.getByTestId("admin-chapter-pick-20"));
    fireEvent.click(screen.getByTestId("admin-ch-move"));
    expect(screen.getByTestId("volume-move-dialog")).toBeTruthy();
    expect(screen.getByTestId("volume-move-sub").textContent).toBe("把已选 1 章移到「卷一」末尾。");
  });
});
