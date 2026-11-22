import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ReaderHelpPanel } from "./ReaderHelpPanel";

/**
 * 帮助面板里的「本设备能力」区块：读者要能自己看清「这台设备、这个地址上哪几项能用」，
 * 而不是遇到「屏幕常亮没反应 / 离线打不开」再回来问人。
 * 这里只守渲染：两种环境各渲染一次，断言行数与不可用项的说明文案。
 */

function stubSecureContext(value: boolean): void {
  Object.defineProperty(window, "isSecureContext", { value, configurable: true });
}

/** 六项入口全有的替身（判定逻辑本身在 lib/capabilities.test.ts 里测）。 */
function stubAllApis(): void {
  const stub = () => Promise.resolve();
  Object.defineProperty(window.navigator, "wakeLock", { value: { request: stub }, configurable: true });
  Object.defineProperty(window.navigator, "serviceWorker", { value: { register: stub }, configurable: true });
  Object.defineProperty(window.navigator, "clipboard", { value: { writeText: stub }, configurable: true });
  Object.defineProperty(window.navigator, "share", { value: stub, configurable: true });
  Object.defineProperty(window.navigator, "storage", { value: { persist: () => Promise.resolve(true) }, configurable: true });
}

function clearApis(): void {
  for (const key of ["wakeLock", "serviceWorker", "clipboard", "share", "storage"] as const) {
    Reflect.deleteProperty(window.navigator, key);
  }
}

function openPanel() {
  render(<ReaderHelpPanel open onClose={() => undefined} />);
  return screen.getByTestId("reader-help-capabilities");
}

afterEach(() => {
  clearApis();
  Reflect.deleteProperty(window, "isSecureContext");
});

describe("帮助面板 · 本设备能力", () => {
  it("六项都在：安全上下文 / 屏幕常亮 / 离线缓存 / 剪贴板 / 分享 / 持久化存储", () => {
    stubSecureContext(true);
    stubAllApis();
    const block = openPanel();
    const labels = Array.from(block.querySelectorAll("dt")).map((node) => node.textContent);
    expect(labels).toEqual([
      "安全上下文",
      "屏幕常亮（Wake Lock）",
      "离线缓存（Service Worker）",
      "剪贴板",
      "分享",
      "持久化存储",
    ]);
  });

  it("全可用时每行写「可用」，不出现「不可用」", () => {
    stubSecureContext(true);
    stubAllApis();
    const block = openPanel();
    for (const id of ["secureContext", "wakeLock", "serviceWorker", "clipboard", "share", "storagePersist"]) {
      expect(within(block).getByTestId(`cap-${id}`).textContent).toContain("可用");
    }
    expect(block.textContent).not.toContain("不可用");
  });

  it("局域网 http（非安全上下文）渲染：每行「不可用」并说明需 https 或 localhost", () => {
    stubSecureContext(false);
    stubAllApis();
    const block = openPanel();
    expect(within(block).getByTestId("cap-secureContext").textContent).toContain("不可用");
    expect(within(block).getByTestId("cap-wakeLock").textContent).toContain("需 https 或 localhost 访问");
    expect(within(block).getByTestId("cap-serviceWorker").textContent).toContain("不可用");
  });

  it("安全上下文里 API 缺失：说「当前浏览器不支持」，不与地址问题混为一谈", () => {
    stubSecureContext(true);
    const block = openPanel();
    expect(within(block).getByTestId("cap-secureContext").textContent).toContain("可用");
    expect(within(block).getByTestId("cap-clipboard").textContent).toContain("当前浏览器不支持");
  });

  it("纯展示：区块里没有按钮，点不了也跳不走", () => {
    stubSecureContext(false);
    const block = openPanel();
    expect(within(block).queryAllByRole("button")).toHaveLength(0);
    expect(within(block).queryAllByRole("link")).toHaveLength(0);
  });

  it("打开面板不改变既有快捷键/手势内容（能力区块是追加，不是替换）", () => {
    stubSecureContext(true);
    render(<ReaderHelpPanel open onClose={() => undefined} />);
    const text = screen.getByTestId("reader-help").textContent ?? "";
    expect(text).toContain("触摸设备");
    expect(text).toContain("单击中间");
    expect(text).toContain("本设备能力");
  });
});
