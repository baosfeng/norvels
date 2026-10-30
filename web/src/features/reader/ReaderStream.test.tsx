import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterBrief, ChapterDetail } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const chapters: ChapterBrief[] = [
  { id: 10, idx: 0, title: "第一章 起风", volumeId: 1, wordCount: 2000 },
  { id: 11, idx: 1, title: "第二章 落雨", volumeId: 1, wordCount: 2100 },
  { id: 12, idx: 2, title: "第三章 归途", volumeId: 1, wordCount: 2200 },
  { id: 13, idx: 3, title: "第四章 长夜", volumeId: 1, wordCount: 2300 },
  { id: 14, idx: 4, title: "第五章 收束", volumeId: 1, wordCount: 2400 },
];

const book: BookDetail = {
  id: 7,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: 5,
  wordCount: 11000,
  volumeCount: 1,
  intro: "",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  volumes: [{ id: 1, idx: 0, title: "正文", chapters }],
};

/** 每章正文互不相同：同屏两章时断言才不会撞车。 */
function chapterOf(id: number): ChapterDetail {
  const brief = chapters.find((c) => c.id === id) ?? chapters[0];
  return {
    bookId: 7,
    bookTitle: book.title,
    id: brief.id,
    idx: brief.idx,
    title: brief.title,
    content: `第${brief.idx + 1}章的第一段。\n\n第${brief.idx + 1}章的第二段。`,
    wordCount: brief.wordCount,
    prevId: 0,
    nextId: 0,
  };
}

const spies = vi.hoisted(() => ({
  saveProgress: vi.fn(async () => true),
  readingProgress: vi.fn(async () => ({ items: [] })),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { ...actual.api, readingProgress: spies.readingProgress } };
});

vi.mock("../../api/hooks", () => ({
  useBook: () => ({ data: book, loading: false, error: null, reload: vi.fn() }),
  useChapter: (_bookId: number, chapterId: number) => ({
    data: chapterOf(chapterId),
    loading: false,
    error: null,
    reload: vi.fn(),
  }),
  useSaveProgress: () => spies.saveProgress,
  useRecordReadingSession: () => vi.fn(async () => true),
  useBookmarks: () => ({ data: { bookmarks: [] }, loading: false, error: null, reload: vi.fn() }),
}));

vi.mock("../../lib/theme", async () => {
  const actual = await vi.importActual<typeof import("../../lib/theme")>("../../lib/theme");
  return {
    ...actual,
    useTheme: () => ({
      theme: "day",
      resolvedTheme: "day",
      setTheme: vi.fn(),
      themes: ["day", "night", "parchment", "system"],
    }),
    prefersReducedMotion: () => false,
  };
});

const { showToast } = vi.hoisted(() => ({ showToast: vi.fn() }));
vi.mock("../../lib/toastStore", () => ({ showToast }));

/** 手摇 IntersectionObserver：用例自己决定「哪一章压在判定带里」。 */
class FakeObserver {
  static instances: FakeObserver[] = [];
  callback: IntersectionObserverCallback;
  targets = new Set<Element>();
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
    FakeObserver.instances.push(this);
  }
  observe(node: Element) {
    this.targets.add(node);
  }
  unobserve(node: Element) {
    this.targets.delete(node);
  }
  disconnect() {
    FakeObserver.instances = FakeObserver.instances.filter((item) => item !== this);
  }
  takeRecords() {
    return [];
  }
  /** 让某一章的 section 进入判定带（只发给真的观察了它的实例）。 */
  static emit(chapterId: number) {
    const node = document.querySelector(`[data-chapter-id="${chapterId}"]`);
    if (!node) throw new Error(`没有第 ${chapterId} 章的 section`);
    for (const observer of FakeObserver.instances) {
      if (!observer.targets.has(node)) continue;
      observer.callback(
        [{ target: node, isIntersecting: true, intersectionRatio: 1 } as unknown as IntersectionObserverEntry],
        observer as unknown as IntersectionObserver,
      );
    }
  }
  static reset() {
    FakeObserver.instances = [];
  }
}

function PathProbe() {
  const location = useLocation();
  return <p data-testid="path-probe">{location.pathname}</p>;
}

const { ReaderPage } = await import("./ReaderPage");

