import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import { isIgnorableSwipeTarget } from "./swipe";

/** 控制层无操作自动收起的时长（ms）；需求口径为 3 秒。 */
export const AUTO_HIDE_MS = 3000;

/** 位移超过这个像素就算滑动/滚动，不当作「单击」。 */
const TAP_SLOP = 12;
/** 按住超过这个时长不算单击（长按多半是在选词）。 */
const TAP_MAX_MS = 700;

interface TapStart {
  x: number;
  y: number;
  t: number;
}

export interface ReaderControlsApi {
  /** 控制层当前是否显示：PC 恒为 true，H5 由单击切换。 */
  visible: boolean;
  toggle: () => void;
  reveal: () => void;
  /** 绑到正文区上的单击识别属性（滑动/点按钮/选字都不触发）。 */
  surfaceProps: {
    onPointerDown: (event: ReactPointerEvent) => void;
    onPointerUp: (event: ReactPointerEvent) => void;
    onPointerCancel: () => void;
  };
}

/**
 * 阅读器控制层显隐：H5 默认收起，单击正文唤出，3 秒无操作自动收起，
 * 再点立即收起；设置面板或目录打开（paused）期间暂停自动收起。
 * 只认「短按 + 位移很小」的一次点击，左右滑动翻章与选字不会被当成唤出。
 */
export function useReaderControls(options: { isH5: boolean; paused: boolean; autoHideMs?: number }): ReaderControlsApi {
  const { isH5, paused, autoHideMs = AUTO_HIDE_MS } = options;
  const [visible, setVisible] = useState(false);
  // 任何一次有效交互都重置自动收起计时
  const [activity, setActivity] = useState(0);
  const start = useRef<TapStart | null>(null);

  useEffect(() => {
    if (!isH5 || !visible || paused) return;
    const timer = window.setTimeout(() => setVisible(false), autoHideMs);
    return () => window.clearTimeout(timer);
  }, [isH5, visible, paused, activity, autoHideMs]);

  const toggle = useCallback(() => {
    setVisible((value) => !value);
    setActivity((n) => n + 1);
  }, []);

  const reveal = useCallback(() => {
    setVisible(true);
    setActivity((n) => n + 1);
  }, []);

  const surfaceProps = useMemo(
    () => ({
      onPointerDown(event: ReactPointerEvent) {
        if (!isH5) return;
        // 点按钮/链接/输入框、拖选文字都不算「单击正文」
        if (isIgnorableSwipeTarget(event.target)) {
          start.current = null;
          return;
        }
        start.current = { x: event.clientX ?? 0, y: event.clientY ?? 0, t: Date.now() };
      },
      onPointerUp(event: ReactPointerEvent) {
        if (!isH5) return;
        const from = start.current;
        start.current = null;
        if (!from) return;
        if (Date.now() - from.t > TAP_MAX_MS) return;
        const dx = (event.clientX ?? 0) - from.x;
        const dy = (event.clientY ?? 0) - from.y;
        // 滑动翻章交给 useSwipePaging：位移大就不在这里处理
        if (Math.abs(dx) > TAP_SLOP || Math.abs(dy) > TAP_SLOP) return;
        toggle();
      },
      onPointerCancel() {
        start.current = null;
      },
    }),
    [isH5, toggle],
  );

  return { visible: isH5 ? visible : true, toggle, reveal, surfaceProps };
}
