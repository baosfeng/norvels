import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AuthorSummary,
  BookDetail,
  Bookmark,
  BookSummary,
  ChapterDetail,
  HealthReport,
  ImportJob,
  ImportTrendPoint,
  Page,
  ReadingProgress,
  SearchResult,
  Stats,
} from "./types";

const stats: Stats = {
  books: 19,
  chapters: 11668,
  authors: 17,
  words: 30418864,
  dbBytes: 326553600,
  lastImportAt: "2026-10-24T20:25:26.409Z",
};

const book: BookSummary = {
  id: 1,
  title: "测试书名",
  authorId: 1,
  authorName: "测试作者",
  chapterCount: 106,
  wordCount: 381769,
  volumeCount: 1,
  intro: "",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-24T20:24:50.859Z",
};

const page = <T,>(item: T): Page<T> => ({ items: [item], total: 1, page: 1, size: 20 });

const mocks = {
  stats: vi.fn(async () => stats),
  healthReport: vi.fn(async (): Promise<HealthReport> => ({
    totalBooks: 19,
    thinBooks: 0,
    missingAuthor: 0,
    missingIntro: 9,
    hugeChapters: 0,
    chapterBuckets: [{ label: "1", count: 0 }],
    chapterLengthBuckets: [{ label: "<1千字", count: 10 }],
    warnings: [],
  })),
  importTrend: vi.fn(async (): Promise<ImportTrendPoint[]> => [{ date: "2026-10-24", books: 12, chapters: 1234 }]),
  books: vi.fn(async () => page(book)),
  book: vi.fn(async (): Promise<BookDetail> => ({ ...book, authors: [book.authorName], volumes: [] })),
  chapter: vi.fn(async (): Promise<ChapterDetail> => ({
    bookId: 1,
    bookTitle: book.title,
    id: 1,
    idx: 0,
    title: "第一章",
    content: "正文",
    wordCount: 2,
    prevId: 0,
    nextId: 0,
  })),
  authors: vi.fn(async () => page<AuthorSummary>({ id: 9, name: "佚名", bookCount: 3, wordCount: 100 })),
  search: vi.fn(async (q: string): Promise<SearchResult> => ({
    q,
    books: { items: [], total: 0, page: 1, size: 20 },
    chapters: page({
      bookId: 1,
      bookTitle: book.title,
      chapterId: 2,
      chapterIdx: 1,
      chapterTitle: "第二章",
      snippet: "…<b>词</b>…",
      score: 1,
    }),
  })),
  importJobs: vi.fn(async () => ({ items: [job] })),
  readingProgress: vi.fn(async () => ({ items: [progress] })),
  saveProgress: vi.fn(async () => undefined),
  bookmarks: vi.fn(async (): Promise<{ bookmarks: Bookmark[] }> => ({ bookmarks: [bookmark] })),
  tagRules: vi.fn(async () => ({ rules: [{ tag: "奇幻", keywords: ["魔法"] }] })),
  importJobFiles: vi.fn(async () => ({
    files: [
      {
        id: 1,
        source: "/path/to/txt/a.txt",
        displayPath: "a.txt",
        status: "failed" as const,
        reason: "parse_failed",
        bookId: 0,
        updatedAt: "2026-10-25T10:01:00.000Z",
      },
    ],
  })),
};

const bookmark: Bookmark = {
  id: 1,
  chapterId: 2,
  chapterIdx: 1,
  chapterTitle: "第二章",
  paraIndex: 3,
  label: "雨从傍晚开始下。",
  createdAt: "2026-10-25T10:00:00.000Z",
};

const job: ImportJob = {
  id: 1,
  sourceDir: "/tmp/books",
  status: "completed",
  total: 20,
  imported: 19,
  skipped: 1,
  failed: 0,
  startedAt: "2026-10-24T20:24:50.745Z",
  finishedAt: "2026-10-24T20:25:26.562Z",
  message: "",
};

const progress: ReadingProgress = {
  bookId: 1,
  bookTitle: book.title,
  chapterId: 2,
  chapterIdx: 1,
  chapterTitle: "第二章",
  percent: 0.36,
  updatedAt: "2026-10-24T20:30:00.000Z",
};

