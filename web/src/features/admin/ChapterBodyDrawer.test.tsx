import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterDetail } from "../../api/types";
import type { ApiError } from "../../api/client";

// vi.mock 的工厂会被提升到文件顶部，桩必须先于它存在 → 用 vi.hoisted 建
const { updateChapterContent, reloadChapter } = vi.hoisted(() => ({
  updateChapterContent: vi.fn(),
  reloadChapter: vi.fn(),
}));

type ChapterResource = {
  data: ChapterDetail | null;
  loading: boolean;
  error: ApiError | null;
  reload: () => void;
};

const chapterState = vi.hoisted(() => ({
  current: { data: null, loading: false, error: null, reload: () => undefined } as unknown as {
    data: null;
    loading: boolean;
    error: null;
    reload: () => void;
  },
}));

vi.mock("../../api/hooks", () => ({ useChapter: () => chapterState.current }));
vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { updateChapterContent } };
});

const { ApiError: ApiErrorClass } = await import("../../api/client");
const { AdminChapters } = await import("./AdminChapters");

/** 虚构正文：18 个非空白字符、2 段（口径见 lib/chapterBody）。 */
const BODY = "雨从屋檐上垂下来。\n\n守夜人把灯芯挑亮。";

const book: BookDetail = {
  id: 7,
  title: "夜雨示例录",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: 2,
  wordCount: 3000,
  volumeCount: 1,
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
        { id: 10, idx: 0, title: "巷口的灯", volumeId: 1, wordCount: 18 },
        { id: 11, idx: 1, title: "第 2 章 远山", volumeId: 1, wordCount: 2982 },
      ],
    },
  ],
};

function detail(content: string): ChapterDetail {
  return {
    bookId: 7,
    bookTitle: book.title,
    id: 10,
    idx: 0,
    title: "巷口的灯",
    content,
    wordCount: 18,
    prevId: 0,
    nextId: 11,
  };
}

function loaded(content = BODY): ChapterResource {
  return { data: detail(content), loading: false, error: null, reload: reloadChapter };
}

const onChanged = vi.fn();
const onError = vi.fn();
const onReload = vi.fn();

function renderAndOpen() {
  render(<AdminChapters book={book} onChanged={onChanged} onError={onError} onReload={onReload} />);
  fireEvent.click(screen.getByTestId("admin-chapter-body-10"));
}

const input = () => screen.getByTestId("chapter-body-input") as HTMLTextAreaElement;

beforeEach(() => {
  updateChapterContent.mockReset();
  reloadChapter.mockReset();
  onChanged.mockReset();
  onError.mockReset();
  onReload.mockReset();
  chapterState.current = loaded() as unknown as typeof chapterState.current;
});

