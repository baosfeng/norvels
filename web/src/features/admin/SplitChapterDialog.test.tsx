import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";

// vi.mock 的工厂会被提升到文件顶部，里面的引用必须先于它存在 → 用 vi.hoisted 建桩
const { chapterState, splitChapter } = vi.hoisted(() => ({
  chapterState: {
    data: {
      bookId: 7,
      bookTitle: "示例书简",
      id: 11,
      idx: 0,
      title: "长章",
      wordCount: 16,
      prevId: 0,
      nextId: 0,
      content: "第一段落。\n第二段落。\n第三段落。\n第四段落。",
    },
    loading: false,
    error: null as unknown,
    reload: () => undefined,
  },
  splitChapter: vi.fn(),
}));

vi.mock("../../api/hooks", () => ({ useChapter: () => chapterState }));
vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { splitChapter } };
});

const { SplitChapterDialog } = await import("./SplitChapterDialog");

const onSplit = vi.fn();
const onClose = vi.fn();

function renderDialog() {
  return render(
    <SplitChapterDialog
      bookId={7}
      chapter={{ id: 11, idx: 0, title: "长章", wordCount: 16 }}
      onClose={onClose}
      onSplit={onSplit}
    />,
  );
}

describe("拆分章节面板", () => {
  beforeEach(() => {
    splitChapter.mockReset();
    onSplit.mockReset();
    onClose.mockReset();
  });

  it("按段落列正文，段落之间给拆分点", () => {
    renderDialog();
    expect(screen.getByTestId("split-sub").textContent).toContain("4 段");
    expect(screen.getByTestId("split-cut-1")).toBeTruthy();
    expect(screen.getByTestId("split-cut-2")).toBeTruthy();
    expect(screen.queryByTestId("split-cut-4")).toBeNull();
  });

  it("没有拆分点时确认不可点，预览只有一片", () => {
    renderDialog();
    const confirm = screen.getByTestId("split-confirm") as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
    expect(screen.getByTestId("split-summary").textContent).toContain("目录不变");
    expect(screen.queryAllByText(/长章（/)).toHaveLength(0);
  });

  it("点一处拆分点后预览两章，字数各自核算", () => {
    renderDialog();
    fireEvent.click(screen.getByTestId("split-cut-1"));
    expect(screen.getByTestId("split-confirm").textContent).toContain("拆分为 2 章");
    expect(screen.getByTestId("split-summary").textContent).toContain("16 字");
    expect(screen.getByText("长章（1/2）")).toBeTruthy();
    expect(screen.getByText("长章（2/2）")).toBeTruthy();
  });

  it("再点一下取消该拆分点", () => {
    renderDialog();
    fireEvent.click(screen.getByTestId("split-cut-1"));
    fireEvent.click(screen.getByTestId("split-cut-1"));
    expect((screen.getByTestId("split-confirm") as HTMLButtonElement).disabled).toBe(true);
  });

  it("目标字数太小会被拦下，不布点", () => {
    renderDialog();
    fireEvent.change(screen.getByTestId("split-target"), { target: { value: "5" } });
    fireEvent.click(screen.getByTestId("split-auto"));
    expect(screen.getByTestId("split-submit-error").textContent).toContain("至少 500 字");
    expect((screen.getByTestId("split-confirm") as HTMLButtonElement).disabled).toBe(true);
  });

  it("目标字数比整章还大时不布点（等于没拆）", () => {
    renderDialog();
    fireEvent.change(screen.getByTestId("split-target"), { target: { value: "5000" } });
    fireEvent.click(screen.getByTestId("split-auto"));
    expect((screen.getByTestId("split-confirm") as HTMLButtonElement).disabled).toBe(true);
  });

  it("确认后按段落下标减一调接口，并把章数回传给页面", async () => {
    splitChapter.mockResolvedValue({ chapters: 2 });
    renderDialog();
    fireEvent.click(screen.getByTestId("split-cut-1"));
    fireEvent.click(screen.getByTestId("split-confirm"));
    await waitFor(() => expect(splitChapter).toHaveBeenCalledWith(7, 11, [0]));
    await waitFor(() => expect(onSplit).toHaveBeenCalledWith(2));
  });

  it("接口 400 时展示后端给的原因，不关闭面板", async () => {
    splitChapter.mockRejectedValue(new ApiError("http", "拆分点落到空行上了", 400, "invalid_param"));
    renderDialog();
    fireEvent.click(screen.getByTestId("split-cut-1"));
    fireEvent.click(screen.getByTestId("split-confirm"));
    await waitFor(() => expect(screen.getByTestId("split-submit-error").textContent).toContain("拆分点"));
    expect(onSplit).not.toHaveBeenCalled();
  });
});
