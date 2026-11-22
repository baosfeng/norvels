import { afterEach, describe, expect, it, vi } from "vitest";
import { ensurePersistentStorage } from "./storagePersist";

/**
 * navigator.storage 在 jsdom 里是 undefined（用例自己换成替身）。
 * 这里只断「调用方拿到什么」，不断浏览器内部的启发式——那部分不可测也不该测。
 */
interface FakeStorage {
  persisted?: ReturnType<typeof vi.fn>;
  persist?: ReturnType<typeof vi.fn>;
}

function installStorage(storage: FakeStorage | undefined): void {
  Object.defineProperty(window.navigator, "storage", { value: storage, configurable: true });
}

afterEach(() => {
  Reflect.deleteProperty(window.navigator, "storage");
  vi.restoreAllMocks();
});

describe("ensurePersistentStorage（离线缓存别被浏览器清掉）", () => {
  it("没有 navigator.storage.persist 时静默返回 false，不抛", async () => {
    installStorage(undefined);
    await expect(ensurePersistentStorage()).resolves.toBe(false);
  });

  it("只有 persisted 没有 persist 也算没有入口：不调用、不抛", async () => {
    const persisted = vi.fn(async () => true);
    installStorage({ persisted });
    await expect(ensurePersistentStorage()).resolves.toBe(false);
    expect(persisted).not.toHaveBeenCalled();
  });

  it("已经是持久态：直接返回 true，不重复申请", async () => {
    const persist = vi.fn(async () => true);
    const persisted = vi.fn(async () => true);
    installStorage({ persist, persisted });

    await expect(ensurePersistentStorage()).resolves.toBe(true);
    expect(persisted).toHaveBeenCalledTimes(1);
    expect(persist).not.toHaveBeenCalled();
  });

  it("还没持久化且申请成功：返回 true", async () => {
    const persist = vi.fn(async () => true);
    const persisted = vi.fn(async () => false);
    installStorage({ persist, persisted });

    await expect(ensurePersistentStorage()).resolves.toBe(true);
    expect(persist).toHaveBeenCalledTimes(1);
  });

  it("申请被拒（浏览器不授予，Safari 常见）：返回 false，不抛", async () => {
    const persist = vi.fn(async () => false);
    installStorage({ persist, persisted: vi.fn(async () => false) });

    await expect(ensurePersistentStorage()).resolves.toBe(false);
  });

  it("查询或申请抛错：当成没拿到，返回 false", async () => {
    const persist = vi.fn(async () => {
      throw new Error("storage disabled");
    });
    installStorage({ persist, persisted: vi.fn(async () => false) });
    await expect(ensurePersistentStorage()).resolves.toBe(false);

    const throwingQuery = vi.fn(async () => {
      throw new Error("blocked by policy");
    });
    installStorage({ persist: vi.fn(async () => true), persisted: throwingQuery });
    await expect(ensurePersistentStorage()).resolves.toBe(false);
  });

  it("没有 persisted 方法（老浏览器）时跳过查询直接申请", async () => {
    const persist = vi.fn(async () => true);
    installStorage({ persist });

    await expect(ensurePersistentStorage()).resolves.toBe(true);
    expect(persist).toHaveBeenCalledTimes(1);
  });
});