function renderReader(entries: string[] = ["/book/7", "/read/7/10"], initialIndex = 1) {
  return render(
    <TierProvider value="h5">
      <MemoryRouter initialEntries={entries} initialIndex={initialIndex}>
        <PathProbe />
        <Routes>
          <Route path="/read/:bookId/:chapterId" element={<ReaderPage />} />
          <Route path="/book/:bookId" element={<p>书籍详情</p>} />
          <Route path="/shelf" element={<p>书架页</p>} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

function surface(): HTMLElement {
  return screen.getByTestId("reader-swipe-area");
}

function path(): string {
  return screen.getByTestId("path-probe").textContent ?? "";
}

/** 某一章 section 当前的屏幕位置（桩版面：位置 = 前面已渲染章节的高度和 − scrollTop）。 */
function segmentTop(chapterId: number): number {
  return document.querySelector<HTMLElement>(`[data-chapter-id="${chapterId}"]`)!.getBoundingClientRect().top;
}

/** 每章正文的高度（jsdom 不做布局，位置补偿要有真实数字）。 */
const SEGMENT_HEIGHT = 1000;
/** 正文容器视口高：只要「可滚距离 > 0」，补偿就该写进容器自己的 scrollTop。 */
const VIEW_HEIGHT = 800;

/**
 * 造一份「可测的版面」：每章高 1000，位置 = 前面所有已渲染章节的高度和；section 的屏幕位置随 scrollTop 上移。
 * 窗口在可见章上方增减一章后，后面的章位置整体前后移动——滚动补偿要拿它来量增减量。
 */
function stubLayout() {
  // 正文容器当成 H5 的可滚容器：jsdom 不做布局，滚动指标只能显式给
  Object.defineProperty(HTMLElement.prototype, "scrollHeight", {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset?.testid === "reader-swipe-area" ? SEGMENT_HEIGHT * 8 : 0;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset?.testid === "reader-swipe-area" ? VIEW_HEIGHT : 0;
    },
  });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const surfaceEl = document.querySelector<HTMLElement>('[data-testid="reader-swipe-area"]');
    const scrollTop = surfaceEl ? surfaceEl.scrollTop : 0;
    // 流的顶随内容一起滚：章节「在流里的位置」要拿它当原点，才与滚动无关
    const stream = document.querySelector<HTMLElement>(".reader-stream");
    if (this === stream) {
      const total = document.querySelectorAll("section[data-chapter-id]").length * SEGMENT_HEIGHT;
      return { top: -scrollTop, bottom: -scrollTop + total, height: total, left: 0, right: 0, width: 0, x: 0, y: 0 } as DOMRect;
    }
    const id = this.dataset?.chapterId;
    if (!id) return { top: 0, bottom: 0, height: 0, left: 0, right: 0, width: 0, x: 0, y: 0 } as DOMRect;
    let start = 0;
    for (const section of Array.from(document.querySelectorAll<HTMLElement>("section[data-chapter-id]"))) {
      if (section.dataset.chapterId === id) break;
      start += SEGMENT_HEIGHT;
    }
    const top = start - scrollTop;
    return { top, bottom: top + SEGMENT_HEIGHT, height: SEGMENT_HEIGHT, left: 0, right: 0, width: 0, x: 0, y: 0 } as DOMRect;
  });
}

