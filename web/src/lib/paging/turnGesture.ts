/**
 * 翻页手势的判定：方向锁、跟手进度 p、提交判定、回弹时长。纯函数。
 *
 * 参数全部取自原型实测（/tmp/norvels-flip-lab/README.md「手势参数」一节），
 * 五种翻页效果共用同一套——换效果只换视觉，手感不变。
 */

/** 松手翻页的位移阈值：超过可视宽的 22% 就翻。 */
export const COMMIT_RATIO = 0.22;
/** 松手翻页的速度阈值（px/ms）；方向必须与拖动一致，反向甩动不算。 */
export const COMMIT_VELOCITY = 0.45;
/** 首末页越界时的最大拉动比例：只做阻尼，不翻页、不回弹穿透。 */
export const RUBBER_MAX = 0.18;
/** 方向锁定阈值（px）：横向位移超过它才判定这段手势归翻页。 */
export const DRAG_START_SLOP = 6;
/** 越界回弹时长（ms）。 */
export const BOUNDARY_REBOUND_MS = 220;

export type DragLock = "undecided" | "horizontal" | "vertical";
/** 翻页方向：+1 下一页（向左滑），-1 上一页（向右滑）。 */
export type TurnDir = 1 | -1;

/**
 * 方向锁：越过阈值就表态，之后不再改判。
 * 竖向优先——读者多半是在上下滚正文，横向手指漂移不该把这一页翻掉。
 */
export function lockDrag(dx: number, dy: number, slop = DRAG_START_SLOP): DragLock {
  const ax = Math.abs(dx);
  const ay = Math.abs(dy);
  if (ax >= slop && ax > ay) return "horizontal";
  if (ay >= slop && ay >= ax) return "vertical";
  return "undecided";
}

/** 翻页方向：左滑（dx < 0）是下一页。 */
export function turnDirection(dx: number): TurnDir {
  return dx < 0 ? 1 : -1;
}

/** 跟手进度 p：位移按**可视宽**归一（不是版心宽，否则两页之间会露缝）。 */
export function dragProgress(dx: number, viewportW: number): number {
  if (!(viewportW > 0)) return 0;
  const p = Math.abs(dx) / viewportW;
  if (!Number.isFinite(p)) return 0;
  return Math.min(1.25, Math.max(0, p));
}

/**
 * 松手是否翻页：位移过阈值，或甩得够快且方向与拖动一致。
 * 速度是 px/ms，负值表示向左（下一页）；方向不一致的快速回甩不该翻页。
 */
export function shouldCommit(p: number, velocity: number, dx: number): boolean {
  if (p > COMMIT_RATIO) return true;
  const speed = Math.abs(velocity);
  if (!(speed > COMMIT_VELOCITY)) return false;
  if (Math.abs(dx) < 1) return true;
  return Math.sign(velocity) === Math.sign(dx);
}

/** 收尾时长：回弹按剩余距离等比缩短（×0.75），提交走满全程。 */
export function settleDuration(base: number, from: number, to: 0 | 1): number {
  if (!(base > 0)) return 0;
  const distance = Math.abs(to - from);
  return Math.round(base * distance * (to === 0 ? 0.75 : 1));
}

/** 越界阻尼位移（px）：easeOutCubic 压扁，越拉越拉不动。 */
export function rubberOffset(dx: number, viewportW: number): number {
  if (!(viewportW > 0)) return 0;
  const pull = Math.min(1, Math.abs(dx) / viewportW);
  return Math.sign(dx) * RUBBER_MAX * viewportW * (1 - Math.pow(1 - pull, 3)) * 0.5;
}
