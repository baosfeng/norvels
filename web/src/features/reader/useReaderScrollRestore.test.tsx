import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { clearPagedPara, rememberPagedPara, useReaderScrollRestore } from "./useReaderScrollRestore";

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

/** 可重渲染的读法：重渲染时切开关，复刻「读者在设置面板切阅读模式」。 */
function mountReader(props: {
  chapterId: number;
  chapterDataId?: number;
  storedParaIndex?: number;
  flipped?: boolean;
  /** 这次换章是连续流滚过章界线出来的。 */
  streamed?: boolean;
  pendingJump?: { chapterId: number; paraIndex: number } | null;
  /** 关掉 = 分页模式（滚动恢复整条路径让位）。 */
  enabled?: boolean;
}) {
  const onRestoreAnchor = vi.fn();
  const consumeFlip = vi.fn(() => props.flipped === true);
  const consumeAdvance = vi.fn(() => props.streamed === true);
  // 无参 rerender()（旧用例的写法）保持初始化时的开关状态
  const defaultEnabled = props.enabled ?? true;
  const hook = renderHook(
    (next: { enabled: boolean } = { enabled: defaultEnabled }) =>
      useReaderScrollRestore({
        enabled: next.enabled,
        searchOpen: false,
        currentHit: -1,
        hitCount: 0,
        chapterId: props.chapterId,
        chapterDataId: props.chapterDataId ?? props.chapterId,
        pendingJump: props.pendingJump ?? null,
        storedParaIndex: props.storedParaIndex ?? 0,
        progressLoading: false,
        consumeFlip,
        consumeAdvance,
        onRestoreAnchor,
      }),
    { initialProps: { enabled: props.enabled ?? true } },
  );
  return { ...hook, onRestoreAnchor, consumeFlip };
}

function mount(props: {
  chapterId: number;
  chapterDataId?: number;
  storedParaIndex?: number;
  flipped?: boolean;
  /** 这次换章是连续流滚过章界线出来的。 */
  streamed?: boolean;
  pendingJump?: { chapterId: number; paraIndex: number } | null;
}) {
  return mountReader(props);
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

  it("滚过章界线换到下一章：位置是读者滚出来的，既不回章首也不跳锚点", async () => {
    const surface = mountSurface(1200);
    const { onRestoreAnchor } = mount({
      chapterId: 1041,
      storedParaIndex: 3,
      streamed: true,
      pendingJump: null,
    });
    await flush();
    expect(surface.scrollTop).toBe(1200);
    expect(onRestoreAnchor).not.toHaveBeenCalled();
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
describe("U9 分页 ↔ 滚动切模式保位", () => {
  afterEach(() => {
    clearPagedPara();
  });

  it("从分页切回滚动：落到分页时那一页的首段，不是章首", async () => {
    mountSurface(0);
    // 分页里翻到第 11 页：这一页首段是第 4 段
    rememberPagedPara(1040, 4);
    const { rerender } = mountReader({ chapterId: 1040, storedParaIndex: 4, enabled: false });
    await flush();
    rerender({ enabled: true });
    await flush();
    expect((document.getElementById("para-4") as HTMLElement).scrollIntoView).toHaveBeenCalledWith({ block: "start" });
  });

  it("切回后写回进度锚点，进度条与下一次进章都对得上", async () => {
    mountSurface(0);
    rememberPagedPara(1040, 4);
    const { rerender, onRestoreAnchor } = mountReader({ chapterId: 1040, storedParaIndex: 0, enabled: false });
    await flush();
    rerender({ enabled: true });
    await flush();
    expect(onRestoreAnchor).toHaveBeenCalledWith({ chapterId: 1040, para: 4 });
  });

  it("分页锚点记的是别的章：不动当前位置，也不改写锚点", async () => {
    const surface = mountSurface(300);
    rememberPagedPara(9999, 5);
    const { rerender, onRestoreAnchor } = mountReader({ chapterId: 1040, storedParaIndex: 2, enabled: false });
    await flush();
    rerender({ enabled: true });
    await flush();
    expect(surface.scrollTop).toBe(300);
    expect(onRestoreAnchor).not.toHaveBeenCalled();
  });

  it("每个分页锚点只消费一次：之后的重渲染不会再滚", async () => {
    mountSurface(0);
    rememberPagedPara(1040, 4);
    const { rerender } = mountReader({ chapterId: 1040, storedParaIndex: 4, enabled: false });
    await flush();
    rerender({ enabled: true });
    await flush();
    const spy = (document.getElementById("para-4") as HTMLElement).scrollIntoView as unknown as ReturnType<typeof vi.fn>;
    const calls = spy.mock.calls.length;
    expect(calls).toBeGreaterThan(0);
    rerender({ enabled: true });
    await flush();
    expect((document.getElementById("para-4") as HTMLElement).scrollIntoView).toHaveBeenCalledTimes(calls);
  });

  it("从滚动切到分页那一下不动位置（位置由页码接手，不是回到章首）", async () => {
    const surface = mountSurface(780);
    const { rerender } = mountReader({ chapterId: 1040, storedParaIndex: 3, enabled: true });
    await flush();
    rerender({ enabled: false });
    await flush();
    expect(surface.scrollTop).toBe(780);
  });
});