beforeEach(() => {
  FakeObserver.reset();
  vi.clearAllMocks();
  vi.stubGlobal("IntersectionObserver", FakeObserver);
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  stubLayout();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("连续章节流：章末无缝衔接下一章", () => {
  it("进章就把下一章预取渲染在下面：不用点、标题与正文都在 DOM 里", () => {
    renderReader();
    const next = screen.getByTestId("reader-segment-11");
    expect(next.textContent).toContain("第二章 落雨");
    expect(next.textContent).toContain("第2章的第一段。");
    // 章末不再有大按钮「下一章」卡片
    expect(screen.queryByRole("button", { name: /下一章/ })).toBeNull();
    expect(screen.getByTestId("reader-stream-mark").textContent).toContain("本章完");
  });

  it("滚过章界线：可见章换成第二章，URL 跟着换成第二章，滚动位置一动不动", async () => {
    renderReader();
    const area = surface();
    area.scrollTop = 640;

    act(() => FakeObserver.emit(11));

    await waitFor(() => expect(path()).toBe("/read/7/11"));
    // replace 不新增历史条目：后退仍然回到离开阅读器前的详情页
    expect(area.scrollTop).toBe(640);
    // 上一章还在（向上回看有内容），没有被摘掉
    expect(screen.getByTestId("reader-segment-10")).toBeTruthy();
    // 进度上报跟着可见章走
    await waitFor(() => expect(spies.saveProgress).toHaveBeenCalledWith(expect.objectContaining({ chapterId: 11 })));
  });

  it("窗口向前移动摘掉上方章节时把高度退回 scrollTop：读者看到的正文不动、不跳", async () => {
    renderReader(["/book/7", "/read/7/12"], 1);
    const area = surface();
    area.scrollTop = 2600;
    const before = segmentTop(12);
    act(() => FakeObserver.emit(13));
    await waitFor(() => expect(path()).toBe("/read/7/13"));

    // 上方那一章（11）被摘掉：滚动位置退回它的一章高度，眼前正文位置不变
    expect(screen.queryByTestId("reader-segment-11")).toBeNull();
    expect(area.scrollTop).toBe(1600);
    expect(segmentTop(12)).toBeCloseTo(before, 0);
    // 没有空白占位块：补的是 scrollTop，不是一屏空 div
    expect(document.querySelector(".reader-stream-spacer")).toBeNull();
  });

  it("向上滚回上一章：可见章与 URL 一起退回去", async () => {
    renderReader(["/book/7", "/read/7/11"], 1);
    act(() => FakeObserver.emit(10));
    await waitFor(() => expect(path()).toBe("/read/7/10"));
    expect(screen.getByTestId("reader-segment-11")).toBeTruthy();
  });

  it("向上回看补章：上一章插进流顶部，插入的高度补进 scrollTop，正文不跳", async () => {
    renderReader(["/book/7", "/read/7/12"], 1);
    const area = surface();
    area.scrollTop = 500;
    const before = segmentTop(12);
    act(() => FakeObserver.emit(11));
    await waitFor(() => expect(path()).toBe("/read/7/11"));

    // 上一章（10）补在流的起点：scrollTop 加上它的高度，读者看到的还是原来那一行
    expect(screen.getByTestId("reader-segment-10").textContent).toContain("第1章的第一段。");
    expect(area.scrollTop).toBe(1500);
    expect(segmentTop(12)).toBeCloseTo(before, 0);
    expect(document.querySelector(".reader-stream-spacer")).toBeNull();
  });

  it("连续向上回看：每次都能补出上一章，scrollTop 不归 0，流里没有空白块", async () => {
    renderReader(["/book/7", "/read/7/13"], 1);
    const area = surface();
    area.scrollTop = 1200;
    for (const id of [12, 11, 10]) {
      act(() => FakeObserver.emit(id));
      await waitFor(() => expect(path()).toBe(`/read/7/${id}`));
      // 可见章有正文、URL 跟上了，滚动位置还在可滚范围内（不再掉回 0 撞进一片空白）
      expect(screen.getByTestId(`reader-segment-${id}`).querySelector("p")).toBeTruthy();
      expect(area.scrollTop).toBeGreaterThan(0);
      expect(document.querySelector(".reader-stream-spacer")).toBeNull();
    }
    // 四章一路回看到第一章：流起点是第一章，再往上才是书首
    expect(screen.getByTestId("reader-segment-10")).toBeTruthy();
  });

  it("末章：全书读完的出口，可回详情页与书架", () => {
    renderReader(["/book/7", "/read/7/14"], 1);
    const end = screen.getByTestId("reader-pager-end");
    expect(end.textContent).toContain("全书读完");
    expect(within(end).getByRole("link", { name: "回详情页" })).toBeTruthy();
    fireEvent.click(within(end).getByRole("button", { name: "回书架" }));
    expect(screen.getByText("书架页")).toBeTruthy();
  });

  it("末章不预取也不显示下一章；章节切换只剩轻量的「← 上一章」", () => {
    renderReader(["/book/7", "/read/7/14"], 1);
    expect(screen.queryByTestId("reader-segment-15")).toBeNull();
    expect(screen.getByRole("button", { name: "← 上一章" }).hasAttribute("disabled")).toBe(false);
    expect(screen.queryByRole("button", { name: /下一章/ })).toBeNull();
  });
});