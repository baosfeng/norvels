import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail } from "../../api/types";

// vi.mock 的工厂会被提升到文件顶部，桩必须先于它存在 → 用 vi.hoisted 建
const { batchDeleteChapters, mergeChapters, renumberChapters, updateChapter, createChapter } = vi.hoisted(() => ({
  batchDeleteChapters: vi.fn(),
  mergeChapters: vi.fn(),
  renumberChapters: vi.fn(),
  updateChapter: vi.fn(),
  createChapter: vi.fn(),
}));

vi.mock("../../api/hooks", () => ({
  useChapter: () => ({ data: null, loading: true, error: null, reload: () => undefined }),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { batchDeleteChapters, mergeChapters, renumberChapters, updateChapter, createChapter } };
});

const { AdminChapters } = await import("./AdminChapters");

const onChanged = vi.fn();
const onError = vi.fn();

const book: BookDetail = {
  id: 7,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: 4,
  wordCount: 6000,
  volumeCount: 2,
  intro: "一段示例简介。",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  hasCover: false,
  tags: [],
  serialStatus: "ongoing",
  volumes: [
    {
      id: 1,
      idx: 0,
      title: "卷一",
      chapters: [
        { id: 10, idx: 0, title: "巷口的灯", volumeId: 1, wordCount: 1000 },
        { id: 11, idx: 1, title: "第 2 章 夜雨", volumeId: 1, wordCount: 2000 },
        { id: 12, idx: 2, title: "第三章", volumeId: 1, wordCount: 300 },
      ],
    },
    {
      id: 2,
      idx: 1,
      title: "卷二",
      chapters: [{ id: 20, idx: 3, title: "远山", volumeId: 2, wordCount: 500 }],
    },
  ],
};

function renderChapters() {
  return render(<AdminChapters book={book} onChanged={onChanged} onError={onError} />);
}

const pick = (id: number) => screen.getByTestId(`admin-chapter-pick-${id}`) as HTMLInputElement;
const batchBar = () => screen.queryByTestId("admin-ch-batch");

describe("章节勾选与批量条", () => {
  beforeEach(() => {
    batchDeleteChapters.mockReset();
    mergeChapters.mockReset();
    renumberChapters.mockReset();
    updateChapter.mockReset();
    onChanged.mockReset();
    onError.mockReset();
  });

  it("没有勾选时不渲染批量条", () => {
    renderChapters();
    expect(batchBar()).toBeNull();
  });

  it("勾一行出现「已选 1 章」，取消选择后消失", () => {
    renderChapters();
    fireEvent.click(pick(10));
    expect(batchBar()?.textContent).toContain("已选 1 章");
    fireEvent.click(screen.getByTestId("admin-ch-clear"));
    expect(batchBar()).toBeNull();
    expect(pick(10).checked).toBe(false);
  });

  it("点行内按钮不会顺手勾中这一行", () => {
    renderChapters();
    fireEvent.click(screen.getByTestId("admin-chapter-edit-10"));
    expect(pick(10).checked).toBe(false);
    expect(batchBar()).toBeNull();
    expect(screen.getByTestId("admin-chapter-title-input")).toBeTruthy();
  });

  it("表头勾选 = 全选/反选已载入的章，再点一次全清", () => {
    renderChapters();
    fireEvent.click(screen.getByTestId("admin-chapter-pick-all"));
    expect(batchBar()?.textContent).toContain("已选 4 章");
    fireEvent.click(screen.getByTestId("admin-chapter-pick-all"));
    expect(batchBar()).toBeNull();
  });

  it("卷头勾选只勾这一卷的章", () => {
    renderChapters();
    fireEvent.click(screen.getByTestId("admin-chapter-pick-vol-0"));
    expect(batchBar()?.textContent).toContain("已选 3 章");
    expect(pick(20).checked).toBe(false);
    fireEvent.click(screen.getByTestId("admin-chapter-pick-vol-1"));
    expect(batchBar()?.textContent).toContain("已选 4 章");
  });
});

