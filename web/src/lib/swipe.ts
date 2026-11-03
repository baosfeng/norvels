/**
 * 阅读器左右滑动翻页的手势判定。
 *
 * 抽成纯函数是为了能单测：阈值、方向判定、与竖向滚动/文字选择互不干扰的规则都在这里，
 * 组件只负责把 touch 事件喂进来。
 */

/** 触发翻页的最小横向位移（px）。 */
export const SWIPE_THRESHOLD = 60;

/**
 * 横向位移必须达到纵向的几倍才算翻页（2.0 ≈ 27°）。
 *
 * 读者的实际动作是「向下滚正文」，手指几乎不可能完全垂直，横向漂移 40–70px 很常见；
 * 旧值 1.2（≈40°）把这种滚动判成了翻章——实测 dx=70/dy=45 直接翻到下一章。
 * 问题不在位移长度（阈值 60 未动），在角度：只有明显横向才接管。
 */
export const SWIPE_ANGLE_RATIO = 2;

/** 方向锁定的最小位移（px）：越过它才决定这段手势归谁，之下的抖动仍是「还没决定」。 */
export const SWIPE_LOCK_SLOP = 12;

/** 跟手位移的上限（px）：一屏宽的 30% 左右，拖起来才像在掀页。 */
export const SWIPE_FOLLOW_MAX = 120;

/** 跟手上限的最小值（px）：窄屏手机上也要能拖动小半屏。 */
export const SWIPE_FOLLOW_MIN = 72;

/** 手指位移到正文偏移的阻尼比：跟手但比手指慢，读者才看得出「拖到头了」。 */
export const SWIPE_FOLLOW_DAMPING = 0.32;

/** 松手未达阈值时弹回原位的过渡时长（ms）。 */
export const SWIPE_SETTLE_MS = 200;

export interface SwipePoint {
  x: number;
  y: number;
}

export type SwipeAction = "prev" | "next" | null;

/** 这段手势归谁：未定 / 横向翻页 / 纵向滚动。判定一次就锁死，后续抖动不改判。 */
export type SwipeLock = "undecided" | "horizontal" | "vertical";

/**
 * 判定一次滑动手势。
 * - 位移不足阈值 → null（可能是误触或慢拖）
 * - 横向位移必须 ≥ 纵向位移 × SWIPE_ANGLE_RATIO，避免斜着滚正文时误翻页
 * - 向右滑（dx > 0）看作「回上一章」，向左滑看作「下一章」
 */
export function resolveSwipe(start: SwipePoint, end: SwipePoint): SwipeAction {
  const dx = end.x - start.x;
  const dy = end.y - start.y;
  if (Math.abs(dx) < SWIPE_THRESHOLD) return null;
  if (Math.abs(dx) < Math.abs(dy) * SWIPE_ANGLE_RATIO) return null;
  return dx > 0 ? "prev" : "next";
}

/**
 * 手势开始后锁定方向：越过 {@link SWIPE_LOCK_SLOP} 才表态。
 * 明显横向 → 交给翻页；其余（含 33° 这种斜着滚）一律当滚动，整段不翻章——
 * 手指微抖不该让读者「翻了一半又变回滚动」。
 */
export function lockSwipe(start: SwipePoint, current: SwipePoint): SwipeLock {
  const dx = Math.abs(current.x - start.x);
  const dy = Math.abs(current.y - start.y);
  if (dx >= SWIPE_LOCK_SLOP && dx >= dy * SWIPE_ANGLE_RATIO) return "horizontal";
  if (dy >= SWIPE_LOCK_SLOP) return "vertical";
  return "undecided";
}

/**
 * 跟手偏移：内容跟着手指走但更慢（阻尼），并在上限处封顶。
 * 左滑 dx < 0 → 偏移为负（内容左移，露出后面的纸面），与方向语义一致。
 *
 * 上限按屏宽取 {@link followLimit}：手机上跟手到底约 1/3 屏，读者看得出这是「翻页」，
 * 不再是 16px 那种似动不动。
 */
export function followOffset(dx: number, max = SWIPE_FOLLOW_MAX): number {
  const raw = dx * SWIPE_FOLLOW_DAMPING;
  if (!Number.isFinite(raw)) return 0;
  const limit = Number.isFinite(max) && max > 0 ? max : SWIPE_FOLLOW_MAX;
  return Math.min(limit, Math.max(-limit, raw));
}

/** 跟手上限：屏宽的 30%，夹在 {@link SWIPE_FOLLOW_MIN}–{@link SWIPE_FOLLOW_MAX} 之间。 */
export function followLimit(viewportWidth: number): number {
  if (!Number.isFinite(viewportWidth) || viewportWidth <= 0) return SWIPE_FOLLOW_MAX;
  return Math.min(SWIPE_FOLLOW_MAX, Math.max(SWIPE_FOLLOW_MIN, viewportWidth * 0.3));
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
