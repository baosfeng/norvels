import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, Bookmark, ChapterDetail } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const chapters = [
  { id: 10, idx: 0, title: "第一章", volumeId: 1, wordCount: 2000 },
  { id: 11, idx: 1, title: "第二章", volumeId: 1, wordCount: 2100 },
];

/** 5 段正文，关键词「雨」出现 3 处（第 1 / 4 / 5 段各一处） */
const CONTENT = [
  "雨从傍晚开始下。",
  "巷口的灯还亮着。",
  "他站着，没有打伞。",
  "雨声更大了一些。",
  "远处有人收摊，雨停了。",
].join("\n");

const book: BookDetail = {
  id: 7,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: 2,
  wordCount: 4100,
  volumeCount: 1,
  intro: "",
  status: "parsed",
  readStatus: "reading",
  bookmarkCount: 0,
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  volumes: [{ id: 1, idx: 0, title: "正文", chapters }],
};

function chapterOf(chapterId: number): ChapterDetail {
  const brief = chapters.find((c) => c.id === chapterId) ?? chapters[0];
  return {
    bookId: 7,
    bookTitle: book.title,
    id: brief.id,
    idx: brief.idx,
    title: brief.title,
    content: CONTENT,
    wordCount: brief.wordCount,
    prevId: 0,
    nextId: 0,
  };
}

// 内存书签库：行为对齐后端契约（同 (chapterId, paraIndex) 幂等、按时间倒序）
let store: Bookmark[] = [];
let nextId = 1;

const chapterApi = vi.fn(async (_bookId: number, chapterId: number) => chapterOf(chapterId));
const bookmarksApi = vi.fn(async () => ({ bookmarks: [...store] }));
const createBookmark = vi.fn(async (_bookId: number, body: { chapterId: number; paraIndex: number; label?: string }) => {
  const existing = store.find((item) => item.chapterId === body.chapterId && item.paraIndex === body.paraIndex);
  if (existing) {
    existing.label = body.label ?? existing.label;
    return existing;
  }
  const created: Bookmark = {
    id: nextId++,
    chapterId: body.chapterId,
    chapterIdx: chapters.find((c) => c.id === body.chapterId)?.idx ?? 0,
    chapterTitle: chapters.find((c) => c.id === body.chapterId)?.title ?? "",
    paraIndex: body.paraIndex,
    label: body.label ?? "",
    createdAt: "2026-10-25T10:00:00.000Z",
  };
  store = [created, ...store];
  return created;
});
const deleteBookmark = vi.fn(async (id: number) => {
  store = store.filter((item) => item.id !== id);
});

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      book: vi.fn(async () => book),
      chapter: chapterApi,
      saveProgress: vi.fn(async () => undefined),
      bookmarks: bookmarksApi,
      createBookmark,
      deleteBookmark,
    },
  };
});

vi.mock("../../lib/theme", async () => {
  const actual = await vi.importActual<typeof import("../../lib/theme")>("../../lib/theme");
  return {
    ...actual,
    useTheme: () => ({ theme: "day", setTheme: vi.fn(), themes: ["day", "night", "parchment"] }),
    prefersReducedMotion: () => false,
  };
});

const { ReaderPage } = await import("./ReaderPage");

