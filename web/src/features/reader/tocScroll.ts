/**
 * 目录首帧定位：打开目录时当前章必须在第一帧就已经在可视区中间。
 *
 * 之前是「setTimeout 0 里 scrollIntoView」两拍后再滚：读者先看到列表停在第 1 章，
 * 隔一拍才跳到当前章（实测在 4413 章的目录里，这一拍就是一次明显的错位闪光）。
 * 定位改成渲染后、绘制前的布局阶段直接算 scrollTop，读者看不到中间态。
 *
 * 抽成纯函数的原因：jsdom 不做布局，只有数学可单测；组件里只剩「量三个数、赋值」。
 */

/** 首帧定位的验收容差：当前章行中心与可视区中心的偏差（px）。 */
export const TOC_CENTER_TOLERANCE = 40;

/** 把某一行放到可视区中间时的 scrollTop（已钳在可滚范围内）。 */
export function centerScrollTop(input: {
  /** 行顶相对滚动内容顶的偏移（含当前 scrollTop）。 */
  rowTop: number;
  rowHeight: number;
  clientHeight: number;
  scrollHeight: number;
}): number {
  const { rowTop, rowHeight, clientHeight, scrollHeight } = input;
  const middle = rowTop + rowHeight / 2 - clientHeight / 2;
  const max = Math.max(0, scrollHeight - clientHeight);
  if (!Number.isFinite(middle)) return 0;
  return Math.max(0, Math.min(max, Math.round(middle)));
}

/** 行顶在滚动内容里的偏移：用 rect 差 + 当前 scrollTop，不依赖 offsetParent 是谁。 */
export function rowTopIn(list: HTMLElement, row: HTMLElement): number {
  return list.scrollTop + (row.getBoundingClientRect().top - list.getBoundingClientRect().top);
}

/** 定位之后当前章行的中心离可视区中心多远（0 = 正中）；审计与单测共用这一个口径。 */
export function centerOffset(input: {
  rowTop: number;
  rowHeight: number;
  clientHeight: number;
  scrollTop: number;
}): number {
  const { rowTop, rowHeight, clientHeight, scrollTop } = input;
  return Math.abs(rowTop + rowHeight / 2 - (scrollTop + clientHeight / 2));
}

import { useLayoutEffect, useRef, type RefObject } from "react";

/**
 * 打开目录时把当前章放到可视区中间：在布局阶段直接算 scrollTop，第一帧就到位。
 *
 * 抽成 hook 只为一件事：TocPanel 里那段定位现在只剩「传参」，
 * 而这一步的数学（居中 / 钳边界 / 误差口径）全在下面的纯函数里，可单测。
 */
export function useTocFirstFramePosition(options: {
  active: boolean;
  loading: boolean;
  error: unknown;
  currentChapterId: number;
  currentIndex: number;
  revealed: number;
  total: number;
  listRef: RefObject<HTMLElement | null>;
  revealTo: (count: number) => void;
  /** 补一批的步长：目标章后面要留够内容，否则滚到底也只能贴底、居中不了。 */
  chunk: number;
}) {
  const { active, loading, error, currentChapterId, currentIndex, revealed, total, listRef, revealTo, chunk } = options;
  const positioned = useRef(false);
  useLayoutEffect(() => {
    if (!active || loading || error || positioned.current) return;
    if (currentChapterId <= 0 || currentIndex < 0) return;
    const list = listRef.current;
    if (!list) return;
    const needed = currentIndex + 1;
    if (needed > revealed) {
      revealTo(Math.min(total, needed + chunk));
      return;
    }
    const row = list.querySelector<HTMLElement>("[data-current-chapter='true']");
    if (!row) return;
    // 只有真的能滚（量到高度）才认这一帧：jsdom / 折叠面板里全是 0，留给真正的滚动路径
    if (list.clientHeight <= 0 || list.scrollHeight <= 0) return;
    positioned.current = true;
    list.scrollTop = centerScrollTop({
      rowTop: rowTopIn(list, row),
      rowHeight: row.offsetHeight || 44,
      clientHeight: list.clientHeight,
      scrollHeight: list.scrollHeight,
    });
  }, [active, loading, error, currentChapterId, currentIndex, revealed, total, listRef, revealTo, chunk]);
}