describe("章节正文抽屉：打开与实时统计", () => {
  it("点「正文」打开抽屉，标题、正文、字数与段数都对得上", () => {
    renderAndOpen();
    expect(screen.getByTestId("chapter-body-title").textContent).toBe("第 1 章 巷口的灯");
    expect(input().value).toBe(BODY);
    expect(screen.getByTestId("chapter-body-words").textContent).toContain("18");
    expect(screen.getByTestId("chapter-body-paras").textContent).toBe("2");
    expect(screen.getByTestId("chapter-body-state").textContent).toBe("未改动");
    expect(screen.getByTestId("chapter-body-meta").textContent).toContain("未改动");
  });

  it("两条常驻提示始终在顶部", () => {
    renderAndOpen();
    const notes = screen.getByTestId("chapter-body-notes").textContent ?? "";
    expect(notes).toContain("保存会替换本章正文，正文长度变化会影响本章字数。");
    expect(notes).toContain("本章已存的书签位置可能偏移。");
  });

  it("改内容后状态变「有未保存的改动」，字数与段数实时跟着变", () => {
    renderAndOpen();
    fireEvent.change(input(), { target: { value: `${BODY}\n\n又添了一段。` } });
    expect(screen.getByTestId("chapter-body-state").textContent).toBe("有未保存的改动");
    expect(screen.getByTestId("chapter-body-words").textContent).toContain("24");
    expect(screen.getByTestId("chapter-body-paras").textContent).toBe("3");
  });

  it("全屏按钮在「全屏 / 退出全屏」之间切换", () => {
    renderAndOpen();
    const full = screen.getByTestId("chapter-body-full");
    expect(full.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(full);
    expect(full.getAttribute("aria-pressed")).toBe("true");
    expect(full.textContent).toBe("退出全屏");
    fireEvent.click(full);
    expect(full.textContent).toBe("全屏");
  });

  it("没改过就关闭，不弹确认，直接关掉", () => {
    renderAndOpen();
    fireEvent.click(screen.getByTestId("chapter-body-close"));
    expect(screen.queryByTestId("chapter-body-leave")).toBeNull();
    expect(screen.queryByTestId("chapter-body-drawer")).toBeNull();
  });
});

describe("章节正文抽屉：保存", () => {
  it("保存调 updateChapterContent，成功后关抽屉并重拉目录，横幅留给别的操作", async () => {
    updateChapterContent.mockResolvedValue(undefined);
    renderAndOpen();
    fireEvent.change(input(), { target: { value: `${BODY}\n\n又添了一段。` } });
    fireEvent.click(screen.getByTestId("chapter-body-save"));

    await waitFor(() => expect(updateChapterContent).toHaveBeenCalledWith(7, 10, `${BODY}\n\n又添了一段。`));
    await waitFor(() => expect(screen.queryByTestId("chapter-body-drawer")).toBeNull());
    expect(onReload).toHaveBeenCalledTimes(1);
    expect(onChanged).not.toHaveBeenCalled();
    expect(onError).not.toHaveBeenCalled();
  });

  it("空正文在本地就拦下：不发请求，提示不为空", () => {
    chapterState.current = loaded("   \n\n  ") as unknown as typeof chapterState.current;
    renderAndOpen();
    fireEvent.click(screen.getByTestId("chapter-body-save"));
    expect(updateChapterContent).not.toHaveBeenCalled();
    const text = screen.getByTestId("chapter-body-save-error").textContent ?? "";
    expect(text.length).toBeGreaterThan(0);
    expect(text).toContain("正文不能为空");
    expect(screen.getByTestId("chapter-body-drawer")).toBeTruthy();
  });

  it("后端 400 的人话 message 直接展示，抽屉不关", async () => {
    const message = "章节正文 1048577 字节，超过上限 1048576 字节（1 MiB，UTF-8 编码后）";
    updateChapterContent.mockRejectedValue(new ApiErrorClass("http", message, 400, "invalid_param"));
    renderAndOpen();
    fireEvent.change(input(), { target: { value: "改过的一版" } });
    fireEvent.click(screen.getByTestId("chapter-body-save"));

    await waitFor(() => expect(screen.getByTestId("chapter-body-save-error").textContent).toBe(message));
    expect(screen.getByTestId("chapter-body-drawer")).toBeTruthy();
    expect(onReload).not.toHaveBeenCalled();
  });
});

describe("章节正文抽屉：未保存关闭的拦截", () => {
  it("有改动时关闭先弹确认，「继续编辑」留在抽屉里", () => {
    renderAndOpen();
    fireEvent.change(input(), { target: { value: `${BODY}\n\n补一句。` } });
    fireEvent.click(screen.getByTestId("chapter-body-close"));

    const leave = screen.getByTestId("chapter-body-leave");
    expect(leave.textContent).toContain("本章正文还没保存");
    expect(leave.textContent).toContain("离开后正文回到上次保存的版本，改过的内容不会留在书里。");
    expect(screen.getByTestId("chapter-body-drawer")).toBeTruthy();

    fireEvent.click(screen.getByTestId("chapter-body-leave-stay"));
    expect(screen.queryByTestId("chapter-body-leave")).toBeNull();
    expect(screen.getByTestId("chapter-body-drawer")).toBeTruthy();
    expect(input().value).toContain("补一句。");
  });

  it("「放弃修改」才真的关掉，且一次请求都不发", () => {
    renderAndOpen();
    fireEvent.change(input(), { target: { value: `${BODY}\n\n补一句。` } });
    fireEvent.click(screen.getByTestId("chapter-body-cancel"));
    expect(screen.getByTestId("chapter-body-leave")).toBeTruthy();

    fireEvent.click(screen.getByTestId("chapter-body-leave-discard"));
    expect(screen.queryByTestId("chapter-body-drawer")).toBeNull();
    expect(updateChapterContent).not.toHaveBeenCalled();
    expect(onReload).not.toHaveBeenCalled();
  });
});

describe("章节正文抽屉：取正文失败", () => {
  it("404 显示错误态与原因，重试会再拉一次", () => {
    chapterState.current = {
      data: null,
      loading: false,
      error: new ApiErrorClass("http", "章节不存在", 404, "not_found"),
      reload: reloadChapter,
    } as unknown as typeof chapterState.current;
    renderAndOpen();

    const box = screen.getByTestId("chapter-body-error");
    expect(box.textContent).toContain("已经不在了");
    expect(screen.queryByTestId("chapter-body-input")).toBeNull();

    fireEvent.click(screen.getByTestId("chapter-body-retry"));
    expect(reloadChapter).toHaveBeenCalledTimes(1);
  });

  it("网络错也能重试，不会留下空白抽屉", () => {
    chapterState.current = {
      data: null,
      loading: false,
      error: new ApiErrorClass("network", "网络不通"),
      reload: reloadChapter,
    } as unknown as typeof chapterState.current;
    renderAndOpen();
    expect(screen.getByTestId("chapter-body-error").textContent).toContain("检查一下网络");
    expect(screen.getByTestId("chapter-body-retry")).toBeTruthy();
  });
});