function renderReader() {
  return render(
    <TierProvider value="pc">
      <MemoryRouter initialEntries={["/read/7/10"]}>
        <Routes>
          <Route path="/read/:bookId/:chapterId" element={<ReaderPage />} />
          <Route path="/book/:bookId" element={<p>书籍详情</p>} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

async function openReader() {
  renderReader();
  await screen.findByText("雨从傍晚开始下。");
}

function countText(): string {
  return screen.getByTestId("reader-search-count").textContent ?? "";
}

describe("阅读器：章节内搜索（纯前端）", () => {
  beforeEach(() => {
    store = [];
    nextId = 1;
    vi.clearAllMocks();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  it("命中 3 处：显示「1 / 3」，点下一处变「2 / 3」，正文只有 3 个高亮", async () => {
    await openReader();

    fireEvent.click(screen.getByLabelText("搜索本章"));
    fireEvent.change(screen.getByTestId("reader-search-input"), { target: { value: "雨" } });

    expect(countText()).toContain("1 / 3");
    expect(document.querySelectorAll("mark[data-hit]")).toHaveLength(3);
    expect(document.querySelectorAll("mark.is-current")).toHaveLength(1);

    fireEvent.click(screen.getByLabelText("下一处"));
    expect(countText()).toContain("2 / 3");

    // 翻到最后一处再点回第一处（循环）
    fireEvent.click(screen.getByLabelText("下一处"));
    expect(countText()).toContain("3 / 3");
    fireEvent.click(screen.getByLabelText("下一处"));
    expect(countText()).toContain("1 / 3");
  });

  it("无匹配时显示「无匹配」，且正文没有高亮", async () => {
    await openReader();

    fireEvent.click(screen.getByLabelText("搜索本章"));
    fireEvent.change(screen.getByTestId("reader-search-input"), { target: { value: "雪" } });

    expect(countText()).toBe("无匹配");
    expect(document.querySelectorAll("mark[data-hit]")).toHaveLength(0);
  });

  it("关闭搜索清除关键词与高亮", async () => {
    await openReader();

    fireEvent.click(screen.getByLabelText("搜索本章"));
    fireEvent.change(screen.getByTestId("reader-search-input"), { target: { value: "雨" } });
    expect(document.querySelectorAll("mark[data-hit]")).toHaveLength(3);

    fireEvent.click(screen.getByLabelText("关闭搜索"));
    expect(screen.queryByTestId("reader-search")).toBeNull();
    expect(document.querySelectorAll("mark[data-hit]")).toHaveLength(0);
  });
});

describe("阅读器：书签幂等与抽屉", () => {
  beforeEach(() => {
    store = [];
    nextId = 1;
    vi.clearAllMocks();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  it("同位置再点「在这里加书签」不新增条目，也不重复发请求", async () => {
    await openReader();

    fireEvent.click(screen.getByLabelText("打开书签"));
    expect(screen.getByTestId("bookmark-drawer").getAttribute("aria-hidden")).toBe("false");
    expect(screen.getByTestId("bookmark-empty")).toBeTruthy();

    fireEvent.click(screen.getByTestId("bookmark-add"));
    await waitFor(() => expect(createBookmark).toHaveBeenCalledTimes(1));
    expect(createBookmark.mock.calls[0][1]).toMatchObject({ chapterId: 10, paraIndex: 0, label: "雨从傍晚开始下。" });
    await waitFor(() => expect(screen.getByTestId("bookmark-list")).toBeTruthy());
    expect(screen.getByTestId("bookmark-list").querySelectorAll(".bookmark-row")).toHaveLength(1);

    // 同章同段再点一次：前端直接拦下，不发第二个 POST
    fireEvent.click(screen.getByTestId("bookmark-add"));
    await waitFor(() => expect(store).toHaveLength(1));
    expect(createBookmark).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("bookmark-list").querySelectorAll(".bookmark-row")).toHaveLength(1);
  });

  it("点书签跳到那一章，删除后回到空态", async () => {
    store = [
      {
        id: 99,
        chapterId: 11,
        chapterIdx: 1,
        chapterTitle: "第二章",
        paraIndex: 1,
        label: "巷口的灯还亮着。",
        createdAt: "2026-10-24T09:00:00.000Z",
      },
    ];
    await openReader();

    fireEvent.click(screen.getByLabelText("打开书签"));
    await waitFor(() => expect(screen.getByTestId("bookmark-list")).toBeTruthy());

    const row = screen.getByTestId("bookmark-99");
    // 第一列是「跳到该段」，第二列是删除
    fireEvent.click(within(row).getAllByRole("button")[0]);
    await waitFor(() => expect(chapterApi.mock.calls.at(-1)?.slice(0, 2)).toEqual([7, 11]));

    fireEvent.click(screen.getByLabelText("打开书签"));
    await waitFor(() => expect(screen.getByTestId("bookmark-99")).toBeTruthy());
    fireEvent.click(screen.getByLabelText("删除书签 巷口的灯还亮着。"));
    await waitFor(() => expect(deleteBookmark).toHaveBeenCalledWith(99));
    await waitFor(() => expect(screen.getByTestId("bookmark-empty")).toBeTruthy());
  });
});
