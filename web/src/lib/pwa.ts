/**
 * PWA：注册 Service Worker。
 * 开发期（Vite HMR）与测试环境（jsdom）下静默失败，不影响主流程。
 */
export function registerServiceWorker(): void {
  if (typeof window === "undefined") return;
  if (!("serviceWorker" in navigator)) return;
  // 避免 vitest/jsdom 中 navigator.serviceWorker 为不可用对象时注册。
  if (typeof navigator.serviceWorker?.register !== "function") return;
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js", { scope: "/" }).catch((err) => {
      // eslint-disable-next-line no-console
      console.warn("[pwa] service worker register failed", err);
    });
  });
}
