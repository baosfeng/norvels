import { useEffect, useState } from "react";
import { offlineBadgeText, offlineState } from "../lib/offlineStatus";

/**
 * 离线状态徽标：在线时不占位置；断网时只在 Service Worker 已接管（真有缓存）时说「离线阅读」，
 * 否则如实说「离线不可用 · 需 https 或 localhost 访问」——判定在 lib/offlineStatus.ts。
 */
function useOnlineStatus(): boolean {
  const [online, setOnline] = useState<boolean>(() => (typeof navigator === "undefined" ? true : navigator.onLine));
  useEffect(() => {
    const on = () => setOnline(true);
    const off = () => setOnline(false);
    window.addEventListener("online", on);
    window.addEventListener("offline", off);
    return () => {
      window.removeEventListener("online", on);
      window.removeEventListener("offline", off);
    };
  }, []);
  return online;
}

/** 已注册的 Service Worker 是否正接管本页（读一次即可，注册发生在 load，不监听变化）。 */
function hasServiceWorkerController(): boolean {
  return typeof navigator !== "undefined" && Boolean(navigator.serviceWorker?.controller);
}

export function PwaStatus() {
  const online = useOnlineStatus();
  const text = offlineBadgeText(offlineState(online, hasServiceWorkerController()));
  if (!text) return null;
  return <span className="pwa-offline-badge">{text}</span>;
}
