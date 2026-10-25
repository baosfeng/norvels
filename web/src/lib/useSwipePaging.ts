import { useEffect, useLayoutEffect, useRef } from "react";
import type { RefObject } from "react";
import { isIgnorableSwipeTarget, resolveSwipe, type SwipePoint } from "./swipe";

/**
 * 阅读器左右滑动翻页。
 *
 * 返回一个 ref 绑到手势区上：元素引用归 hook 自己持有，渲染期不读 ref.current。
 * 只在触摸设备上生效（H5 目标）；起点落在按钮/链接/输入框或已选文字上时不接管，交给浏览器原本行为。
 */
export function useSwipePaging(handlers: { onPrev: () => void; onNext: () => void; enabled: boolean }) {
  const element = useRef<HTMLElement | null>(null);
  const ref: RefObject<HTMLElement | null> = useRef<HTMLElement | null>(null);

  // 用一个回调 ref 同步到 element，避免在渲染期读取/写入 ref
  const setRef = (node: HTMLElement | null) => {
    element.current = node;
    ref.current = node;
  };

  const start = useRef<SwipePoint | null>(null);
  const latest = useRef(handlers);
  useLayoutEffect(() => {
    latest.current = handlers;
  }, [handlers]);

  useEffect(() => {
    const target = element.current;
    if (!target) return;
    if (!latest.current.enabled) return;

    const onTouchStart = (event: TouchEvent) => {
      if (event.touches.length !== 1) {
        start.current = null;
        return;
      }
      const touch = event.touches[0];
      start.current = isIgnorableSwipeTarget(event.target) ? null : { x: touch.clientX, y: touch.clientY };
    };

    const onTouchEnd = (event: TouchEvent) => {
      const from = start.current;
      start.current = null;
      if (!from) return;
      const touch = event.changedTouches[0];
      if (!touch) return;
      const action = resolveSwipe(from, { x: touch.clientX, y: touch.clientY });
      if (action === "prev") latest.current.onPrev();
      else if (action === "next") latest.current.onNext();
    };

    const onTouchCancel = () => {
      start.current = null;
    };

    target.addEventListener("touchstart", onTouchStart, { passive: true });
    target.addEventListener("touchend", onTouchEnd, { passive: true });
    target.addEventListener("touchcancel", onTouchCancel, { passive: true });
    return () => {
      target.removeEventListener("touchstart", onTouchStart);
      target.removeEventListener("touchend", onTouchEnd);
      target.removeEventListener("touchcancel", onTouchCancel);
    };
  }, [handlers.enabled, handlers.onPrev, handlers.onNext]);

  return setRef;
}
