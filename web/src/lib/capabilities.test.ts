import { afterEach, describe, expect, it, vi } from "vitest";
import { browserCapabilityEnv, detectCapabilities } from "./capabilities";
import type { CapabilityEnv } from "./capabilities";

/**
 * 能力自检只回答一件事：此刻这个地址上哪几项真能用、不能用时该说哪句话。
 * 三种情形各走一遍——全可用 / 非安全上下文 / 属性缺失，最后守「判定不抛、不读全局」。
 * 清单只收界面里真有消费点的四项：安全上下文、屏幕常亮、离线缓存、持久化存储。
 */

const ALL_IDS = ["secureContext", "wakeLock", "serviceWorker", "storagePersist"] as const;
const PY_IDS = ["wakeLock", "serviceWorker", "storagePersist"] as const;

/** 安全上下文 + 四项入口齐全的理想环境。 */
function fullEnv(): CapabilityEnv {
  return {
    isSecureContext: true,
    wakeLock: { request: () => Promise.resolve() },
    serviceWorker: { register: () => Promise.resolve() },
    storage: { persist: () => Promise.resolve(true) },
  };
}

const byId = (env: CapabilityEnv) =>
  Object.fromEntries(detectCapabilities(env).map((item) => [item.id, item]));

afterEach(() => {
  Reflect.deleteProperty(window.navigator, "wakeLock");
  Reflect.deleteProperty(window.navigator, "serviceWorker");
  Reflect.deleteProperty(window.navigator, "storage");
  Reflect.deleteProperty(window, "isSecureContext");
});

describe("detectCapabilities：https / localhost 上全部可用", () => {
  it("四项齐全且都在安全上下文时逐项标记可用，不带任何提示", () => {
    const reports = detectCapabilities(fullEnv());
    expect(reports.map((item) => item.id)).toEqual([...ALL_IDS]);
    for (const item of reports) {
      expect(item.available).toBe(true);
      expect(item.hint).toBe("");
      expect(item.label.length).toBeGreaterThan(0);
    }
  });

  it("同样的 API 在 http://192.168.x.x 上：除安全上下文外全部不可用，提示指向地址", () => {
    const map = byId({ ...fullEnv(), isSecureContext: false });
    expect(map.secureContext.available).toBe(false);
    expect(map.secureContext.hint).toContain("https");
    for (const id of PY_IDS) {
      expect(map[id].available).toBe(false);
      // 根因是地址而不是浏览器：提示必须是「换个访问方式」，不能让人去换浏览器
      expect(map[id].hint).toContain("需 https 或 localhost 访问");
    }
  });
});

describe("detectCapabilities：安全上下文里但属性缺失（老内核 / 关闭的 API）", () => {
  it("属性整个不存在时按不可用算，提示归到浏览器", () => {
    const map = byId({ isSecureContext: true });
    expect(map.secureContext.available).toBe(true);
    for (const id of PY_IDS) {
      expect(map[id].available).toBe(false);
      expect(map[id].hint).toBe("当前浏览器不支持");
    }
  });

  it("属性存在但不是函数（占位对象）同样算不可用", () => {
    const map = byId({ isSecureContext: true, wakeLock: {}, serviceWorker: {}, storage: {} });
    for (const id of PY_IDS) {
      expect(map[id].available).toBe(false);
    }
    // 安全上下文本身是布尔，缺省（undefined）不算可用
    expect(byId({}).secureContext.available).toBe(false);
  });

  it("只缺一项时不影响其它项（逐项独立判定）", () => {
    const env = fullEnv();
    delete env.serviceWorker;
    const map = byId(env);
    expect(map.serviceWorker.available).toBe(false);
    expect(map.wakeLock.available).toBe(true);
    expect(map.storagePersist.available).toBe(true);
  });
});

describe("browserCapabilityEnv：从真实 navigator 取值，不抛", () => {
  it("把 navigator 上的入口原样转成判定输入", () => {
    Object.defineProperty(window, "isSecureContext", { value: true, configurable: true });
    Object.defineProperty(window.navigator, "wakeLock", { value: fullEnv().wakeLock, configurable: true });

    const env = browserCapabilityEnv();
    expect(env.isSecureContext).toBe(true);
    expect(env.wakeLock).toBeDefined();
    expect(detectCapabilities(env).filter((item) => item.available).map((item) => item.id)).toEqual([
      "secureContext",
      "wakeLock",
    ]);
  });

  it("没有 window（服务端渲染 / 纯逻辑环境）时返回空环境，不抛", () => {
    vi.stubGlobal("window", undefined);
    try {
      expect(browserCapabilityEnv()).toEqual({});
      expect(detectCapabilities(browserCapabilityEnv()).every((item) => !item.available)).toBe(true);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("属性全缺失时仍然返回对象（判定退化成四项不可用）", () => {
    Object.defineProperty(window, "isSecureContext", { value: false, configurable: true });
    const reports = detectCapabilities(browserCapabilityEnv());
    expect(reports).toHaveLength(4);
    expect(reports.every((item) => !item.available)).toBe(true);
  });
});
