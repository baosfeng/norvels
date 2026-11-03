import { useCallback, useEffect, useLayoutEffect, useRef } from "react";
import type { RefObject } from "react";
import {
  followLimit,
  followOffset,
  isIgnorableSwipeTarget,
  lockSwipe,
  resolveSwipe,
  SWIPE_SETTLE_MS,
  type SwipeLock,
  type SwipePoint,
} from "./swipe";

/** 跟手位移的下标：内层承载正文，外层负责点击与「可聚焦区域」，只动内层就不影响三区手势。 */
const FOLLOW_TARGET_SELECTOR = ".reader-swipe-layer";

/** 位移归零的过渡：松手未达阈值时弹回原位，读者看得出「这次没翻」。 */
const SETTLE_TRANSITION = `transform ${SWIPE_SETTLE_MS}ms cubic-bezier(0.2, 0, 0, 1)`;

/**
 * 阅读器左右滑动翻页。
 *
 * 返回一个 ref 绑到手势区上：元素引用归 hook 自己持有，渲染期不读 ref.current。
 * 只在触摸设备且没有面板打开时生效（enabled）；起点落在按钮/链接/输入框或已选文字上时不接管，
 * 交给浏览器原本行为。手势一旦判定为横向就锁死方向，之后不再改判为滚动。
 *
 * 跟手反馈只写一处 inline transform（不叠 CSS 动画）：翻章成功由页面的翻页转场接手
 * （见 lib/chapterFlip：旧快照带着位移滑出、新页从原位进场），本 hook 不抢着复位。
 */
export function useSwipePaging(handlers: {
  onPrev: () => void;
  onNext: () => void;
  enabled: boolean;
  reducedMotion: boolean;
}) {
  const element = useRef<HTMLElement | null>(null);
  const ref: RefObject<HTMLElement | null> = useRef<HTMLElement | null>(null);

  // 用一个回调 ref 同步到 element，避免在渲染期读取/写入 ref
  const setRef = (node: HTMLElement | null) => {
    element.current = node;
    ref.current = node;
  };

  const start = useRef<SwipePoint | null>(null);
  const lock = useRef<SwipeLock>("undecided");
  const offset = useRef(0);
  const latest = useRef(handlers);
  useLayoutEffect(() => {
    latest.current = handlers;
  }, [handlers]);

  /**
   * 手势层复位。
   * - 未达阈值 / 取消：带 200ms 过渡弹回，读者看得出「这次没翻」
   * - 翻章成功：不在这里复位（见 onTouchEnd）
   */
  const reset = useCallback((withTransition = true) => {
    const surface = element.current;
    if (!surface) return;
    const layer = surface.querySelector<HTMLElement>(FOLLOW_TARGET_SELECTOR);
    if (layer && offset.current !== 0) {
      layer.style.transition = withTransition ? SETTLE_TRANSITION : "none";
      layer.style.transform = "translateX(0px)";
    }
    offset.current = 0;
  }, []);

  /** 跟手位移：只在横向锁定后生效；减少动效时整条路径不写样式。 */
  const follow = useCallback((dx: number) => {
    const surface = element.current;
    if (!surface || latest.current.reducedMotion) return;
    const layer = surface.querySelector<HTMLElement>(FOLLOW_TARGET_SELECTOR);
    if (!layer) return;
    const next = followOffset(dx, followLimit(window.innerWidth));
    if (next === offset.current) return;
    offset.current = next;
    layer.style.transition = "none";
    layer.style.transform = `translateX(${next}px)`;
  }, []);

  useEffect(() => {
    const target = element.current;
    if (!target) return;
    if (!latest.current.enabled) {
      reset();
      return;
    }

    const onTouchStart = (event: TouchEvent) => {
      lock.current = "undecided";
      if (event.touches.length !== 1) {
        start.current = null;
        return;
      }
      const touch = event.touches[0];
      start.current = isIgnorableSwipeTarget(event.target) ? null : { x: touch.clientX, y: touch.clientY };
    };

    const onTouchMove = (event: TouchEvent) => {
      const from = start.current;
      if (!from) return;
      const touch = event.touches[0];
      if (!touch) return;
      const now = { x: touch.clientX, y: touch.clientY };
      if (lock.current === "undecided") lock.current = lockSwipe(from, now);
      if (lock.current === "horizontal") follow(now.x - from.x);
    };

    const onTouchEnd = (event: TouchEvent) => {
      const from = start.current;
      const locked = lock.current;
      start.current = null;
      lock.current = "undecided";
      if (!from) {
        reset();
        return;
      }
      // 先判为纵向：这段手势归滚动，之后横向甩多远都不翻章
      if (locked === "vertical") {
        reset();
        return;
      }
      const touch = event.changedTouches[0];
      if (!touch) {
        reset();
        return;
      }
      // 横向已锁定，或一次 move 都没收到（极快轻扫）：交给阈值判定
      const action = resolveSwipe(from, { x: touch.clientX, y: touch.clientY });
      if (action === null) {
        reset();
        return;
      }
      // 翻章：支持快照转场时把位移留给转场（旧页带着它滑出、新页从原位进场，
      // 清理由 lib/chapterFlip 在拍新快照前完成）；不支持时立刻复位，别让正文歪着。
      if (typeof document.startViewTransition !== "function") reset(false);
      else offset.current = 0;
      if (action === "prev") latest.current.onPrev();
      else latest.current.onNext();
    };

    const onTouchCancel = () => {
      start.current = null;
      lock.current = "undecided";
      reset();
    };

    target.addEventListener("touchstart", onTouchStart, { passive: true });
    target.addEventListener("touchmove", onTouchMove, { passive: true });
    target.addEventListener("touchend", onTouchEnd, { passive: true });
    target.addEventListener("touchcancel", onTouchCancel, { passive: true });
    return () => {
      target.removeEventListener("touchstart", onTouchStart);
      target.removeEventListener("touchmove", onTouchMove);
      target.removeEventListener("touchend", onTouchEnd);
      target.removeEventListener("touchcancel", onTouchCancel);
    };
  }, [handlers.enabled, handlers.onPrev, handlers.onNext, handlers.reducedMotion, follow, reset]);

  return setRef;
}
