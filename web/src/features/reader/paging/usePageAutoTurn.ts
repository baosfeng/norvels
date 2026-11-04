import { useEffect } from "react";
import { pageHoldMs } from "../../../lib/paging/turnEffects";

/**
 * 自动翻页：分页模式下 settings.autoScroll 的含义从「自动滚」变成「自动翻」。
 *
 * 每页停留 pageHoldMs(速度档) 毫秒后翻一页；到章末由引擎回调 onEdge 决定是否继续下一章
 * （沿用 autoScrollNext 的口径）。面板打开 / 窗口失焦时暂停——与自动滚动同一套条件，
 * 读者回来时不会发现「自己翻走了好几页」。
 */
export function usePageAutoTurn(options: {
  enabled: boolean;
  paused: boolean;
  speed: number;
  turn: () => void;
}): void {
  const { enabled, paused, speed, turn } = options;

  useEffect(() => {
    if (!enabled || paused) return;
    const timer = window.setInterval(turn, Math.max(500, pageHoldMs(speed)));
    return () => window.clearInterval(timer);
  }, [enabled, paused, speed, turn]);
}
