import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterBrief, ChapterDetail } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const chapters: ChapterBrief[] = [
  { id: 10, idx: 0, title: "第一章 起风", volumeId: 1, wordCount: 2000 },
  { id: 11, idx: 1, title: "第二章 落雨", volumeId: 1, wordCount: 2100 },
];

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
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  volumes: [{ id: 1, idx: 0, title: "正文", chapters }],
};

/** 六段正文：翻章后要能滚到章首，段落 id 也要对得上恢复逻辑。 */
const PARAGRAPHS = Array.from({ length: 6 }, (_, i) => `第 ${i} 段正文，用来把章节撑得足够长。`);

function chapterOf(id: number): ChapterDetail {
  const brief = chapters.find((c) => c.id === id) ?? chapters[0];
  return {
    bookId: 7,
    bookTitle: book.title,
    id: brief.id,
    idx: brief.idx,
    title: brief.title,
    content: PARAGRAPHS.join("\n\n"),
    wordCount: brief.wordCount,
    prevId: 0,
    nextId: 0,
  };
}

/** 上次读到第 1 章第 2 段（paraIndex=2）：目录点回 / 浏览器后退要恢复到这段。 */
const PROGRESS = [
  { bookId: 7, chapterId: 10, chapterTitle: chapters[0].title, chapterIdx: 0, percent: 0.4, paraIndex: 2, updatedAt: "2026-10-30T00:00:00Z" },
];

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: { ...actual.api, readingProgress: async () => ({ items: PROGRESS }) },
  };
});

vi.mock("../../api/hooks", () => ({
  useBook: () => ({ data: book, loading: false, error: null, reload: vi.fn() }),
  useChapter: (_bookId: number, chapterId: number) => ({
    data: chapterOf(chapterId),
    loading: false,
    error: null,
    reload: vi.fn(),
  }),
  useSaveProgress: () => vi.fn(async () => true),
  useRecordReadingSession: () => vi.fn(async () => true),
  useBookmarks: () => ({ data: { bookmarks: [] }, loading: false, error: null, reload: vi.fn() }),
}));

vi.mock("../../lib/theme", async () => {
  const actual = await vi.importActual<typeof import("../../lib/theme")>("../../lib/theme");
  return {
    ...actual,
    useTheme: () => ({ theme: "day", resolvedTheme: "day", setTheme: vi.fn(), themes: ["day"] }),
    prefersReducedMotion: () => false,
  };
});

vi.mock("../../lib/toastStore", () => ({ showToast: vi.fn() }));

const { ReaderPage } = await import("./ReaderPage");

function renderReader(entry: string) {
  return render(
    <TierProvider value="h5">
      <MemoryRouter initialEntries={[entry]}>
        <Routes>
          <Route path="/read/:bookId/:chapterId" element={<ReaderPage />} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

/** 顶栏进度文本（「第 2 章 / 共 2 章」）就是当前章号：路由变化不必另做探针。 */
function progressText(): string {
  return screen.getByTestId("reader-top-progress").textContent ?? "";
}

/** 正文容器：H5 下 .reader-main 自己滚，window 不滚。 */
function surface(): HTMLElement {
  return screen.getByTestId("reader-swipe-area");
}

/** jsdom 没有 Touch / TouchEvent 构造器：造一个带 touches / changedTouches 的事件，hook 只读坐标。 */
function touch(type: string, x: number, y: number) {
  const point = { clientX: x, clientY: y };
  const event = new Event(type, { bubbles: true, cancelable: true });
  Object.assign(event, {
    touches: type === "touchend" ? [] : [point],
    changedTouches: [point],
    targetTouches: type === "touchend" ? [] : [point],
  });
  return event;
}

/**
 * jsdom 不做布局：给每个章节 section 一个「在正文里的起始位置」，落位逻辑才有数可算。
 * 容器自己的 rect 固定在屏幕上（top=0），section 的 rect 随 scrollTop 上移。
 */
function stubLayout(offsets: Record<string, number>) {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const id = (this as HTMLElement).dataset?.chapterId;
    const start = id === undefined ? 0 : offsets[id] ?? 0;
    const top = id === undefined ? 0 : start - surface().scrollTop;
    return { top, bottom: top, height: 0, left: 0, right: 0, width: 0, x: 0, y: 0 } as DOMRect;
  });
}

/** 一次横滑：起点 → 两个中间点 → 松手。 */
function swipe(from: number, to: number, dy = 2) {
  const node = surface();
  fireEvent(node, touch("touchstart", from, 400));
  fireEvent(node, touch("touchmove", (from + to) / 2, 400 + dy));
  fireEvent(node, touch("touchend", to, 400 + dy * 2));
}

describe("翻章与回章首", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  /**
   * 防复发：实测章内滚到 70%（scrollTop=780）左滑翻章，新章停在 56%——
   * 旧实现用 window.scrollTo({top:0}) 回章首，而正文在容器内滚动，它对这个容器无效。
   */
  it("章内滚到一半左滑翻章，新章从章首开始", async () => {
    stubLayout({ "10": 0, "11": 2000 });
    renderReader("/read/7/10");
    surface().scrollTop = 500;
    swipe(300, 200);
    await waitFor(() => expect(progressText()).toContain("第 2 章 / 共 2 章"));
    // 按「第二章的 section」对齐到屏幕顶：连续流里上方还有第一章，不能停在它里面
    await waitFor(() => expect(surface().scrollTop).toBe(2000));
    // 旧实现滚的是窗口：容器 scrollTop 一直是 500
    expect(window.scrollY).toBe(0);
  });

  it("右滑回上一章同样落在章首", async () => {
    stubLayout({ "10": 0, "11": 2000 });
    renderReader("/read/7/11");
    surface().scrollTop = 480;
    swipe(100, 220);
    await waitFor(() => expect(progressText()).toContain("第 1 章 / 共 2 章"));
    await waitFor(() => expect(surface().scrollTop).toBe(0));
  });

  it("斜向漂移（dx=70 / dy=45）不翻章：读者在往下滚", () => {
    renderReader("/read/7/10");
    const node = surface();
    fireEvent(node, touch("touchstart", 300, 200));
    fireEvent(node, touch("touchmove", 260, 240));
    fireEvent(node, touch("touchend", 230, 245));
    expect(progressText()).toContain("第 1 章 / 共 2 章");
  });

  it("目录抽屉打开时横滑不翻章，关掉后才翻", async () => {
    renderReader("/read/7/10");
    // 唤出控制层（点击中间区），再打开目录
    const node = surface();
    fireEvent.pointerDown(node, { clientX: 195, clientY: 400 });
    fireEvent.pointerUp(node, { clientX: 195, clientY: 400 });
    fireEvent.click(screen.getByLabelText("章节目录"));
    await waitFor(() => expect(screen.getByTestId("reader-drawer").getAttribute("aria-hidden")).toBe("false"));

    swipe(300, 200);
    expect(progressText()).toContain("第 1 章 / 共 2 章");
  });
});