/**
 * 段落锚点：页码 ↔ 段号的换算。纯函数。
 *
 * 为什么锚点是段号不是页码：分页结果随字号 / 行距 / 视口高变化，
 * 页码在重排后没有意义；段落边界跨字号稳定，读者感知就是「还停在这段」。
 */
import { pageOfOffset } from "./pageGeometry";

export { pageOfOffset };

/** 一个段落在排版源里的位置：左边距（相对排版源左缘）+ 段顶（相对排版源顶）+ 段号。 */
export interface ParaBox {
  /** 段落号（章内 0 起；标题记作 -1，不参与锚点）。 */
  pi: number;
  /** 段首碎片所在列的横向偏移（= 列号 × 页宽）。 */
  left: number;
  /** 段首碎片在这一列里的顶端偏移（0 = 从列首开始）。缺省按 0 处理（列首）。 */
  top?: number;
}

/** 该段落起始于第几页；找不到时为 null。 */
export function pageOfParagraph(boxes: readonly ParaBox[], pi: number, pageW: number): number | null {
  if (!Number.isFinite(pi)) return null;
  const box = boxes.find((item) => item.pi === pi);
  return box ? pageOfOffset(box.left, pageW) : null;
}

/**
 * 某页的「锚点段落」= 这一页页首正在被读的那一段。
 *
 * 口径是「起点**不晚于页首**的最后一个段落」：它可能早就开始了（跨页长段），
 * 页首那几行正是它的后半截——拿它当锚点，字号一变读者回到的是「这一段的中途」，
 * 而不是被弹回段首或跳到本页末尾才开始的那一段。
 * 一页里恰好只有新段起点时，两者是同一条；没有正文时返回 null。
 *
 * 别用「起始页 ≤ 本页」判：那会把**本页内才开始**的段也算进来，取到页尾那一段。
 * 实测 20→28 字号：第 3 页大半是长段的后半、页尾才开始新段，锚点记成页尾段，
 * 重排后读者被送到第 6 页（往后漂了整整一页多）。
 */
export function firstParaOnPage(boxes: readonly ParaBox[], page: number, pageW: number): number | null {
  if (boxes.length === 0 || !(pageW > 0)) return null;
  const pageStart = page * pageW;
  let pick: number | null = null;
  for (const box of boxes) {
    // 只看「起点不晚于页首」的段：跨页段（起于前面的列）或恰好从列首开始的那一段。
    // 本页列中部才开始的段不算——那可能是本页最后一小段，拿它当锚点读者会被送走大半页。
    const atPageStart = box.left > pageStart - 0.5 && box.left <= pageStart + 0.5 && (box.top ?? 0) <= 0.5;
    if (box.left <= pageStart - 0.5 || atPageStart) pick = box.pi;
  }
  if (pick !== null) return pick;
  // 兜底：本页没有任何段从列首开始（页首全是上一段的后半），取起始页 ≤ 本页的最后一段
  for (const box of boxes) {
    if (box.left <= pageStart + 0.5) pick = box.pi;
  }
  return pick ?? boxes[0].pi;
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
