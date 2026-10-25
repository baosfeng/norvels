import { afterEach, describe, expect, it, vi } from "vitest";
import { activateFocusTrap, activeTrapCount, focusableElements, tabTargetIndex } from "./focusTrap";

/** 本次用例启用的陷阱：afterEach 统一拆除，避免上一个用例的栈漏到下一个。 */
const opened: Array<() => void> = [];

function trap(root: HTMLElement, options?: { onEscape?: () => void; restoreTo?: HTMLElement | null }) {
  const detach = activateFocusTrap(root, options);
  opened.push(detach);
  return detach;
}

/** 造一个带 n 个按钮的面板，并把它挂到 body 上（jsdom 里 focus 只对已挂载元素生效）。 */
function makePanel(labels: string[]): { root: HTMLElement; trigger: HTMLButtonElement } {
  const trigger = document.createElement("button");
  trigger.textContent = "打开";
  document.body.appendChild(trigger);
  const root = document.createElement("div");
  for (const label of labels) {
    const btn = document.createElement("button");
    btn.textContent = label;
    root.appendChild(btn);
  }
  document.body.appendChild(root);
  trigger.focus();
  return { root, trigger };
}

function press(key: string, shiftKey = false) {
  const event = new KeyboardEvent("keydown", { key, shiftKey, bubbles: true, cancelable: true });
  document.dispatchEvent(event);
  return event;
}

afterEach(() => {
  while (opened.length > 0) opened.pop()?.();
  document.body.innerHTML = "";
});

describe("焦点陷阱：纯逻辑", () => {
  it("Tab 到头折返，Shift+Tab 到头反折", () => {
    expect(tabTargetIndex(3, 0, false)).toBe(1);
    expect(tabTargetIndex(3, 2, false)).toBe(0);
    expect(tabTargetIndex(3, 0, true)).toBe(2);
    expect(tabTargetIndex(3, 1, true)).toBe(0);
  });

  it("焦点还不在面板里时：向后从第一个开始，向前从最后一个开始", () => {
    expect(tabTargetIndex(3, -1, false)).toBe(0);
    expect(tabTargetIndex(3, -1, true)).toBe(2);
  });

  it("面板里没有可聚焦元素时返回 -1", () => {
    expect(tabTargetIndex(0, -1, false)).toBe(-1);
  });

  it("跳过 disabled 与 aria-hidden 子树", () => {
    const root = document.createElement("div");
    root.innerHTML =
      '<button id="a">a</button><button id="b" disabled>b</button><div aria-hidden="true"><button id="c">c</button></div><a href="#x" id="d">d</a>';
    const ids = focusableElements(root).map((node) => node.id);
    expect(ids).toEqual(["a", "d"]);
  });
});

describe("焦点陷阱：Tab 循环 / Esc / 焦点归还", () => {
  it("打开即聚焦第一个可聚焦元素", () => {
    const { root } = makePanel(["一", "二", "三"]);
    trap(root);
    expect(document.activeElement?.textContent).toBe("一");
  });

  it("Tab 在面板内循环：最后一个回到第一个，Shift+Tab 反向", () => {
    const { root } = makePanel(["一", "二", "三"]);
    trap(root);
    const buttons = Array.from(root.querySelectorAll("button"));
    expect(document.activeElement).toBe(buttons[0]);

    press("Tab");
    expect(document.activeElement).toBe(buttons[1]);
    press("Tab");
    expect(document.activeElement).toBe(buttons[2]);
    press("Tab");
    expect(document.activeElement).toBe(buttons[0]);
    press("Tab", true);
    expect(document.activeElement).toBe(buttons[2]);
  });

  it("Esc 交给调用方关闭；没给 onEscape 时不拦截", () => {
    const first = makePanel(["一"]);
    const onEscape = vi.fn();
    const detach = trap(first.root, { onEscape });

    const event = press("Escape");
    expect(onEscape).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
    detach();

    const second = makePanel(["一"]);
    trap(second.root);
    expect(press("Escape").defaultPrevented).toBe(false);
  });

  it("拆除后焦点回到触发元素", () => {
    const { root, trigger } = makePanel(["一", "二"]);
    const detach = trap(root);
    expect(document.activeElement).not.toBe(trigger);

    detach();
    expect(document.activeElement).toBe(trigger);
  });

  it("多面板叠开时只有最上层接管 Tab", () => {
    const first = makePanel(["A"]);
    const second = makePanel(["B"]);
    trap(first.root);
    const detachSecond = trap(second.root);
    expect(activeTrapCount()).toBe(2);

    press("Tab");
    expect(second.root.contains(document.activeElement)).toBe(true);

    detachSecond();
    press("Tab");
    expect(first.root.contains(document.activeElement)).toBe(true);
    expect(activeTrapCount()).toBe(1);
  });
});