vi.mock("./client", async () => {
  const actual = await vi.importActual<typeof import("./client")>("./client");
  return { ...actual, api: mocks };
});

const hooks = await import("./hooks");

describe("各域取数 hook 都能拿到数据", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("useStats", async () => {
    const { result } = renderHook(() => hooks.useStats());
    await waitFor(() => expect(result.current.data?.books).toBe(19));
    expect(mocks.stats).toHaveBeenCalled();
  });

  it("useHealthReport", async () => {
    const { result } = renderHook(() => hooks.useHealthReport());
    await waitFor(() => expect(result.current.data?.totalBooks).toBe(19));
  });

  it("useImportTrend 按天数取", async () => {
    const { result } = renderHook(() => hooks.useImportTrend(14));
    await waitFor(() => expect(result.current.data).toHaveLength(1));
    expect(mocks.importTrend).toHaveBeenCalledWith(14, expect.anything());
  });

  it("useBooks 与 useBook / useChapter", async () => {
    const list = renderHook(() => hooks.useBooks({ sort: "updated", page: 1, size: 20 }));
    await waitFor(() => expect(list.result.current.data?.total).toBe(1));

    const detail = renderHook(() => hooks.useBook(1));
    await waitFor(() => expect(detail.result.current.data?.title).toBe("测试书名"));

    const chapter = renderHook(() => hooks.useChapter(1, 2));
    await waitFor(() => expect(chapter.result.current.data?.title).toBe("第一章"));
  });

  it("useAuthors", async () => {
    const { result } = renderHook(() => hooks.useAuthors({ sort: "words" }));
    await waitFor(() => expect(result.current.data?.items[0].name).toBe("佚名"));
  });

  it("useSearch 关键词不足 2 字不发请求", async () => {
    const short = renderHook(() => hooks.useSearch("字"));
    expect(short.result.current.loading).toBe(false);
    expect(mocks.search).not.toHaveBeenCalled();

    const ok = renderHook(() => hooks.useSearch("两个字"));
    await waitFor(() => expect(ok.result.current.data?.chapters.total).toBe(1));
    expect(mocks.search).toHaveBeenCalledWith("两个字", { type: "all" }, expect.anything());
  });

  it("useImportJobs 与 useReadingProgress", async () => {
    const jobs = renderHook(() => hooks.useImportJobs());
    await waitFor(() => expect(jobs.result.current.data?.items).toHaveLength(1));

    const reading = renderHook(() => hooks.useReadingProgress());
    await waitFor(() => expect(reading.result.current.data?.items[0].percent).toBe(0.36));
  });

  it("useBookmarks 取回本书书签，书不存在时不发请求", async () => {
    const list = renderHook(() => hooks.useBookmarks(1));
    await waitFor(() => expect(list.result.current.data?.bookmarks[0].label).toBe("雨从傍晚开始下。"));
    expect(mocks.bookmarks).toHaveBeenCalledWith(1, expect.anything());

    const disabled = renderHook(() => hooks.useBookmarks(null));
    expect(disabled.result.current.loading).toBe(false);
    expect(disabled.result.current.data).toBeNull();
  });

  it("useTagRules 与 useFailedFiles 都走同一出口，任务为空时不发请求", async () => {
    const rules = renderHook(() => hooks.useTagRules());
    await waitFor(() => expect(rules.result.current.data?.rules[0].tag).toBe("奇幻"));

    const files = renderHook(() => hooks.useFailedFiles(7));
    await waitFor(() => expect(files.result.current.data?.files[0].displayPath).toBe("a.txt"));
    expect(mocks.importJobFiles).toHaveBeenCalledWith(7, "failed", expect.anything());

    const disabled = renderHook(() => hooks.useFailedFiles(null));
    expect(disabled.result.current.loading).toBe(false);
    expect(disabled.result.current.data).toBeNull();
  });

  it("useSaveProgress 落库失败返回 false 而不是抛错", async () => {
    const { result } = renderHook(() => hooks.useSaveProgress());
    await expect(result.current({ bookId: 1, chapterId: 2, percent: 0.1 })).resolves.toBe(true);

    mocks.saveProgress.mockRejectedValueOnce(new Error("offline"));
    await expect(result.current({ bookId: 1, chapterId: 3, percent: 0.2 })).resolves.toBe(false);
  });
});
