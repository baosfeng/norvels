import { afterEach, describe, expect, it, vi } from "vitest";
import { scrollToChapterTop, type Surface } from "./readerStreamLayout";

/**
 * 显式翻章的落位：目标章章首必须对到正文可视区顶。
 *
 * 两种滚动模式各有一套基准，混用会让落位过冲——
 * 实测 PC 上拿「正文容器自己的 rect.top」当基准（容器是整条流，随时跟着滚），
 * 算出来的是章在流里的偏移，再累加到 scrollY 上，翻一章多滚 4500px，
 * 落位停在目标章下方几屏处，判定带随后把可见章判成更后面那一章。
 */
function rectOf(top: number, height = 100): DOMRect {
  return { top, height, bottom: top + height, left: 0, right: 0, width: 0, x: 0, y: top, toJSON: () => ({}) } as DOMRect;
}

function mountChapter(id: number, top: number): HTMLElement {
  const node = document.createElement("div");
  node.dataset.chapterId = String(id);
  node.getBoundingClientRect = () => rectOf(top, 4000);
  document.body.append(node);
  return node;
}

function surfaceOf(scrolls: boolean, top = 0): Surface & { el: HTMLElement & { scrollTop: number } } {
  const el = document.createElement("div");
  el.getBoundingClientRect = () => rectOf(top, 900);
  el.scrollTop = 0;
  return { el: el as HTMLElement & { scrollTop: number }, scrolls };
}

afterEach(() => {
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

describe("翻章落位：目标章章首对到正文可视区顶", () => {
  it("整页滚（PC）：基准是视口顶，不拿会跟着滚的正文容器当基准", () => {
    mountChapter(9, 200);
    const surface = surfaceOf(false, -800);
    const scrollBy = vi.spyOn(window, "scrollBy").mockImplementation(() => {});

    expect(scrollToChapterTop(surface, 9)).toBe(true);
    // 200（相对视口），不是 200 - (-800) = 1000
    expect(scrollBy).toHaveBeenCalledWith(0, 200);
  });

  it("容器自己滚（H5）：基准是容器可视区顶", () => {
    mountChapter(9, 300);
    const surface = surfaceOf(true, 61);
    surface.el.scrollTop = 100;
    const scrollBy = vi.spyOn(window, "scrollBy").mockImplementation(() => {});

    expect(scrollToChapterTop(surface, 9)).toBe(true);
    expect(surface.el.scrollTop).toBe(339);
    expect(scrollBy).not.toHaveBeenCalled();
  });

  it("已经对齐在顶部就不动；目标章还没渲染返回 false（下个提交再试）", () => {
    mountChapter(9, 0);
    const scrollBy = vi.spyOn(window, "scrollBy").mockImplementation(() => {});
    expect(scrollToChapterTop(surfaceOf(false), 9)).toBe(true);
    expect(scrollBy).not.toHaveBeenCalled();

    expect(scrollToChapterTop(surfaceOf(false), 404)).toBe(false);
  });
});
