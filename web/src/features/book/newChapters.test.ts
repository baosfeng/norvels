import { describe, expect, it, vi, beforeEach } from "vitest";
import type { BookDetail, ChapterBrief } from "../../api/types";

const ackNewChapters = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { ...actual.api, ackNewChapters: (id: number) => ackNewChapters(id) } };
});

const { isLastChapter, ackNewChaptersIfLast } = await import("./newChapters");

const chapters: ChapterBrief[] = [
  { id: 11, idx: 0, title: "第一章", volumeId: 1, wordCount: 100 },
  { id: 12, idx: 1, title: "第二章", volumeId: 1, wordCount: 100 },
];

function book(over: Partial<BookDetail> = {}): BookDetail {
  return {
    id: 7,
    title: "示例书",
    authorId: 1,
    authorName: "示例作者",
    chapterCount: 2,
    wordCount: 200,
    volumeCount: 1,
    intro: "",
    status: "parsed",
    importedAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
    authors: ["示例作者"],
    newChapters: 2,
    lastChapterIdx: 1,
    volumes: [{ id: 1, idx: 0, title: "未分卷", chapters }],
    ...over,
  };
}

beforeEach(() => {
  ackNewChapters.mockReset();
  ackNewChapters.mockResolvedValue(undefined);
});

describe("追更角标清零", () => {
  it("最后一章的判据优先用 lastChapterIdx，缺省时按目录最大 idx 兜底", () => {
    expect(isLastChapter(book(), chapters[1])).toBe(true);
    expect(isLastChapter(book(), chapters[0])).toBe(false);
    expect(isLastChapter(book({ lastChapterIdx: undefined }), chapters[1])).toBe(true);
  });

  it("打开最后一章才清角标", async () => {
    const onCleared = vi.fn();
    await ackNewChaptersIfLast(book(), chapters[1], onCleared);
    expect(ackNewChapters).toHaveBeenCalledWith(7);
    expect(onCleared).toHaveBeenCalled();
  });

  it("不是最后一章 / 没有待清角标时一个请求都不发", async () => {
    await ackNewChaptersIfLast(book(), chapters[0]);
    await ackNewChaptersIfLast(book({ newChapters: 0 }), chapters[1]);
    expect(ackNewChapters).not.toHaveBeenCalled();
  });

  it("清零失败不抛错（只影响角标数字）", async () => {
    ackNewChapters.mockRejectedValueOnce(new Error("离线"));
    await expect(ackNewChaptersIfLast(book(), chapters[1])).resolves.toBeUndefined();
  });
});
