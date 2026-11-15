import { afterEach, describe, expect, it, vi } from "vitest";
import { registerServiceWorker } from "./pwa";

/** 注册走 window 的 load 事件 + navigator.serviceWorker.register，两个都换成替身。 */

interface FakeRegister {
  register: ReturnType<typeof vi.fn>;
}

function installServiceWorker(register: FakeRegister["register"]): FakeRegister {
  const fake = { register };
  Object.defineProperty(window.navigator, "serviceWorker", { value: fake, configurable: true });
  return fake;
}

function removeServiceWorker(): void {
  Reflect.deleteProperty(window.navigator, "serviceWorker");
}

/** 触发 load（registerServiceWorker 是在 load 之后才注册的）。 */
function fireLoad(): void {
  window.dispatchEvent(new Event("load"));
}

afterEach(() => {
  removeServiceWorker();
  vi.restoreAllMocks();
});

describe("registerServiceWorker", () => {
  it("没有 serviceWorker 能力时什么都不做，也不监听 load", () => {
    removeServiceWorker();
    const addSpy = vi.spyOn(window, "addEventListener");
    registerServiceWorker();
    fireLoad();
    expect(addSpy).not.toHaveBeenCalledWith("load", expect.any(Function));
  });

  it("serviceWorker.register 不是函数时同样跳过", () => {
    Object.defineProperty(window.navigator, "serviceWorker", { value: {}, configurable: true });
    const addSpy = vi.spyOn(window, "addEventListener");
    registerServiceWorker();
    expect(addSpy).not.toHaveBeenCalledWith("load", expect.any(Function));
  });

  it("页面 load 之后按根作用域注册 /sw.js", async () => {
    const register = vi.fn(async () => undefined);
    installServiceWorker(register);

    registerServiceWorker();
    expect(register).not.toHaveBeenCalled();

    fireLoad();
    expect(register).toHaveBeenCalledWith("/sw.js", { scope: "/" });
    await Promise.resolve();
  });

  it("注册失败只打一条告警，不抛给调用方", async () => {
    const register = vi.fn(async () => {
      throw new Error("insecure context");
    });
    installServiceWorker(register);
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});

    registerServiceWorker();
    fireLoad();
    await Promise.resolve();
    await Promise.resolve();

    expect(warn).toHaveBeenCalledWith("[pwa] service worker register failed", expect.any(Error));
  });
});
