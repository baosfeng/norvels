import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Volume } from "../../api/types";

const { createChapter } = vi.hoisted(() => ({ createChapter: vi.fn() }));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { createChapter } };
});

const { ApiError } = await import("../../api/client");
const { NewChapterDialog } = await import("./NewChapterDialog");

const volumes: Volume[] = [
  {
    id: 1,
    idx: 0,
    title: "卷一",
    chapters: [
      { id: 10, idx: 0, title: "开头", volumeId: 1, wordCount: 1000 },
      { id: 11, idx: 1, title: "夜雨", volumeId: 1, wordCount: 2000 },
    ],
  },
  {
    id: 2,
    idx: 1,
    title: "卷二",
    chapters: [{ id: 20, idx: 2, title: "远山", volumeId: 2, wordCount: 500 }],
  },
];

function renderDialog() {
  const props = {
    bookId: 7,
    volumes,
    onClose: vi.fn(),
    onCreated: vi.fn(),
    onFailed: vi.fn(),
  };
  render(<NewChapterDialog {...props} />);
  return props;
}

function fill(title: string, content: string) {
  fireEvent.change(screen.getByTestId("new-chapter-name"), { target: { value: title } });
  fireEvent.change(screen.getByTestId("new-chapter-content"), { target: { value: content } });
}

describe("新建章节面板", () => {
  beforeEach(() => {
    createChapter.mockReset();
  });

  it("插入位置默认追加到末尾，候选章跨卷按章序列出", () => {
    renderDialog();
    const select = screen.getByTestId("new-chapter-after") as HTMLSelectElement;
    expect(select.value).toBe("");
    const labels = [...select.options].map((option) => option.textContent ?? "");
    expect(labels[0]).toContain("追加到末尾");
    expect(labels.slice(1)).toEqual([
      "插在第 1 章「开头」之后",
      "插在第 2 章「夜雨」之后",
      "插在第 3 章「远山」之后",
    ]);
  });

  it("空标题 / 空正文都在本地拦下，不发请求", () => {
    renderDialog();

    fireEvent.click(screen.getByTestId("new-chapter-save"));
    expect(createChapter).not.toHaveBeenCalled();
    expect(screen.getByTestId("new-chapter-error").textContent).toContain("标题不能为空");

    fill("第九章 归来", "   ");
    fireEvent.click(screen.getByTestId("new-chapter-save"));
    expect(createChapter).not.toHaveBeenCalled();
    expect(screen.getByTestId("new-chapter-error").textContent).toContain("正文不能为空");
  });

  it("追加到末尾时不带 afterChapterId，成功后回传新章", async () => {
    createChapter.mockResolvedValue({ id: 99, idx: 3, title: "第九章 归来", volumeId: 2, wordCount: 12 });
    const props = renderDialog();

    fill(" 第九章 归来 ", "正文第一段\n\n正文第二段");
    fireEvent.click(screen.getByTestId("new-chapter-save"));

    await waitFor(() =>
      expect(createChapter).toHaveBeenCalledWith(7, {
        title: "第九章 归来",
        content: "正文第一段\n\n正文第二段",
      }),
    );
    await waitFor(() => expect(props.onCreated).toHaveBeenCalledWith(expect.objectContaining({ id: 99 })));
  });

  it("选择「插在某章之后」时把 chapterId 一起提交", async () => {
    createChapter.mockResolvedValue({ id: 98, idx: 1, title: "插章", volumeId: 1, wordCount: 8 });
    renderDialog();

    fill("插章", "补一段");
    fireEvent.change(screen.getByTestId("new-chapter-after"), { target: { value: "10" } });
    fireEvent.click(screen.getByTestId("new-chapter-save"));

    await waitFor(() => expect(createChapter).toHaveBeenCalledWith(7, { title: "插章", content: "补一段", afterChapterId: 10 }));
  });

  it("失败把后端人话留在面板上并通知外层，面板不关", async () => {
    createChapter.mockRejectedValueOnce(new ApiError("http", "正文超过 1 MiB 上限", 400, "invalid_param"));
    const props = renderDialog();

    fill("第九章 归来", "正文");
    fireEvent.click(screen.getByTestId("new-chapter-save"));

    await waitFor(() => expect(screen.getByTestId("new-chapter-error").textContent).toContain("1 MiB"));
    expect(props.onFailed).toHaveBeenCalledWith(expect.stringContaining("1 MiB"));
    expect(props.onCreated).not.toHaveBeenCalled();
    expect(props.onClose).not.toHaveBeenCalled();
    // 正文还留在输入框里，可以直接改小再点一次
    expect((screen.getByTestId("new-chapter-content") as HTMLTextAreaElement).value).toBe("正文");
  });

  it("超时单独说，不把失败当成功", async () => {
    createChapter.mockRejectedValueOnce(new ApiError("timeout", "请求超时"));
    const props = renderDialog();

    fill("第九章 归来", "正文");
    fireEvent.click(screen.getByTestId("new-chapter-save"));

    await waitFor(() => expect(screen.getByTestId("new-chapter-error").textContent).toContain("超时"));
    expect(props.onCreated).not.toHaveBeenCalled();
  });
});
