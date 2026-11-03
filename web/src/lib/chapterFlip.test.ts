import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./theme", () => ({ prefersReducedMotion: vi.fn(() => false) }));

const { prefersReducedMotion } = await import("./theme");
const { canFlipWithSnapshot, settleChapterFlip, startChapterFlip } = await import("./chapterFlip");

/** 装上 / 摘掉同文档视图转场：jsdom 没有这个 API，浏览器里由真实实现负责。 */
function setViewTransition(impl: ((update: () => Promise<void>) => unknown) | null): void {
  if (impl) Object.defineProperty(document, "startViewTransition", { configurable: true, writable: true, value: impl });
  else Reflect.deleteProperty(document, "startViewTransition");
}

/** 假的转场：只关心「apply 什么时候跑、promise 什么时候放行、动画什么时候结束」。 */
function installViewTransition(): { updates: Promise<void>[]; finishes: (() => void)[]; skips: number[] } {
  const updates: Promise<void>[] = [];
  const finishes: (() => void)[] = [];
  const skips: number[] = [];
  setViewTransition((update) => {
    updates.push(update());
    const finished = new Promise<void>((resolve) => finishes.push(resolve));
    return { finished, skipTransition: () => skips.push(updates.length - 1) };
  });
  return { updates, finishes, skips };
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
    document.documentElement.style.removeProperty("--reader-flip-sign");
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
    // 方向走自定义属性：view-transition 伪元素只能继承它
    expect(document.documentElement.style.getPropertyValue("--reader-flip-sign")).toBe("1");
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

  it("落位时定住入场淡入：转场后不会再闪一次白", async () => {
    const { updates } = installViewTransition();
    const article = document.createElement("section");
    article.className = "reader-article is-entering";
    document.body.appendChild(article);
    startChapterFlip("next", vi.fn());
    await Promise.resolve();
    settleChapterFlip();
    await expect(updates[0]).resolves.toBeUndefined();
    expect(article.style.animation).toBe("none");
  });

  it("反向连点：被 skip 的旧转场不许删掉新转场的方向", async () => {
    const { finishes } = installViewTransition();
    startChapterFlip("next", vi.fn());
    await Promise.resolve();
    startChapterFlip("prev", vi.fn());
    await Promise.resolve();
    // 旧转场被浏览器 skip，它的 finished 先到——新转场的方向必须留着
    finishes[0]();
    await Promise.resolve();
    await Promise.resolve();
    expect(document.documentElement.dataset.readerFlip).toBe("prev");
  });

  it("连翻时先结束上一次转场，别让旧方向留在屏幕上", async () => {
    const { skips } = installViewTransition();
    startChapterFlip("next", vi.fn());
    await Promise.resolve();
    startChapterFlip("prev", vi.fn());
    await Promise.resolve();
    expect(skips).toHaveLength(1);
    expect(document.documentElement.dataset.readerFlip).toBe("prev");
  });

  it("转场结束后撤掉方向标记，下一次翻章重新写", async () => {
    const { updates, finishes } = installViewTransition();
    startChapterFlip("prev", vi.fn());
    await Promise.resolve();
    settleChapterFlip();
    await updates[0];
    expect(document.documentElement.dataset.readerFlip).toBe("prev");
    finishes[0]();
    await Promise.resolve();
    await Promise.resolve();
    expect(document.documentElement.dataset.readerFlip).toBeUndefined();
    expect(document.documentElement.style.getPropertyValue("--reader-flip-sign")).toBe("");
  });
});
