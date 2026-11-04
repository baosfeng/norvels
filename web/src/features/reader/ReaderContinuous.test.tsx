import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ApiError } from "../../api/client";
import type { BookDetail, ChapterBrief, ChapterDetail } from "../../api/types";
import { TierProvider } from "../../lib/tier";
import { READER_KEYS } from "./readerSettings";

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

/** 哪几章取数失败（或一直加载中）：由用例按需打开。 */
const failures = vi.hoisted(() => ({ ids: new Set<number>(), pending: new Set<number>() }));
const reloads = vi.hoisted(() => ({ count: 0 }));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { ...actual.api, readingProgress: vi.fn(async () => ({ items: [] })) } };
});

vi.mock("../../api/hooks", () => ({
  useBook: () => ({ data: book, loading: false, error: null, reload: vi.fn() }),
  useChapter: (_bookId: number, chapterId: number | null) => {
    if (chapterId === null) return { data: null, loading: false, error: null, reload: vi.fn() };
    if (failures.pending.has(chapterId)) return { data: null, loading: true, error: null, reload: vi.fn() };
    if (failures.ids.has(chapterId)) {
      const error = { code: "internal", message: "内部错误", status: 500 } as unknown as ApiError;
      return {
        data: null,
        loading: false,
        error,
        reload: () => {
          reloads.count += 1;
        },
      };
    }
    return { data: chapterOf(chapterId), loading: false, error: null, reload: vi.fn() };
  },
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

function renderReader(entries: string[] = ["/book/7", "/read/7/12"]) {
  return render(
    <TierProvider value="h5">
      <MemoryRouter initialEntries={entries} initialIndex={entries.length - 1}>
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

/** 底栏收起态是 inert + aria-hidden（读屏不可达），用例按 aria-label 直接取节点点它。 */
function clickBarButton(label: string) {
  const button = document.querySelector<HTMLButtonElement>(`[aria-label="${label}"]`);
  if (!button) throw new Error(`底栏里找不到「${label}」`);
  button.click();
}

function segmentTop(chapterId: number): number {
  return document.querySelector<HTMLElement>(`[data-chapter-id="${chapterId}"]`)!.getBoundingClientRect().top;
}

const SEGMENT_HEIGHT = 1000;
const VIEW_HEIGHT = 800;

/** 可测版面：每章高 1000、可滚区固定 8 屏；section 的屏幕位置随 scrollTop 上移。 */
function stubLayout() {
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

/** 把正文容器滚到某个位置并触发一次真实 scroll 事件（越界监听靠它）。 */
async function scrollTo(top: number) {
  await act(async () => {
    const area = surface();
    area.scrollTop = top;
    area.dispatchEvent(new Event("scroll"));
    await new Promise((resolve) => window.requestAnimationFrame(() => resolve(null)));
    await new Promise((resolve) => window.requestAnimationFrame(() => resolve(null)));
  });
}

beforeEach(() => {
  // 这些用例守的是 H5 的连续滚动：显式把阅读模式钉在 scroll（默认已是分页）
  localStorage.setItem(READER_KEYS.pageMode, "scroll");
  FakeObserver.reset();
  failures.ids.clear();
  failures.pending.clear();
  reloads.count = 0;
  vi.clearAllMocks();
  vi.stubGlobal("IntersectionObserver", FakeObserver);
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  stubLayout();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("连续阅读：越界接续", () => {
  it("滚到底：下一章正文已经在下面接着排，可见章不跳、不回到顶部", async () => {
    renderReader(["/read/7/12"]);
    // 进章窗口只有 11/12/13：13 是窗口尾部
    expect(screen.getByTestId("reader-segment-13")).toBeTruthy();
    expect(screen.queryByTestId("reader-segment-14")).toBeNull();

    // 距底不足一屏 ── 窗口尾部再补一章，读者继续下滑就有东西读
    await scrollTo(SEGMENT_HEIGHT * 8 - VIEW_HEIGHT);
    await waitFor(() => expect(screen.getByTestId("reader-segment-14")).toBeTruthy());
    // 只是扩了窗口：可见章与地址都没变
    expect(path()).toBe("/read/7/12");
    expect(screen.getByTestId("reader-segment-12")).toBeTruthy();
  });

  it("接着往下读：滚过章界线换成下一章，正文位置连续（不回顶部、不重新加载）", async () => {
    renderReader(["/read/7/12"]);
    await scrollTo(SEGMENT_HEIGHT * 8 - VIEW_HEIGHT);
    await waitFor(() => expect(screen.getByTestId("reader-segment-14")).toBeTruthy());
    const before = segmentTop(13);

    act(() => FakeObserver.emit(13));
    await waitFor(() => expect(path()).toBe("/read/7/13"));
    // 下一章早就渲染好了：换章只发生一次判定，没有加载态、没有位置跳变
    expect(screen.queryByTestId("reader-loading")).toBeNull();
    // 眼前那一行没动：新可见章的屏幕位置与判定前完全一致（上方摘掉的一章由补偿抵消）
    expect(segmentTop(13)).toBeCloseTo(before, 0);
    expect(segmentTop(12)).toBeCloseTo(0, 0);
    // 上一章仍在上面，向上回看有内容
    expect(screen.getByTestId("reader-segment-12")).toBeTruthy();
  });

  it("滚到顶：把上一章补在头上，读者停在上一章的末尾而不是章首", async () => {
    renderReader(["/read/7/12"]);
    // 进章窗口 11/12/13，再往上就没有 10
    expect(screen.queryByTestId("reader-segment-10")).toBeNull();
    const before = segmentTop(12);

    await scrollTo(0);
    await waitFor(() => expect(screen.getByTestId("reader-segment-10")).toBeTruthy());
    // 上面插进来一整章：高度补进 scrollTop，眼前的正文一动不动
    expect(surface().scrollTop).toBeGreaterThan(0);
    expect(segmentTop(12)).toBeCloseTo(before, 0);
    // 读者此刻看到的是 11 的末尾（10 的正文在更上面）
    expect(segmentTop(10)).toBeLessThan(0);
    expect(path()).toBe("/read/7/12");
  });

  it("滚到顶后接着退回：可见章换成上一章，落点是上一章内容而不是整页顶部", async () => {
    renderReader(["/read/7/12"]);
    await scrollTo(0);
    await waitFor(() => expect(screen.getByTestId("reader-segment-10")).toBeTruthy());
    act(() => FakeObserver.emit(11));
    await waitFor(() => expect(path()).toBe("/read/7/11"));
    expect(screen.getByTestId("reader-segment-10")).toBeTruthy();
    expect(surface().scrollTop).toBeGreaterThan(0);
  });

  it("连续下滑到底再滑回来：一路换章不回顶，回滑时停在上一章的末尾", async () => {
    renderReader(["/read/7/12"]);
    // 一路往下：每一步都还在可滚范围内，走到尾章之前每次滚到底都只扩窗口
    for (const top of [2000, 4000, 6000]) {
      await scrollTo(top);
      act(() => FakeObserver.emit(12));
    }
    expect(path()).toBe("/read/7/12");
    // 一路往下并越过界线
    await scrollTo(SEGMENT_HEIGHT * 8 - VIEW_HEIGHT);
    act(() => FakeObserver.emit(13));
    await waitFor(() => expect(path()).toBe("/read/7/13"));
    // 掉头往上：越过上半段界线，回到上一章内容，不回整页顶部
    await scrollTo(200);
    act(() => FakeObserver.emit(12));
    await waitFor(() => expect(path()).toBe("/read/7/12"));
    expect(surface().scrollTop).toBeGreaterThan(0);
  });

  it("中途的正常滚动不触发任何章节切换", async () => {
    renderReader(["/read/7/12"]);
    await scrollTo(SEGMENT_HEIGHT * 3);
    act(() => FakeObserver.emit(12));
    await scrollTo(SEGMENT_HEIGHT * 2);
    act(() => FakeObserver.emit(12));
    expect(path()).toBe("/read/7/12");
  });
});

describe("连续阅读：翻章按钮真的加载章节", () => {
  it("点「下一章」：换成下一章并把它对齐到正文区顶", async () => {
    renderReader(["/read/7/12"]);
    act(() => {
      clickBarButton("下一章");
    });
    await waitFor(() => expect(path()).toBe("/read/7/13"));
    // 落章首：章首正好在正文区顶（连续流里 scrollTop = 0 停的是上一章的开头）
    await waitFor(() => expect(segmentTop(13)).toBeCloseTo(0, 0));
    expect(screen.getByTestId("reader-segment-13").textContent).toContain("第4章的第一段。");
  });

  it("点「上一章」：换成上一章并把它对齐到正文区顶", async () => {
    renderReader(["/read/7/12"]);
    act(() => {
      clickBarButton("上一章");
    });
    await waitFor(() => expect(path()).toBe("/read/7/11"));
    await waitFor(() => expect(segmentTop(11)).toBeCloseTo(0, 0));
  });

  it("连续点「下一章」两次：每次都是真的换章，不会被判定带判回去", async () => {
    renderReader(["/read/7/12"]);
    act(() => {
      clickBarButton("下一章");
    });
    await waitFor(() => expect(path()).toBe("/read/7/13"));
    // 判定带此刻还压着旧位置：不许把可见章判回上一章
    act(() => FakeObserver.emit(12));
    expect(path()).toBe("/read/7/13");
    act(() => {
      clickBarButton("下一章");
    });
    await waitFor(() => expect(path()).toBe("/read/7/14"));
    await waitFor(() => expect(segmentTop(14)).toBeCloseTo(0, 0));
  });
});

describe("连续阅读：下一章打不开", () => {
  it("窗口尾部取数失败：正文底部给可见提示与重试，点了真的重取", async () => {
    failures.ids.add(14);
    renderReader(["/read/7/12"]);
    await scrollTo(SEGMENT_HEIGHT * 8 - VIEW_HEIGHT);
    const tail = await screen.findByTestId("reader-stream-tail-error");
    expect(tail.textContent).toContain("下一章没能打开");
    fireEvent.click(screen.getByRole("button", { name: "重新读取下一章" }));
    expect(reloads.count).toBe(1);
  });

  it("翻到打不开的章：给出可见的错误态与重试，不留在旧章不动", async () => {
    failures.ids.add(14);
    renderReader(["/read/7/13"]);
    act(() => {
      clickBarButton("下一章");
    });
    await waitFor(() => expect(path()).toBe("/read/7/14"));
    const panel = await screen.findByTestId("reader-error");
    expect(panel.textContent).toContain("正文没能打开");
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(reloads.count).toBe(1);
  });

  it("尾部还在加载：给一行加载提示，正文位置不动", async () => {
    failures.pending.add(14);
    renderReader(["/read/7/12"]);
    const before = segmentTop(12);
    await scrollTo(SEGMENT_HEIGHT * 8 - VIEW_HEIGHT);
    expect(await screen.findByTestId("reader-stream-tail-loading")).toBeTruthy();
    expect(segmentTop(12)).toBeCloseTo(before, 0);
  });
});

describe("连续阅读：冷却期补判的定时器", () => {
  // 回归守卫：曾经漏了卸载清理，冷却期内离开阅读器后定时器仍在跑，回调读 window
  // 抛 ReferenceError。vitest 把它算成 "Unhandled Errors" —— 1481 条用例全绿、
  // 进程退出码却是 1（实测 6 次里 1 次），门禁 11 因此偶发假红。
  //
  // 断言口径：先抓住「补判定时器」的真实 id，再断言卸载时**那个 id** 被清掉。
  // 不能只断言「clearTimeout 被调用过」——其它组件（转场、滚动补偿）也会清自己的
  // 定时器，那样的断言在修复被移除后依然会绿（实测过）。
  // 反向验证：临时删掉 useChapterStream 里的卸载清理，本用例必须红
  // （AssertionError: expected [...] to include Timeout{...}），恢复即绿。
  it("卸载时清掉冷却期补判挂上的那个定时器", async () => {
    const setSpy = vi.spyOn(window, "setTimeout");
    const clearSpy = vi.spyOn(window, "clearTimeout");
    const { unmount } = renderReader(["/read/7/12"]);

    // 换一次章 → 进入冷却期；冷却期内再判一次 → judge 挂上补判定时器。
    await scrollTo(SEGMENT_HEIGHT * 8 - VIEW_HEIGHT);
    await waitFor(() => expect(screen.getByTestId("reader-segment-14")).toBeTruthy());
    act(() => FakeObserver.emit(13));
    await waitFor(() => expect(path()).toBe("/read/7/13"));

    const beforeJudge = setSpy.mock.results.length;
    act(() => FakeObserver.emit(13));
    expect(setSpy.mock.results.length).toBeGreaterThan(beforeJudge);
    // 补判定时器的 id = 冷却期那次 setTimeout 的返回值（judge 里只挂这一个）。
    const judgeTimerId = setSpy.mock.results[setSpy.mock.results.length - 1]?.value;

    unmount();

    const cleared = clearSpy.mock.calls.map((call) => call[0]);
    expect(cleared).toContain(judgeTimerId);
    setSpy.mockRestore();
    clearSpy.mockRestore();
  });
});
