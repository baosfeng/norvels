/**
 * 段落锚点：页码 ↔ 段号的换算。纯函数。
 *
 * 为什么锚点是段号不是页码：分页结果随字号 / 行距 / 视口高变化，
 * 页码在重排后没有意义；段落边界跨字号稳定，读者感知就是「还停在这段」。
 */
import { pageOfOffset } from "./pageGeometry";

export { pageOfOffset };

/** 一个段落在排版源里的位置：左边距（相对排版源左缘）+ 段号。 */
export interface ParaBox {
  /** 段落号（章内 0 起；标题记作 -1，不参与锚点）。 */
  pi: number;
  left: number;
}

/** 该段落起始于第几页；找不到时为 null。 */
export function pageOfParagraph(boxes: readonly ParaBox[], pi: number, pageW: number): number | null {
  if (!Number.isFinite(pi)) return null;
  const box = boxes.find((item) => item.pi === pi);
  return box ? pageOfOffset(box.left, pageW) : null;
}

/**
 * 某页的「锚点段落」= 从这一页起第一个**起始**在本页或之后的段落。
 *
 * 取「起始在页首之后」而不是「页首那个字所属的段落」：后者可能是一个横跨四五页的
 * 长段落，拿它当锚点等于把读者永远钉在段落开头，字号一变就往回跳。
 * 找不到（这一页后面已经没有段落起点）时回落到最后一个段落——
 * 末页常常是某个长段的后半截，此时锚点只能是它。
 */
export function firstParaOnPage(boxes: readonly ParaBox[], page: number, pageW: number): number | null {
  if (boxes.length === 0 || !(pageW > 0)) return null;
  for (const box of boxes) {
    if (pageOfOffset(box.left, pageW) >= page) return box.pi;
  }
  return boxes[boxes.length - 1].pi;
}

/**
 * 某页覆盖的段落区间（含首尾）：读屏用的「这一页有哪些文字」按它切。
 *
 * 起点 = 第一个起始于本页或之后的段落（与 firstParaOnPage 同一口径），
 * 终点 = 最后一个**起始页 ≤ 本页**的段落。两者可能重合（一页里只有一个新段起点），
 * 也可能差好几段（段落很短）。返回 null 表示这页没有正文（空章）。
 */
export function paragraphRangeOfPage(
  boxes: readonly ParaBox[],
  page: number,
  pageW: number,
): { start: number; end: number } | null {
  if (boxes.length === 0 || !(pageW > 0)) return null;
  const starts = boxes.map((box) => ({ pi: box.pi, at: pageOfOffset(box.left, pageW) }));
  // 终点 = 本页最后一个「已经开始」的段落：这一页至少读到这里
  const started = starts.filter((item) => item.at <= page);
  const end = started.length > 0 ? started[started.length - 1].pi : starts[0].pi;
  // 起点 = 起始页严格早于本页的最后一段（它可能是跨页长文，本页读的是后半截）；
  // 没有跨页段时从第一个「起始于本页」的段落读起。
  const spanning = started.filter((item) => item.at < page);
  const start =
    spanning.length > 0
      ? spanning[spanning.length - 1].pi
      : starts.find((item) => item.at >= page)?.pi ?? end;
  return { start: Math.min(start, end), end };
}

/** 段号文案的合法化：书签 / 进度里的段号可能是后端来的脏值。 */
export function normalizeParaIndex(value: unknown, fallback = 0): number {
  const n = typeof value === "number" ? value : Number(value);
  if (!Number.isFinite(n) || n < 0) return fallback;
  return Math.floor(n);
}
