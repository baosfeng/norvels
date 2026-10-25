import { useSyncExternalStore } from "react";

/**
 * 极简全局 toast 的状态层：跨路由的操作结果（例如「已删除《书名》」）需要在跳页之后仍然可见，
 * 所以状态放在模块级 store，由 AppShell 里的 ToastHost 单点渲染，不发请求、不加依赖。
 */

export type ToastTone = "info" | "error";

export interface ToastMessage {
  id: number;
  text: string;
  tone: ToastTone;
}

/** 自动消失时间：足够读完一句话，又不至于挡住下一页的内容。 */
export const TOAST_MS = 4000;

let current: ToastMessage | null = null;
let seq = 0;
const listeners = new Set<() => void>();

function emit(): void {
  for (const listener of listeners) listener();
}

export function showToast(text: string, tone: ToastTone = "info"): void {
  if (!text) return;
  seq += 1;
  current = { id: seq, text, tone };
  emit();
}

export function clearToast(): void {
  if (!current) return;
  current = null;
  emit();
}

/** 只清掉指定那条：期间又弹了新提示就让它继续显示。 */
export function clearToastIf(id: number): void {
  if (current?.id === id) clearToast();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** 读当前 toast；没有时返 null。 */
export function useToast(): ToastMessage | null {
  return useSyncExternalStore(subscribe, () => current);
}
