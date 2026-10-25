/**
 * 阅读器左右滑动翻页的手势判定。
 *
 * 抽成纯函数是为了能单测：阈值、方向判定、与竖向滚动/文字选择互不干扰的规则都在这里，
 * 组件只负责把 touch 事件喂进来。
 */

/** 触发翻页的最小横向位移（px）。 */
export const SWIPE_THRESHOLD = 60;

/** 横向必须明显大于纵向，否则视为滚动而非翻页。 */
export const SWIPE_ANGLE_RATIO = 1.2;

export interface SwipePoint {
  x: number;
  y: number;
}

export type SwipeAction = "prev" | "next" | null;

/**
 * 判定一次滑动手势。
 * - 位移不足阈值 → null（可能是误触或慢拖）
 * - 横向位移必须 > 纵向位移 × SWIPE_ANGLE_RATIO，避免斜着滚正文时误翻页
 * - 向右滑（dx > 0）看作「回上一章」，向左滑看作「下一章」
 */
export function resolveSwipe(start: SwipePoint, end: SwipePoint): SwipeAction {
  const dx = end.x - start.x;
  const dy = end.y - start.y;
  if (Math.abs(dx) < SWIPE_THRESHOLD) return null;
  if (Math.abs(dx) <= Math.abs(dy) * SWIPE_ANGLE_RATIO) return null;
  return dx > 0 ? "prev" : "next";
}

/**
 * 手势起点是否落在可交互元素或可选中文本上：
 * 落在这些位置时不接管，交给浏览器原本的行为（点按钮、拖选文字）。
 */
export const SWIPE_IGNORE_SELECTOR = [
  "a",
  "button",
  "input",
  "select",
  "textarea",
  "label",
  "[role='button']",
  "[role='dialog']",
  "[contenteditable='true']",
].join(",");

export function isIgnorableSwipeTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  if (target.closest(SWIPE_IGNORE_SELECTOR)) return true;
  // 已经选中了文字时不动，避免把「选词」当成翻页
  const selection = typeof window !== "undefined" ? window.getSelection() : null;
  return Boolean(selection && selection.toString().length > 0);
}
