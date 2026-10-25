import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterDetail, Stats } from "../../api/types";

/**
 * A 项的验收用例：写操作成功后左栏统计要重取一次（不是整页刷新，也不清空旧数字）。
 * 这里同时挂真实的 Sidebar（useStats 的真实实现）与真实的章节区，
 * 只把 api 换成桩——通道（lib/statsRevision → useStats.reload）走的是真代码。
 */
const { stats, chapter, updateChapterContent, deleteChapter, readingProgress } = vi.hoisted(() => ({
  stats: vi.fn(),
  chapter: vi.fn(),
  updateChapterContent: vi.fn(),
  deleteChapter: vi.fn(),
  readingProgress: vi.fn(),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { stats, chapter, updateChapterContent, deleteChapter, readingProgress } };
});

const { AdminChapters } = await import("./AdminChapters");
const { Sidebar } = await import("../../layouts/Sidebar");

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
        { id: 10, idx: 0, title: "巷口的灯", volumeId: 1, wordCount: 16 },
        { id: 11, idx: 1, title: "第 2 章 远山", volumeId: 1, wordCount: 2982 },
      ],
    },
  ],
};

function statsValue(words: number): Stats {
  return { books: 3, chapters: 5, authors: 2, words, dbBytes: 1_000_000, lastImportAt: null };
}

function detail(): ChapterDetail {
  return {
    bookId: 7,
    bookTitle: book.title,
    id: 10,
    idx: 0,
    title: "巷口的灯",
    content: BODY,
    wordCount: 16,
    prevId: 0,
    nextId: 11,
  };
}

const noop = () => undefined;

function renderAdmin() {
  render(
    <MemoryRouter>
      <Sidebar />
      <AdminChapters book={book} onChanged={noop} onError={noop} onReload={noop} />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  // 第一次取到 1000，之后的每次重取都是 2000：重取发生没发生，看数字就知道
  let round = 0;
  stats.mockImplementation(async () => statsValue(round++ === 0 ? 1000 : 2000));
  readingProgress.mockResolvedValue({ items: [] });
  chapter.mockResolvedValue(detail());
  updateChapterContent.mockResolvedValue(undefined);
  deleteChapter.mockResolvedValue(undefined);
});

describe("写操作成功后左栏统计重取", () => {
  it("正文保存成功 → stats 再取一次，侧栏字数从 1000 变成 2000", async () => {
    renderAdmin();
    await waitFor(() => expect(screen.getByText("1000")).toBeTruthy());
    expect(stats).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByTestId("admin-chapter-body-10"));
    const input = (await screen.findByTestId("chapter-body-input")) as HTMLTextAreaElement;
    fireEvent.change(input, { target: { value: `${BODY}\n\n又添了一段。` } });
    fireEvent.click(screen.getByTestId("chapter-body-save"));

    await waitFor(() =>
      expect(updateChapterContent).toHaveBeenCalledWith(7, 10, expect.stringContaining("又添了一段。")),
    );
    await waitFor(() => expect(screen.getByText("2000")).toBeTruthy());
    expect(stats).toHaveBeenCalledTimes(2);
  });

  it("保存失败不重取（统计只跟成功的写操作走）", async () => {
    updateChapterContent.mockRejectedValue(new Error("网络断了"));
    renderAdmin();
    await waitFor(() => expect(screen.getByText("1000")).toBeTruthy());

    fireEvent.click(screen.getByTestId("admin-chapter-body-10"));
    const input = (await screen.findByTestId("chapter-body-input")) as HTMLTextAreaElement;
    fireEvent.change(input, { target: { value: "改过的一版" } });
    fireEvent.click(screen.getByTestId("chapter-body-save"));

    await waitFor(() => expect(screen.getByTestId("chapter-body-save-error")).toBeTruthy());
    expect(stats).toHaveBeenCalledTimes(1);
    expect(screen.getByText("1000")).toBeTruthy();
  });

  it("删除一章成功 → stats 再取一次", async () => {
    renderAdmin();
    await waitFor(() => expect(screen.getByText("1000")).toBeTruthy());

    fireEvent.click(screen.getByTestId("admin-chapter-delete-10"));
    fireEvent.click(screen.getByTestId("chapter-delete-confirm"));

    await waitFor(() => expect(deleteChapter).toHaveBeenCalledWith(7, 10));
    await waitFor(() => expect(screen.getByText("2000")).toBeTruthy());
    expect(stats).toHaveBeenCalledTimes(2);
  });
});
