import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { releaseScreenWakeLock } from "../../lib/wakeLock";
import { useReaderWakeLock } from "./useReaderWakeLock";

/**
 * 阅读器的屏幕常亮：挂载申请、卸载释放、回到前台重新申请。
 * jsdom 没有 Screen Wake Lock，这里替身整个 navigator.wakeLock，
 * 「不支持」那条用来锁住「静默跳过、不影响阅读」这个前提。
 */

type FakeSentinel = WakeLockSentinel & {
  releasedFlag: boolean;
  releasedBySystem: () => void;
  isReleased: () => boolean;
};

function fakeSentinel(): FakeSentinel {
  const listeners = new Set<() => void>();
  const sentinel = {
    releasedFlag: false,
    get released() {
      return sentinel.releasedFlag;
    },
    type: "screen" as const,
    release: vi.fn(async () => {
      sentinel.releasedFlag = true;
      for (const listener of listeners) listener();
    }),
    /** 模拟系统自己收回（切后台 / 电量策略），不经过我们的 release */
    releasedBySystem: () => {
      sentinel.releasedFlag = true;
      for (const listener of listeners) listener();
    },
    addEventListener: vi.fn((_: string, handler: () => void) => void listeners.add(handler)),
    removeEventListener: vi.fn((_: string, handler: () => void) => void listeners.delete(handler)),
    dispatchEvent: vi.fn(),
    onrelease: null,
    isReleased: () => sentinel.releasedFlag,
  };
  return sentinel as unknown as FakeSentinel;
}

function stubWakeLock(request: () => Promise<WakeLockSentinel>) {
  const wakeLock = { request: vi.fn(request) } as unknown as WakeLock;
  Object.defineProperty(navigator, "wakeLock", { value: wakeLock, configurable: true });
  return wakeLock;
}

function setVisibility(state: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", { value: state, configurable: true });
  document.dispatchEvent(new Event("visibilitychange"));
}

beforeEach(async () => {
  Reflect.deleteProperty(navigator, "wakeLock");
  await releaseScreenWakeLock();
  setVisibility("visible");
});

afterEach(() => {
  Reflect.deleteProperty(navigator, "wakeLock");
  setVisibility("visible");
  vi.restoreAllMocks();
});

describe("useReaderWakeLock", () => {
  it("挂载即申请屏幕常亮", async () => {
    const sentinel = fakeSentinel();
    const api = stubWakeLock(async () => sentinel);
    renderHook(() => useReaderWakeLock());
    await waitFor(() => expect(api.request).toHaveBeenCalledTimes(1));
    expect(api.request).toHaveBeenCalledWith("screen");
  });

  it("卸载时释放，别让读者离开阅读器后屏幕还亮着", async () => {
    const sentinel = fakeSentinel();
    stubWakeLock(async () => sentinel);
    const hook = renderHook(() => useReaderWakeLock());
    await waitFor(() => expect(sentinel.release).not.toHaveBeenCalled());
    hook.unmount();
    await waitFor(() => expect(sentinel.release).toHaveBeenCalledTimes(1));
    expect(sentinel.isReleased()).toBe(true);
  });

  it("切后台再回到可见时重新申请（系统切后台会自动收走锁）", async () => {
    let current = fakeSentinel();
    const api = stubWakeLock(async () => current);
    renderHook(() => useReaderWakeLock());
    await waitFor(() => expect(api.request).toHaveBeenCalledTimes(1));

    // 系统在切后台时把锁收了，页面重新可见时不应该继续用这个已释放的 sentinel
    await act(async () => {
      setVisibility("hidden");
      current.releasedBySystem();
      current = fakeSentinel();
      setVisibility("visible");
    });
    await waitFor(() => expect(api.request).toHaveBeenCalledTimes(2));
  });

  it("同一个 sentinel 只挂一次 release 监听，不因可见性抖动重复挂", async () => {
    const sentinel = fakeSentinel();
    stubWakeLock(async () => sentinel);
    renderHook(() => useReaderWakeLock());
    await waitFor(() => expect(sentinel.addEventListener).toHaveBeenCalledTimes(1));

    await act(async () => {
      setVisibility("hidden");
      setVisibility("visible");
    });
    // 复用同一把仍然有效的锁：既不再申请，也不重复挂监听
    expect(sentinel.addEventListener).toHaveBeenCalledTimes(1);
  });

  it("浏览器不支持 Screen Wake Lock 时静默跳过，不抛也不影响渲染", async () => {
    Reflect.deleteProperty(navigator, "wakeLock");
    const hook = renderHook(() => useReaderWakeLock());
    await act(async () => {
      setVisibility("hidden");
      setVisibility("visible");
    });
    expect(hook.result.current).toBeUndefined();
  });

  it("enabled=false 时不申请（留给不需要常亮的场景）", async () => {
    const api = stubWakeLock(async () => fakeSentinel());
    renderHook(() => useReaderWakeLock(false));
    await act(async () => {
      setVisibility("visible");
    });
    expect(api.request).not.toHaveBeenCalled();
  });

  it("页面在后台时挂载：先不申请，等回到可见再发", async () => {
    setVisibility("hidden");
    const sentinel = fakeSentinel();
    const api = stubWakeLock(async () => sentinel);
    renderHook(() => useReaderWakeLock());
    await act(async () => {});
    expect(api.request).not.toHaveBeenCalled();
    await act(async () => {
      setVisibility("visible");
    });
    await waitFor(() => expect(api.request).toHaveBeenCalledTimes(1));
  });
});
