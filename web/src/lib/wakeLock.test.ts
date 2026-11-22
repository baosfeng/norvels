import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  currentScreenWakeLock,
  releaseScreenWakeLock,
  requestScreenWakeLock,
} from "./wakeLock";

/**
 * Screen Wake Lock 的薄封装：jsdom 没有这个 API，这里用替身把三条出口都走一遍
 * ——支持 / 不支持 / 被系统拒绝，断言的重点是「任何情况下都不抛」。
 */

type FakeSentinel = WakeLockSentinel & { releasedFlag: boolean };

function fakeSentinel(behavior: { failRelease?: boolean } = {}): FakeSentinel {
  const sentinel = {
    releasedFlag: false,
    get released() {
      return sentinel.releasedFlag;
    },
    type: "screen" as const,
    release: vi.fn(async () => {
      if (behavior.failRelease) throw new Error("release rejected");
      sentinel.releasedFlag = true;
    }),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
    onrelease: null,
  };
  return sentinel as unknown as FakeSentinel;
}

/** 把 navigator.wakeLock 换成可控替身；返回 request 桩便于断言调用次数。 */
function stubWakeLock(request: () => Promise<WakeLockSentinel>) {
  const wakeLock = { request: vi.fn(request) } as unknown as WakeLock;
  Object.defineProperty(navigator, "wakeLock", { value: wakeLock, configurable: true });
  return wakeLock;
}

/** navigator 上没有 wakeLock 属性（老浏览器）的情形。 */
function removeWakeLock() {
  Reflect.deleteProperty(navigator, "wakeLock");
}

// 模块内单例：每条用例开始前先把上一轮可能残留的锁清掉，用例之间不互相影响
beforeEach(async () => {
  removeWakeLock();
  await releaseScreenWakeLock();
});

afterEach(() => {
  removeWakeLock();
  vi.restoreAllMocks();
});

describe("requestScreenWakeLock", () => {
  it("支持时拿到 sentinel 并返回 true", async () => {
    const sentinel = fakeSentinel();
    stubWakeLock(async () => sentinel);
    await expect(requestScreenWakeLock()).resolves.toBe(true);
    expect(currentScreenWakeLock()).toBe(sentinel);
  });

  it("已经持锁时不再重复申请（重复申请在部分实现上会被直接拒绝）", async () => {
    const sentinel = fakeSentinel();
    const api = stubWakeLock(async () => sentinel);
    await requestScreenWakeLock();
    await expect(requestScreenWakeLock()).resolves.toBe(true);
    expect(api.request).toHaveBeenCalledTimes(1);
  });

  it("浏览器不支持 wakeLock 时静默返回 false，且不抛", async () => {
    removeWakeLock();
    await expect(requestScreenWakeLock()).resolves.toBe(false);
    expect(currentScreenWakeLock()).toBeNull();
  });

  it("系统拒绝（省电模式 / 非安全上下文）时静默返回 false", async () => {
    stubWakeLock(async () => {
      throw new Error("NotAllowedError");
    });
    await expect(requestScreenWakeLock()).resolves.toBe(false);
    expect(currentScreenWakeLock()).toBeNull();
  });

  it("request 返回 null 也算没拿到锁", async () => {
    stubWakeLock(async () => null as unknown as WakeLockSentinel);
    await expect(requestScreenWakeLock()).resolves.toBe(false);
    expect(currentScreenWakeLock()).toBeNull();
  });

  it("系统已收回的 sentinel 不算持锁，下次会重新申请", async () => {
    const first = fakeSentinel();
    const second = fakeSentinel();
    const api = stubWakeLock(async () => first);
    await requestScreenWakeLock();
    first.releasedFlag = true;
    expect(currentScreenWakeLock()).toBeNull();
    stubWakeLock(async () => second);
    await expect(requestScreenWakeLock()).resolves.toBe(true);
    expect(currentScreenWakeLock()).toBe(second);
    expect(api.request).toHaveBeenCalledTimes(1);
  });
});

describe("releaseScreenWakeLock", () => {
  it("释放当前锁并清空引用", async () => {
    const sentinel = fakeSentinel();
    stubWakeLock(async () => sentinel);
    await requestScreenWakeLock();
    await expect(releaseScreenWakeLock()).resolves.toBe(true);
    expect(sentinel.release).toHaveBeenCalledTimes(1);
    expect(currentScreenWakeLock()).toBeNull();
  });

  it("本来就没锁时也算成功（调用方只关心「现在不持锁」）", async () => {
    removeWakeLock();
    await expect(releaseScreenWakeLock()).resolves.toBe(true);
  });

  it("release 抛错时返回 false 而不是把异常抛给调用方", async () => {
    const sentinel = fakeSentinel({ failRelease: true });
    stubWakeLock(async () => sentinel);
    await requestScreenWakeLock();
    await expect(releaseScreenWakeLock()).resolves.toBe(false);
    // 引用照样清掉：不放走一个已经不可靠的 sentinel
    expect(currentScreenWakeLock()).toBeNull();
  });
});
