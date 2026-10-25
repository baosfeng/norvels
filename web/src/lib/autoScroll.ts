/**
 * 自动滚动的纯逻辑：速度档 → px/s、按帧推进、到底回调。
 *
 * DOM 与计时都从参数注入，所以这段能在 jsdom 里逐帧驱动、不需要真实 rAF，
 * 也不会因为测试里的假定时器而卡死。组件只负责给 scrollBy / atBottom 与速度。
 */

/** 速度档：0.5–5，步进 0.5（慢档适合中文长文精读，快档适合扫读）。 */
export const SPEED_MIN = 0.5;
export const SPEED_MAX = 5;
export const SPEED_STEP = 0.5;

/** 每档之间的换算系数：速度 1 档 = 14 px/s（0.5 档 7 px/s，5 档 70 px/s）。 */
export const PX_PER_SPEED_UNIT = 14;

/** 可选档位（0.5 / 1 / 1.5 … 5），面板按它步进。 */
export const SPEED_STEPS: number[] = Array.from({ length: 10 }, (_, index) => Number(((index + 1) * SPEED_STEP).toFixed(1)));

/** 单帧最多算多久：切回标签页 / 系统休眠后别把积攒的时间一次性滚完。 */
export const MAX_FRAME_MS = 250;

/** 一帧最多挪多少像素（低于半像素不挪，避免每帧都调用 scrollBy 造成抖动）。 */
export const MIN_STEP_PX = 0.5;

/** 把任意速度吸附到 0.5 的整数倍并夹到 0.5–5（非数字回落 3 档）。 */
export function normalizeSpeed(speed: number): number {
  if (!Number.isFinite(speed)) return 3;
  const snapped = Math.round(speed / SPEED_STEP) * SPEED_STEP;
  return Math.min(SPEED_MAX, Math.max(SPEED_MIN, Number(snapped.toFixed(1))));
}

/** 速度档 → 每秒像素（线性：1 档 14 px/s）。 */
export function speedToPxPerSec(speed: number): number {
  return Math.round(normalizeSpeed(speed) * PX_PER_SPEED_UNIT);
}

/** 帧间隔夹取：负数按 0 处理，过长按 MAX_FRAME_MS 截断。 */
export function clampFrameMs(elapsedMs: number): number {
  if (!Number.isFinite(elapsedMs) || elapsedMs <= 0) return 0;
  return Math.min(MAX_FRAME_MS, elapsedMs);
}

/** 本帧要挪的像素（浮点，调用方自己决定舍入）。 */
export function frameDeltaPx(speed: number, elapsedMs: number): number {
  return (speedToPxPerSec(speed) * clampFrameMs(elapsedMs)) / 1000;
}

export interface ScrollMetrics {
  scrollTop: number;
  clientHeight: number;
  scrollHeight: number;
}

/** 是否已经滚到底（留 tolerance 像素余量，避免亚像素误差导致永远差一点）。 */
export function isAtBottom(metrics: ScrollMetrics, tolerance = 2): boolean {
  return metrics.scrollTop + metrics.clientHeight >= metrics.scrollHeight - tolerance;
}

export interface AutoScrollDeps {
  /** 滚动一屏的增量（px，正数向下）。 */
  scrollBy(deltaY: number): void;
  /** 当前是否已在底部。 */
  atBottom(): boolean;
  /** 滚到底时回调一次（是否翻章由调用方决定）。 */
  onBottom?: () => void;
  requestFrame?(callback: (time: number) => void): number;
  cancelFrame?(handle: number): void;
}

export interface AutoScrollController {
  start(speed?: number): void;
  stop(): void;
  running(): boolean;
  setSpeed(speed: number): void;
  speed(): number;
}

function defaultRequestFrame(callback: (time: number) => void): number {
  if (typeof requestAnimationFrame === "function") return requestAnimationFrame(callback);
  return setTimeout(() => callback(Date.now()), 16) as unknown as number;
}

function defaultCancelFrame(handle: number): void {
  if (typeof cancelAnimationFrame === "function") {
    cancelAnimationFrame(handle);
    return;
  }
  clearTimeout(handle as unknown as ReturnType<typeof setTimeout>);
}

/**
 * 起一个按帧推进的自动滚动：start 后每帧滚一点，stopped / 到底即停。
 * 同一实例可以反复 start / stop（面板开关直接映射到它）。
 */
export function createAutoScroll(deps: AutoScrollDeps, initialSpeed = 3): AutoScrollController {
  const requestFrame = deps.requestFrame ?? defaultRequestFrame;
  const cancelFrame = deps.cancelFrame ?? defaultCancelFrame;
  let speed = normalizeSpeed(initialSpeed);
  let handle: number | null = null;
  let last = 0;
  let pending = 0;
  let active = false;

  const schedule = () => {
    if (!active || handle !== null) return;
    handle = requestFrame(frame);
  };

  function frame(time: number) {
    handle = null;
    if (!active) return;
    pending += frameDeltaPx(speed, time - last);
    last = time;
    if (pending >= MIN_STEP_PX) {
      const delta = pending;
      pending = 0;
      deps.scrollBy(delta);
      // 到底就停：翻不翻章由 onBottom 的调用方决定，这里不做导航
      if (deps.atBottom()) {
        active = false;
        deps.onBottom?.();
        return;
      }
    }
    schedule();
  }

  return {
    start(nextSpeed?: number) {
      if (nextSpeed !== undefined) speed = normalizeSpeed(nextSpeed);
      if (active) return;
      active = true;
      pending = 0;
      last = typeof performance !== "undefined" ? performance.now() : Date.now();
      schedule();
    },
    stop() {
      active = false;
      pending = 0;
      if (handle !== null) {
        cancelFrame(handle);
        handle = null;
      }
    },
    running: () => active,
    setSpeed(nextSpeed: number) {
      speed = normalizeSpeed(nextSpeed);
    },
    speed: () => speed,
  };
}
