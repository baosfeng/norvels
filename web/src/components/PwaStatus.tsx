import { useEffect, useState } from "react";

interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed"; platform: string }>;
}

/** 避免在 SSR/jsdom 下访问 navigator */
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

/** 全局 PWA 状态：离线提示 + 安装提示。放在路由之外，所有页面（含阅读器）可见。 */
export function PwaStatus() {
  const online = useOnlineStatus();
  const [deferred, setDeferred] = useState<BeforeInstallPromptEvent | null>(null);
  const [installed, setInstalled] = useState<boolean>(() => {
    try {
      return window.matchMedia?.("(display-mode: standalone)").matches ?? false;
    } catch {
      return false;
    }
  });

  useEffect(() => {
    const onBeforeInstall = (event: Event) => {
      event.preventDefault();
      setDeferred(event as BeforeInstallPromptEvent);
    };
    const onInstalled = () => {
      setDeferred(null);
      setInstalled(true);
    };
    window.addEventListener("beforeinstallprompt", onBeforeInstall);
    window.addEventListener("appinstalled", onInstalled);
    return () => {
      window.removeEventListener("beforeinstallprompt", onBeforeInstall);
      window.removeEventListener("appinstalled", onInstalled);
    };
  }, []);

  // 安装入口收进设置/菜单（原型 A），避免页面上再出现悬浮安装按钮
  const showInstall = false;

  if (online && !showInstall) return null;

  return (
    <div className="pwa-floating" aria-live="polite">
      {!online ? <span className="pwa-offline-badge">离线阅读</span> : null}
      {showInstall ? (
        <button
          type="button"
          className="pwa-install-btn"
          onClick={() => {
            deferred?.prompt().catch(() => undefined);
            deferred?.userChoice.finally(() => setDeferred(null));
          }}
        >
          安装应用
        </button>
      ) : null}
    </div>
  );
}
