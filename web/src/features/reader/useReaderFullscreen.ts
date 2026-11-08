import { useCallback, useEffect, useState } from "react";
import { showToast } from "../../lib/toastStore";

/**
 * 全屏阅读：优先用浏览器 Fullscreen API；不支持或被环境拒绝时，
 * 降级为“页面内全屏”——把阅读器铺满视口、收起控制栏，保证按钮总能进入一种全屏状态。
 */
export function useReaderFullscreen({ onUnsupported }: { onUnsupported: () => void }) {
  const canApi = typeof document !== "undefined" && document.fullscreenEnabled === true;
  const [pseudo, setPseudo] = useState(false);
  const [browserFullscreen, setBrowserFullscreen] = useState(false);

  useEffect(() => {
    const onChange = () => setBrowserFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener("fullscreenchange", onChange);
    return () => document.removeEventListener("fullscreenchange", onChange);
  }, []);

  const fullscreen = browserFullscreen || pseudo;

  const toggle = useCallback(async () => {
    if (browserFullscreen || pseudo) {
      setPseudo(false);
      if (document.fullscreenElement) {
        try {
          await document.exitFullscreen();
        } catch {
          // 已进入页面内全屏即可
        }
      }
      return;
    }
    if (canApi) {
      try {
        await document.documentElement.requestFullscreen();
        showToast("已进入全屏阅读");
        return;
      } catch {
        // fall through to page-level fullscreen
      }
    }
    setPseudo(true);
    onUnsupported();
    showToast(canApi ? "浏览器拒绝了全屏，已切换为页面内全屏" : "当前浏览器不支持全屏，已切换为页面内全屏");
  }, [browserFullscreen, pseudo, canApi, onUnsupported]);

  return { fullscreen, canFullscreen: true, toggle };
}
