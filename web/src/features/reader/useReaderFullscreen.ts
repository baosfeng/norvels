import { useCallback, useEffect, useState } from "react";
import { showToast } from "../../lib/toastStore";

/**
 * 全屏阅读：能力探测 + 状态同步 + 不支持时的降级（收起控制栏并说明原因）。
 * 浏览器不支持时按钮照样可点——让读者知道「点了，只是这浏览器做不到」。
 */
export function useReaderFullscreen({ onUnsupported }: { onUnsupported: () => void }) {
  const canFullscreen = typeof document !== "undefined" && document.fullscreenEnabled === true;
  const [fullscreen, setFullscreen] = useState(false);

  useEffect(() => {
    const onChange = () => setFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener("fullscreenchange", onChange);
    return () => document.removeEventListener("fullscreenchange", onChange);
  }, []);

  const toggle = useCallback(async () => {
    if (!canFullscreen) {
      onUnsupported();
      showToast("当前浏览器不支持全屏，已收起控制栏");
      return;
    }
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else {
        await document.documentElement.requestFullscreen();
        showToast("已进入全屏阅读");
      }
    } catch {
      onUnsupported();
      showToast("当前浏览器不支持全屏，已收起控制栏");
    }
  }, [canFullscreen, onUnsupported]);

  return { fullscreen, canFullscreen, toggle };
}
