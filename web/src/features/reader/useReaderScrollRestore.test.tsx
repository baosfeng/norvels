import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useReaderScrollRestore } from "./useReaderScrollRestore";

/** 正文容器：H5 下 .reader-main 自己滚（定高 overflow），不是窗口滚。 */
function mountSurface(scrollTop: number) {
  document.body.innerHTML = "";
  const surface = document.createElement("div");
  surface.dataset.testid = "reader-swipe-area";
  surface.scrollTop = scrollTop;
  document.body.appendChild(surface);
  for (let i = 0; i < 6; i += 1) {
    const p = document.createElement("p");
    p.id = `para-${i}`;
    p.textContent = `第 ${i} 段`;
    // jsdom 不做布局，scrollIntoView 也不存在：这里只记调用，用来区分两条恢复路径
    p.scrollIntoView = vi.fn();
    surface.appendChild(p);
  }
  return surface;
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 5));

function mount(props: {
  chapterId: number;
  chapterDataId?: number;
  storedParaIndex?: number;
  flipped?: boolean;
  pendingJump?: { chapterId: number; paraIndex: number } | null;
}) {
  const onRestoreAnchor = vi.fn();
  const consumeFlip = vi.fn(() => props.flipped === true);
  const hook = renderHook(() =>
    useReaderScrollRestore({
      searchOpen: false,
      currentHit: -1,
      hitCount: 0,
      chapterId: props.chapterId,
      chapterDataId: props.chapterDataId ?? props.chapterId,
      pendingJump: props.pendingJump ?? null,
      storedParaIndex: props.storedParaIndex ?? 0,
      progressLoading: false,
      consumeFlip,
      onRestoreAnchor,
    }),
  );
  return { ...hook, onRestoreAnchor, consumeFlip };
}

afterEach(() => {
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

describe("useReaderScrollRestore 回章首", () => {
  /**
   * 防复发：实测在章内滚到 70%（scrollTop=780）后左滑翻章，新章停在 56%——
   * 因为旧实现调的是 window.scrollTo({top:0})，而正文在 .reader-main 容器内滚动。
   */
  it("翻章后滚到容器的章首（scrollTop 0），不是停在上一章的位置", async () => {
    const surface = mountSurface(780);
    mount({ chapterId: 1041, storedParaIndex: 3, flipped: true });
    await flush();
    expect(surface.scrollTop).toBe(0);
    expect(window.scrollY).toBe(0);
  });

  it("翻章时不按段锚点恢复（读者要从章首接着读）", async () => {
    mountSurface(780);
    const { onRestoreAnchor } = mount({ chapterId: 1041, storedParaIndex: 3, flipped: true });
    await flush();
    expect(onRestoreAnchor).not.toHaveBeenCalled();
  });

  it("目录点回 / 浏览器后退（不是翻章）仍按段锚点恢复，不被丢回章首", async () => {
    const surface = mountSurface(780);
    const { onRestoreAnchor } = mount({ chapterId: 1040, storedParaIndex: 3, flipped: false });
    await flush();
    expect(onRestoreAnchor).toHaveBeenCalledWith({ chapterId: 1040, para: 3 });
    expect(surface.scrollTop).toBe(780);
    const node = document.getElementById("para-3") as HTMLElement;
    expect(node.scrollIntoView).toHaveBeenCalled();
  });

  it("书签跳转（pendingJump）优先，既不当翻章也不重复恢复段落", async () => {
    const surface = mountSurface(300);
    const { onRestoreAnchor } = mount({
      chapterId: 1040,
      storedParaIndex: 2,
      flipped: true,
      pendingJump: { chapterId: 1040, paraIndex: 4 },
    });
    await flush();
    expect(surface.scrollTop).toBe(300);
    expect(onRestoreAnchor).not.toHaveBeenCalled();
    expect((document.getElementById("para-4") as HTMLElement).scrollIntoView).toHaveBeenCalled();
  });

  it("同一章只处理一次：重渲染不会再把读者滚回去", async () => {
    const surface = mountSurface(0);
    const { rerender, onRestoreAnchor } = mount({ chapterId: 1040, storedParaIndex: 2, flipped: false });
    await flush();
    surface.scrollTop = 420;
    rerender();
    await flush();
    expect(surface.scrollTop).toBe(420);
    expect(onRestoreAnchor).toHaveBeenCalledTimes(1);
  });
});
