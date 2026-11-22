import { useCallback, useEffect, useState } from "react";
import { showToast } from "../../lib/toastStore";

/**
 * 全屏阅读：优先用浏览器 Fullscreen API；不支持或被环境拒绝时，
 * 降级为“页面内全屏”——把阅读器铺满视口，保证按钮总能进入一种全屏状态。
 *
 * 两条路径都调 onEnter：全屏的意义是正文铺满视口，控制栏留着既不算真全屏，
 * 还会让外壳按展开态让出一条 125px 的底部空白带。
 */
export function useReaderFullscreen({ onEnter }: { onEnter: () => void }) {
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
        onEnter();
        showToast("已进入全屏阅读");
        return;
      } catch {
        // fall through to page-level fullscreen
      }
    }
    setPseudo(true);
    onEnter();
    showToast(canApi ? "浏览器拒绝了全屏，已切换为页面内全屏" : "当前浏览器不支持全屏，已切换为页面内全屏");
  }, [browserFullscreen, pseudo, canApi, onEnter]);

  return { fullscreen, canFullscreen: true, toggle };
}