describe("合并章节", () => {
  beforeEach(() => {
    mergeChapters.mockReset();
    updateChapter.mockReset();
    onChanged.mockReset();
    onError.mockReset();
  });

  it("单章时合并按钮禁用并说明原因", () => {
    renderChapters();
    fireEvent.click(pick(10));
    expect((screen.getByTestId("admin-ch-merge") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("admin-ch-merge-reason").textContent).toContain("至少要选 2 章");
  });

  it("跨卷选择时禁用并给出「只能合并同一卷里连续的章节」", () => {
    renderChapters();
    fireEvent.click(pick(10));
    fireEvent.click(pick(20));
    expect((screen.getByTestId("admin-ch-merge") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("admin-ch-merge-reason").textContent).toContain("只能合并同一卷里连续的章节");
  });

  it("同卷相邻的章可以合并，面板写明字数与失效提示", () => {
    renderChapters();
    fireEvent.click(pick(10));
    fireEvent.click(pick(11));
    expect((screen.getByTestId("admin-ch-merge") as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByTestId("admin-ch-merge"));
    expect(screen.getByTestId("chapter-merge-list").textContent).toContain("巷口的灯");
    expect((screen.getByTestId("chapter-merge-title") as HTMLInputElement).value).toBe("巷口的灯");
    expect(screen.getByTestId("chapter-merge-sum").textContent).toContain("3,000 字");
    expect(screen.getByTestId("chapter-merge-note").textContent).toContain("阅读进度与书签会失效");
  });

  it("合并成功后面板关闭、回传新的全书章数", async () => {
    mergeChapters.mockResolvedValue({ chapters: 3 });
    renderChapters();
    fireEvent.click(pick(10));
    fireEvent.click(pick(11));
    fireEvent.click(screen.getByTestId("admin-ch-merge"));
    fireEvent.click(screen.getByTestId("chapter-merge-confirm"));
    await waitFor(() => expect(mergeChapters).toHaveBeenCalledWith(7, [10, 11]));
    await waitFor(() => expect(screen.queryByTestId("chapter-merge-dialog")).toBeNull());
    expect(onChanged).toHaveBeenCalledWith(expect.stringContaining("已把 2 章合并为 1 章"));
    expect(onChanged).toHaveBeenCalledWith(expect.stringContaining("3 章"));
    expect(updateChapter).not.toHaveBeenCalled();
    expect(batchBar()).toBeNull();
  });

  it("面板里改过标题就在合并成功后补一次 PATCH", async () => {
    mergeChapters.mockResolvedValue({ chapters: 3 });
    updateChapter.mockResolvedValue(undefined);
    renderChapters();
    fireEvent.click(pick(10));
    fireEvent.click(pick(11));
    fireEvent.click(screen.getByTestId("admin-ch-merge"));
    fireEvent.change(screen.getByTestId("chapter-merge-title"), { target: { value: "巷口的灯（合）" } });
    fireEvent.click(screen.getByTestId("chapter-merge-confirm"));
    await waitFor(() => expect(updateChapter).toHaveBeenCalledWith(7, 10, "巷口的灯（合）"));
    expect(onError).not.toHaveBeenCalled();
  });

  it("标题 PATCH 失败只提示，不回滚合并", async () => {
    mergeChapters.mockResolvedValue({ chapters: 3 });
    updateChapter.mockRejectedValue(new Error("boom"));
    renderChapters();
    fireEvent.click(pick(10));
    fireEvent.click(pick(11));
    fireEvent.click(screen.getByTestId("admin-ch-merge"));
    fireEvent.change(screen.getByTestId("chapter-merge-title"), { target: { value: "新标题" } });
    fireEvent.click(screen.getByTestId("chapter-merge-confirm"));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    expect(onError).toHaveBeenCalledWith(expect.stringContaining("新标题没能保存"));
  });

  it("409 not_mergeable 展示后端给的原因", async () => {
    const { ApiError } = await vi.importActual<typeof import("../../api/client")>("../../api/client");
    mergeChapters.mockRejectedValue(new ApiError("http", "选中的章节跨了卷，不能合并", 409, "not_mergeable"));
    renderChapters();
    fireEvent.click(pick(10));
    fireEvent.click(pick(11));
    fireEvent.click(screen.getByTestId("admin-ch-merge"));
    fireEvent.click(screen.getByTestId("chapter-merge-confirm"));
    await waitFor(() =>
      expect(screen.getByTestId("chapter-merge-error").textContent).toContain("选中的章节跨了卷，不能合并"),
    );
    expect(screen.getByTestId("chapter-merge-dialog")).toBeTruthy();
  });
});

describe("批量删除章节", () => {
  beforeEach(() => {
    batchDeleteChapters.mockReset();
    onChanged.mockReset();
    onError.mockReset();
  });

  it("确认框写明书名、章数、章号区间、字数与无法撤销，焦点落在取消", () => {
    renderChapters();
    fireEvent.click(pick(10));
    fireEvent.click(pick(11));
    fireEvent.click(screen.getByTestId("admin-ch-batch-delete"));
    const text = screen.getByTestId("chapter-batch-delete-dialog").textContent ?? "";
    expect(text).toContain("示例书简");
    expect(text).toContain("将删除 2 章（第 1–2 章，共 3,000 字）");
    expect(text).toContain("无法撤销");
    expect(document.activeElement?.getAttribute("data-testid")).toBe("chapter-batch-delete-cancel");
  });

  it("确认后一次提交所有 chapterId 并回传结果", async () => {
    batchDeleteChapters.mockResolvedValue(undefined);
    renderChapters();
    fireEvent.click(pick(10));
    fireEvent.click(pick(11));
    fireEvent.click(screen.getByTestId("admin-ch-batch-delete"));
    fireEvent.click(screen.getByTestId("chapter-batch-delete-confirm"));
    await waitFor(() => expect(batchDeleteChapters).toHaveBeenCalledWith(7, [10, 11]));
    await waitFor(() => expect(screen.queryByTestId("chapter-batch-delete-dialog")).toBeNull());
    expect(onChanged).toHaveBeenCalledWith(expect.stringContaining("已删除 2 章"));
    expect(batchBar()).toBeNull();
  });
});

describe("按序号重命名", () => {
  beforeEach(() => {
    renumberChapters.mockReset();
    onChanged.mockReset();
    onError.mockReset();
  });

  it("面板给规则说明与前 3 条预览，默认改当前卷", () => {
    renderChapters();
    fireEvent.click(screen.getByTestId("admin-chapter-rename-open"));
    expect(screen.getByTestId("chapter-rename-rule").textContent).toContain("不会出现两遍章号");
    const preview = screen.getByTestId("chapter-rename-prev").textContent ?? "";
    expect(preview).toContain("巷口的灯");
    expect(preview).toContain("第 1 章 巷口的灯");
    expect(preview).toContain("第 2 章 夜雨");
    expect(screen.getByTestId("chapter-rename-sub").textContent).toContain("本次改 3 章");
    expect(screen.getByTestId("chapter-rename-scope-note").textContent).toContain("卷一");
  });

  it("切到全书后预览与章数按全书算", () => {
    renderChapters();
    fireEvent.click(screen.getByTestId("admin-chapter-rename-open"));
    fireEvent.click(screen.getByTestId("chapter-rename-scope-book"));
    expect(screen.getByTestId("chapter-rename-sub").textContent).toContain("本次改 4 章");
    expect(screen.getByTestId("chapter-rename-scope-note").textContent).toContain("全书");
  });

  it("确认后按范围调接口并回传改了几章", async () => {
    renumberChapters.mockResolvedValue({ renamed: 3 });
    renderChapters();
    fireEvent.click(screen.getByTestId("admin-chapter-rename-open"));
    fireEvent.click(screen.getByTestId("chapter-rename-confirm"));
    await waitFor(() => expect(renumberChapters).toHaveBeenCalledWith(7, { scope: "volume", volumeId: 1 }));
    await waitFor(() => expect(screen.queryByTestId("chapter-rename-dialog")).toBeNull());
    expect(onChanged).toHaveBeenCalledWith(expect.stringContaining("已按序号重命名 3 章"));
  });
});
describe("新建章节入口", () => {
  beforeEach(() => {
    createChapter.mockReset();
    onChanged.mockReset();
    onError.mockReset();
  });

  it("工具栏入口打开面板；保存后调接口、关面板并刷新目录与统计", async () => {
    createChapter.mockResolvedValue({ id: 30, idx: 4, title: "第 5 章 新章", volumeId: 2, wordCount: 10 });
    const onReload = vi.fn();
    render(<AdminChapters book={book} onChanged={onChanged} onError={onError} onReload={onReload} />);

    fireEvent.click(screen.getByTestId("admin-chapter-new-open"));
    expect(screen.getByTestId("new-chapter-dialog")).toBeTruthy();

    fireEvent.change(screen.getByTestId("new-chapter-name"), { target: { value: "第 5 章 新章" } });
    fireEvent.change(screen.getByTestId("new-chapter-content"), { target: { value: "新写的一段正文" } });
    fireEvent.click(screen.getByTestId("new-chapter-save"));

    await waitFor(() => expect(createChapter).toHaveBeenCalledWith(7, { title: "第 5 章 新章", content: "新写的一段正文" }));
    await waitFor(() => expect(screen.queryByTestId("new-chapter-dialog")).toBeNull());
    expect(onReload).toHaveBeenCalled();
  });
});
