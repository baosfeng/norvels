import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./theme", () => ({ prefersReducedMotion: vi.fn(() => false) }));

const { prefersReducedMotion } = await import("./theme");
const { canFlipWithSnapshot, settleChapterFlip, startChapterFlip } = await import("./chapterFlip");

/** 装上 / 摘掉同文档视图转场：jsdom 没有这个 API，浏览器里由真实实现负责。 */
function setViewTransition(impl: ((update: () => Promise<void>) => unknown) | null): void {
  if (impl) Object.defineProperty(document, "startViewTransition", { configurable: true, writable: true, value: impl });
  else Reflect.deleteProperty(document, "startViewTransition");
}

/** 假的转场：只关心「apply 什么时候跑、promise 什么时候放行」。 */
function installViewTransition(): { updates: Promise<void>[] } {
  const updates: Promise<void>[] = [];
  setViewTransition((update) => {
    updates.push(update());
    return { finished: Promise.resolve() };
  });
  return { updates };
}

/** 手势层：转场拍新快照前必须清掉跟手位移，否则新页歪着落定。 */
function swipeLayer(): HTMLElement {
  const layer = document.createElement("div");
  layer.className = "reader-swipe-layer";
  layer.style.transform = "translateX(-40px)";
  document.body.appendChild(layer);
  return layer;
}

describe("翻章转场", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    document.documentElement.removeAttribute("data-reader-flip");
    vi.mocked(prefersReducedMotion).mockReturnValue(false);
  });

  afterEach(() => {
    setViewTransition(null);
    vi.useRealTimers();
    document.documentElement.removeAttribute("data-reader-flip");
  });

  it("浏览器没有视图转场时直接换章：功能不受影响", () => {
    setViewTransition(null);
    expect(canFlipWithSnapshot()).toBe(false);
    const apply = vi.fn();
    startChapterFlip("next", apply);
    expect(apply).toHaveBeenCalledTimes(1);
  });

  it("系统要求减少动效时同样直接换章", () => {
    const { updates } = installViewTransition();
    vi.mocked(prefersReducedMotion).mockReturnValue(true);
    const apply = vi.fn();
    startChapterFlip("prev", apply);
    expect(apply).toHaveBeenCalledTimes(1);
    expect(updates).toHaveLength(0);
    expect(document.documentElement.dataset.readerFlip).toBeUndefined();
  });

  it("起转场：方向写进 data 属性、换章在 update 里跑、落位后放行", async () => {
    const { updates } = installViewTransition();
    const apply = vi.fn();
    startChapterFlip("next", apply);
    expect(document.documentElement.dataset.readerFlip).toBe("next");
    // update 由浏览器异步调用：这里手动放行一次微任务
    await Promise.resolve();
    expect(apply).toHaveBeenCalledTimes(1);
    expect(updates).toHaveLength(1);
    settleChapterFlip();
    await expect(updates[0]).resolves.toBeUndefined();
  });

  it("落位迟迟不上报时兜底放行，页面不会锁死在快照里", async () => {
    vi.useFakeTimers();
    const { updates } = installViewTransition();
    startChapterFlip("next", vi.fn());
    await Promise.resolve();
    expect(updates).toHaveLength(1);
    vi.advanceTimersByTime(600);
    await expect(updates[0]).resolves.toBeUndefined();
  });

  it("拍新快照前清掉手势层位移：新页从原位进场", async () => {
    const { updates } = installViewTransition();
    const layer = swipeLayer();
    startChapterFlip("next", vi.fn());
    await Promise.resolve();
    expect(layer.style.transform).toBe("");
    settleChapterFlip();
    await expect(updates[0]).resolves.toBeUndefined();
  });

  it("转场结束后撤掉方向标记，下一次翻章重新写", async () => {
    const { updates } = installViewTransition();
    startChapterFlip("prev", vi.fn());
    await Promise.resolve();
    settleChapterFlip();
    await updates[0];
    await Promise.resolve();
    expect(document.documentElement.dataset.readerFlip).toBeUndefined();
  });
});
