import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReaderScrollProgress } from "./ReaderScrollProgress";
import { PROGRESS_HIDE_MS, activePercent, scrollPercentOf } from "./useReaderScrollPercent";

/** 正文容器尺寸：可滚距离 = 1851 - 844 = 1007（与实测的 390×844 一致）。 */
const SCROLL_HEIGHT = 1851;
const CLIENT_HEIGHT = 844;

function mountSurface() {
  const surface = document.createElement("div");
  surface.dataset.testid = "reader-swipe-area";
  document.body.appendChild(surface);
  return surface;
}

/** 改写容器的滚动指标并派发 scroll：jsdom 不做布局，只能显式给数。 */
async function scrollTo(surface: HTMLElement, scrollTop: number) {
  Object.defineProperty(surface, "scrollHeight", { configurable: true, value: SCROLL_HEIGHT });
  Object.defineProperty(surface, "clientHeight", { configurable: true, value: CLIENT_HEIGHT });
  surface.scrollTop = scrollTop;
  await act(async () => {
    surface.dispatchEvent(new Event("scroll"));
  });
  await flushFrames();
}

/** 假定时器下 rAF 也是一条 16ms 的定时器：推过去让这一帧算完。 */
async function flushFrames(ms = 20) {
  await act(async () => {
    vi.advanceTimersByTime(ms);
  });
}

function barWidth() {
  return parseFloat(screen.getByTestId("reader-progress-bar").style.width || "0");
}

function chip() {
  return screen.getByTestId("reader-progress-chip");
}

describe("ReaderScrollProgress", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
    document.body.innerHTML = "";
  });

  it("滚到章中时细条宽度介于 0 与 100% 之间，浮层显示同一个比例", async () => {
    const surface = mountSurface();
    render(<ReaderScrollProgress resetKey={46} />);
    await scrollTo(surface, 500);

    const width = barWidth();
    expect(width).toBeGreaterThan(0);
    expect(width).toBeLessThan(100);
    expect(width).toBeCloseTo(49.65, 1);
    expect(chip().textContent).toBe("本章 50%");
    expect(chip().className).toContain("is-shown");
  });

  it("滚到章末时细条接近满格，比例封顶 100%", async () => {
    const surface = mountSurface();
    render(<ReaderScrollProgress resetKey={46} />);
    await scrollTo(surface, SCROLL_HEIGHT);

    expect(barWidth()).toBe(100);
    expect(chip().textContent).toBe("本章 100%");
    expect(screen.getByTestId("reader-progress-track").getAttribute("aria-valuenow")).toBe("100");
  });

  it("滚动停手 1.2s 后浮层淡出，但细条保持在本章比例上", async () => {
    const surface = mountSurface();
    render(<ReaderScrollProgress resetKey={46} />);
    await scrollTo(surface, 500);
    await flushFrames(PROGRESS_HIDE_MS - 200);
    expect(chip().className).toContain("is-shown");

    await flushFrames(300);
    expect(chip().className).not.toContain("is-shown");
    expect(barWidth()).toBeCloseTo(49.65, 1);
  });

  it("换章（resetKey 变化）后重新量一次，上一章的比例不会留在条上", async () => {
    const surface = mountSurface();
    const { rerender } = render(<ReaderScrollProgress resetKey={46} />);
    await scrollTo(surface, 800);
    expect(barWidth()).toBeGreaterThan(50);

    surface.scrollTop = 0;
    rerender(<ReaderScrollProgress resetKey={47} />);
    await flushFrames();
    expect(barWidth()).toBe(0);
  });

  it("PC 由整页滚（容器滚不动）时退回窗口口径", () => {
    const page = { scrollTop: 300, clientHeight: 800, scrollHeight: 1800 };
    // 容器不可滚（scrollHeight == clientHeight）→ 听窗口
    expect(activePercent({ scrollTop: 0, clientHeight: 800, scrollHeight: 800 }, page)).toBeCloseTo(0.3, 3);
    // 容器可滚 → 只听容器，窗口滚多少都不算
    expect(activePercent({ scrollTop: 100, clientHeight: 800, scrollHeight: 1800 }, page)).toBeCloseTo(0.1, 3);
  });

  it("内容不足一屏时按 0 处理，不显示成半截", () => {
    expect(scrollPercentOf({ scrollTop: 0, clientHeight: 844, scrollHeight: 844 })).toBe(0);
    expect(scrollPercentOf({ scrollTop: 40, clientHeight: 900, scrollHeight: 800 })).toBe(0);
    expect(scrollPercentOf({ scrollTop: 100, clientHeight: 844, scrollHeight: 1851 })).toBeCloseTo(0.0993, 3);
  });
});