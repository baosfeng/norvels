import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useReaderFullscreen } from "./useReaderFullscreen";

/**
 * 全屏：优先浏览器 Fullscreen API，被拒或不支持时降级为页面内全屏。
 * jsdom 没有真实全屏，这里把 documentElement.requestFullscreen / exitFullscreen / fullscreenEnabled 换成替身。
 */

type FsDoc = Document & {
  fullscreenEnabled: boolean;
  fullscreenElement: Element | null;
  exitFullscreen: () => Promise<void>;
  documentElement: HTMLElement & { requestFullscreen: () => Promise<void> };
};

const doc = document as FsDoc;
const original = {
  fullscreenEnabled: Object.getOwnPropertyDescriptor(doc, "fullscreenEnabled"),
  fullscreenElement: Object.getOwnPropertyDescriptor(doc, "fullscreenElement"),
  requestFullscreen: doc.documentElement.requestFullscreen,
  exitFullscreen: doc.exitFullscreen,
};

function setFullscreenApi(options: { enabled: boolean; requestFails?: boolean }): void {
  Object.defineProperty(doc, "fullscreenEnabled", { value: options.enabled, configurable: true });
  Object.defineProperty(doc, "fullscreenElement", { value: null, configurable: true });
  doc.documentElement.requestFullscreen = vi.fn(async () => {
    if (options.requestFails) throw new Error("denied by policy");
    Object.defineProperty(doc, "fullscreenElement", { value: doc.documentElement, configurable: true });
    document.dispatchEvent(new Event("fullscreenchange"));
  });
  doc.exitFullscreen = vi.fn(async () => {
    Object.defineProperty(doc, "fullscreenElement", { value: null, configurable: true });
    document.dispatchEvent(new Event("fullscreenchange"));
  });
}

beforeEach(() => {
  setFullscreenApi({ enabled: true });
});

afterEach(() => {
  if (original.fullscreenEnabled) Object.defineProperty(doc, "fullscreenEnabled", original.fullscreenEnabled);
  if (original.fullscreenElement) Object.defineProperty(doc, "fullscreenElement", original.fullscreenElement);
  doc.documentElement.requestFullscreen = original.requestFullscreen;
  doc.exitFullscreen = original.exitFullscreen;
  vi.restoreAllMocks();
});

describe("useReaderFullscreen", () => {
  it("支持 Fullscreen API 时进入浏览器全屏，退出后回到普通态", async () => {
    const onUnsupported = vi.fn();
    const hook = renderHook(() => useReaderFullscreen({ onUnsupported }));
    expect(hook.result.current.fullscreen).toBe(false);
    expect(hook.result.current.canFullscreen).toBe(true);

    await act(async () => {
      await hook.result.current.toggle();
    });
    expect(doc.documentElement.requestFullscreen).toHaveBeenCalled();
    expect(hook.result.current.fullscreen).toBe(true);

    await act(async () => {
      await hook.result.current.toggle();
    });
    expect(doc.exitFullscreen).toHaveBeenCalled();
    expect(hook.result.current.fullscreen).toBe(false);
    expect(onUnsupported).not.toHaveBeenCalled();
  });

  it("浏览器拒绝全屏时降级为页面内全屏并通知调用方", async () => {
    setFullscreenApi({ enabled: true, requestFails: true });
    const onUnsupported = vi.fn();
    const hook = renderHook(() => useReaderFullscreen({ onUnsupported }));

    await act(async () => {
      await hook.result.current.toggle();
    });
    expect(hook.result.current.fullscreen).toBe(true);
    expect(onUnsupported).toHaveBeenCalledTimes(1);
  });

  it("环境不支持 Fullscreen API 时直接进入页面内全屏", async () => {
    setFullscreenApi({ enabled: false });
    const onUnsupported = vi.fn();
    const hook = renderHook(() => useReaderFullscreen({ onUnsupported }));

    await act(async () => {
      await hook.result.current.toggle();
    });
    expect(doc.documentElement.requestFullscreen).not.toHaveBeenCalled();
    expect(hook.result.current.fullscreen).toBe(true);
    expect(onUnsupported).toHaveBeenCalledTimes(1);

    // 再点一次退出页面内全屏
    await act(async () => {
      await hook.result.current.toggle();
    });
    expect(hook.result.current.fullscreen).toBe(false);
  });

  it("外部（Esc 键）退出浏览器全屏时状态跟着回来", async () => {
    const hook = renderHook(() => useReaderFullscreen({ onUnsupported: () => undefined }));
    await act(async () => {
      await hook.result.current.toggle();
    });
    expect(hook.result.current.fullscreen).toBe(true);

    await act(async () => {
      Object.defineProperty(doc, "fullscreenElement", { value: null, configurable: true });
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    expect(hook.result.current.fullscreen).toBe(false);
  });
});
