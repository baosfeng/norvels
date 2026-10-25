import { describe, expect, it, vi } from "vitest";
import {
  MAX_FRAME_MS,
  MIN_STEP_PX,
  SPEED_MAX,
  SPEED_MIN,
  SPEED_STEPS,
  clampFrameMs,
  createAutoScroll,
  frameDeltaPx,
  isAtBottom,
  normalizeSpeed,
  speedToPxPerSec,
} from "./autoScroll";

/** 手摇 rAF：把帧回调存起来，用例自己决定什么时候、按什么时间戳推进。 */
function manualFrames() {
  let nextHandle = 1;
  const queued = new Map<number, (time: number) => void>();
  return {
    requestFrame: (callback: (time: number) => void) => {
      const handle = nextHandle++;
      queued.set(handle, callback);
      return handle;
    },
    cancelFrame: (handle: number) => void queued.delete(handle),
    /** 推进一帧：把当前挂着的回调按时间戳跑掉。 */
    tick(time: number) {
      const callbacks = [...queued.values()];
      queued.clear();
      for (const callback of callbacks) callback(time);
    },
    pending: () => queued.size,
  };
}

describe("自动滚动：速度档", () => {
  it("档位是 0.5 步进的 0.5–5（共 10 档），档位越高滚得越快", () => {
    expect(SPEED_MIN).toBe(0.5);
    expect(SPEED_MAX).toBe(5);
    expect(SPEED_STEPS).toEqual([0.5, 1, 1.5, 2, 2.5, 3, 3.5, 4, 4.5, 5]);
    const rates = SPEED_STEPS.map((step) => speedToPxPerSec(step));
    for (let i = 1; i < rates.length; i += 1) expect(rates[i]).toBeGreaterThan(rates[i - 1]);
    // 线性口径：1 档 14 px/s
    expect(speedToPxPerSec(1)).toBe(14);
    expect(speedToPxPerSec(0.5)).toBe(7);
    expect(speedToPxPerSec(5)).toBe(70);
  });

  it("任意速度吸附到 0.5 的整数倍，越界夹到 0.5–5，非数字回落 3 档", () => {
    expect(normalizeSpeed(0)).toBe(0.5);
    expect(normalizeSpeed(9)).toBe(5);
    expect(normalizeSpeed(2.6)).toBe(2.5);
    expect(normalizeSpeed(0.74)).toBe(0.5);
    expect(normalizeSpeed(Number.NaN)).toBe(3);
  });
});

describe("自动滚动：帧推进", () => {
  it("单帧位移 = 速度 × 时间，超长帧按 250ms 截断", () => {
    expect(frameDeltaPx(3, 100)).toBeCloseTo(speedToPxPerSec(3) * 0.1, 5);
    expect(clampFrameMs(5000)).toBe(MAX_FRAME_MS);
    expect(clampFrameMs(-10)).toBe(0);
    expect(frameDeltaPx(3, 5000)).toBeCloseTo((speedToPxPerSec(3) * MAX_FRAME_MS) / 1000, 5);
  });

  it("把时间累加成位移：连续 10 帧 100ms 约等于 1 秒的量", () => {
    let total = 0;
    for (let i = 0; i < 10; i += 1) total += frameDeltaPx(4, 100);
    expect(total).toBeCloseTo(speedToPxPerSec(4), 5);
  });

  it("isAtBottom 留了 2px 余量", () => {
    expect(isAtBottom({ scrollTop: 900, clientHeight: 100, scrollHeight: 1000 })).toBe(true);
    expect(isAtBottom({ scrollTop: 898, clientHeight: 100, scrollHeight: 1000 })).toBe(true);
    expect(isAtBottom({ scrollTop: 897, clientHeight: 100, scrollHeight: 1000 })).toBe(false);
  });
});

describe("自动滚动：控制器", () => {
  it("start 后每帧滚动，stop 后不再滚动也不留挂起帧", () => {
    const frames = manualFrames();
    const scrollBy = vi.fn();
    const controller = createAutoScroll({ ...frames, scrollBy, atBottom: () => false });
    controller.start(5);
    expect(controller.running()).toBe(true);

    frames.tick(performance.now() + 100);
    expect(scrollBy).toHaveBeenCalledTimes(1);
    expect(scrollBy.mock.calls[0][0]).toBeGreaterThan(0);

    controller.stop();
    expect(controller.running()).toBe(false);
    expect(frames.pending()).toBe(0);
    frames.tick(performance.now() + 300);
    expect(scrollBy).toHaveBeenCalledTimes(1);
  });

  it("小于半像素不调 scrollBy（避免每帧抖动）", () => {
    const frames = manualFrames();
    const scrollBy = vi.fn();
    const controller = createAutoScroll({ ...frames, scrollBy, atBottom: () => false }, 1);
    controller.start();
    frames.tick(performance.now() + 1); // 1 档 1ms 只有 0.012px
    expect(scrollBy).not.toHaveBeenCalled();
    expect(1).toBeLessThan(MIN_STEP_PX * 100);
  });

  it("到底时停住并回调一次（不重复回调）", () => {
    const frames = manualFrames();
    let bottom = false;
    const onBottom = vi.fn(() => {
      bottom = true;
    });
    let scrollYs = 0;
    const controller = createAutoScroll({
      ...frames,
      scrollBy: (delta) => {
        scrollYs += delta;
      },
      atBottom: () => scrollYs > 30 || bottom,
      onBottom,
    });
    controller.start(5);

    const base = performance.now();
    for (let i = 1; i <= 20 && controller.running(); i += 1) frames.tick(base + i * 100);

    expect(onBottom).toHaveBeenCalledTimes(1);
    expect(controller.running()).toBe(false);
    expect(frames.pending()).toBe(0);
  });

  it("切换速度立即生效；重复 start 不重复排帧", () => {
    const frames = manualFrames();
    const scrollBy = vi.fn();
    const controller = createAutoScroll({ ...frames, scrollBy, atBottom: () => false }, 1);
    controller.start();
    controller.start(4);
    expect(controller.speed()).toBe(4);
    expect(frames.pending()).toBe(1);
  });
});
