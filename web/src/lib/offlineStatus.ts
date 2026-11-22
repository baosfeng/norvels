import { NEED_SECURE_CONTEXT_HINT } from "./capabilities";

/**
 * 离线徽标只在**真的能离线读**时才敢说「离线阅读」。
 *
 * 判据不是 navigator.onLine，而是 navigator.serviceWorker.controller：有控制器说明
 * 已注册的 Service Worker 正接管这个页面，Cache Storage 里才有断网可用的内容。
 * 非安全上下文（http://192.168.x.x）里浏览器直接拒绝注册，此时断网就是打不开——
 * 承诺「离线阅读」等于说假话，所以落回 offline-unavailable 并说明原因。
 *
 * 纯函数：在线状态与控制器有无都由调用方传入，三种状态可单测。
 */
export type OfflineState = "online" | "offline-ready" | "offline-unavailable";

/** 离线不可用时的说明，与能力面板同一句口径。 */
export const OFFLINE_UNAVAILABLE_HINT = `离线不可用 · ${NEED_SECURE_CONTEXT_HINT}`;

/** 徽标文案；只有 offline-ready 才有可说的事，在线时不占位置（返回 null）。 */
export function offlineBadgeText(state: OfflineState): string | null {
  if (state === "online") return null;
  return state === "offline-ready" ? "离线阅读" : OFFLINE_UNAVAILABLE_HINT;
}

export function offlineState(online: boolean, hasController: boolean): OfflineState {
  if (online) return "online";
  return hasController ? "offline-ready" : "offline-unavailable";
}
