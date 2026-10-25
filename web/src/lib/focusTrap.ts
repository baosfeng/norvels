import { useCallback, useEffect, useRef } from "react";
import type { RefObject } from "react";

/**
 * 抽屉 / 弹层的焦点陷阱。
 *
 * 纯逻辑部分（选择器、Tab 目标下标、栈）与 DOM 部分分开，前者可单测；
 * 后者由 hook 负责：打开时把焦点送进面板、Tab / Shift+Tab 在面板内循环、
 * Esc 交给调用方关闭、关闭后把焦点还给触发元素（无障碍要求「哪里来回哪里去」）。
 */

/** 面板内可聚焦元素：按文档顺序。排除 disabled / tabindex=-1 / aria-hidden 子树。 */
export const FOCUSABLE_SELECTOR = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  "[tabindex]:not([tabindex='-1'])",
].join(",");

export function focusableElements(root: HTMLElement | null): HTMLElement[] {
  if (!root) return [];
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (node) => node.getAttribute("aria-hidden") !== "true" && !node.closest("[aria-hidden='true']"),
  );
}

/**
 * Tab 键的目标下标：到头折返。
 * current < 0 表示焦点还不在面板里：向后从第一个开始，向前从最后一个开始。
 */
export function tabTargetIndex(count: number, current: number, shift: boolean): number {
  if (count <= 0) return -1;
  if (current < 0) return shift ? count - 1 : 0;
  return (current + (shift ? -1 : 1) + count) % count;
}

/** 当前打开的陷阱栈：多个面板同时开时只有最上面那个接管 Tab，避免焦点互抢。 */
const trapStack: HTMLElement[] = [];

export function activeTrapCount(): number {
  return trapStack.length;
}

export interface FocusTrapOptions {
  /** Esc 按下：调用方负责真正关闭面板（陷阱本身不改 UI 状态）。 */
  onEscape?: () => void;
  /**
   * 关闭后把焦点还给谁；默认还给打开前的 document.activeElement。
   * 抽屉由按钮触发，默认值就是那个按钮。
   */
  restoreTo?: HTMLElement | null;
}

/**
 * 在一个已挂载的 DOM 节点上启用焦点陷阱，返回拆除函数。
 * 打开时先把焦点移到面板内第一个可聚焦元素（没有就聚焦面板本身，需自备 tabindex）。
 */
export function activateFocusTrap(root: HTMLElement, options: FocusTrapOptions = {}): () => void {
  const previous =
    options.restoreTo ?? (typeof document === "undefined" ? null : (document.activeElement as HTMLElement | null));
  const first = focusableElements(root)[0];
  if (first) first.focus();
  else {
    root.tabIndex = -1;
    root.focus();
  }
  trapStack.push(root);

  const onKeyDown = (event: KeyboardEvent) => {
    // 只有最上层的陷阱处理按键：多面板叠开时不互相抢焦点
    if (trapStack[trapStack.length - 1] !== root) return;
    if (event.key === "Escape") {
      if (!options.onEscape) return;
      event.preventDefault();
      options.onEscape();
      return;
    }
    if (event.key !== "Tab") return;
    const list = focusableElements(root);
    if (list.length === 0) {
      event.preventDefault();
      return;
    }
    const current = list.indexOf(document.activeElement as HTMLElement);
    const next = tabTargetIndex(list.length, current, event.shiftKey);
    event.preventDefault();
    list[next]?.focus();
  };

  document.addEventListener("keydown", onKeyDown, true);
  return () => {
    document.removeEventListener("keydown", onKeyDown, true);
    const at = trapStack.lastIndexOf(root);
    if (at >= 0) trapStack.splice(at, 1);
    if (previous && previous.isConnected && typeof previous.focus === "function") previous.focus();
  };
}

/**
 * React 包装：open 变 true 时挂陷阱、变 false 时拆掉并还焦点。
 * onEscape 走 ref，避免每次渲染都重挂陷阱（重挂会重置焦点位置）。
 */
export function useFocusTrap(
  open: boolean,
  ref: RefObject<HTMLElement | null>,
  onEscape?: () => void,
): { restoreFocus: () => void } {
  const escape = useRef(onEscape);
  useEffect(() => {
    escape.current = onEscape;
  }, [onEscape]);
  const restore = useRef<(() => void) | null>(null);
  const restoreFocus = useCallback(() => {
    restore.current?.();
    restore.current = null;
  }, []);

  useEffect(() => {
    if (!open) return;
    const root = ref.current;
    if (!root) return;
    const detach = activateFocusTrap(root, { onEscape: () => escape.current?.() });
    restore.current = detach;
    return detach;
  }, [open, ref]);

  return { restoreFocus };
}
