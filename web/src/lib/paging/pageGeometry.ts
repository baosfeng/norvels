/**
 * 分页几何：页宽 / 页高 / 行高对齐 / 页码换算。纯函数，单测覆盖。
 *
 * 「页」由多栏排版源（.pg-source）的列决定：列宽 = 页宽，列数 = 总页数。
 * 所以这里只算两件事：一列多宽（= 可视宽减版心留白）、一列多高（= 可用高向下取整到行高倍数）。
 */

export interface PageBoxInput {
  /** 可视宽（.pg-stage 的 offsetWidth）。 */
  stageW: number;
  /** 可视高（.pg-stage 的 offsetHeight）。 */
  stageH: number;
  /** 版心左右留白（px）。H5 阅读器的留白由 .reader-main 的 padding 承担，这里通常是 0。 */
  padX: number;
  /** 行高（px）：页高必须对齐到它的整数倍。 */
  lineH: number;
}

export interface PageBox {
  /** 一页的正文宽度（= 多栏排版源的列宽）。 */
  pageW: number;
  /** 一页的正文高度（行高整数倍）。 */
  pageH: number;
  padX: number;
  /** 页高之外的余量，上下均分（正文块因此在可视区里垂直居中）。 */
  padY: number;
  lineH: number;
  viewportW: number;
  viewportH: number;
}

export const EMPTY_BOX: PageBox = {
  pageW: 0,
  pageH: 0,
  padX: 0,
  padY: 0,
  lineH: 0,
  viewportW: 0,
  viewportH: 0,
};

/** 一份不能用的尺寸（元素还没挂载 / 隐藏 / 容器 0 高）。 */
export function isEmptyBox(box: PageBox): boolean {
  return !(box.pageW > 0 && box.pageH > 0);
}

function positiveInt(value: number): number {
  return Number.isFinite(value) && value > 0 ? Math.floor(value) : 0;
}

/**
 * 算一页的尺寸。
 *
 * 页高向下取整到行高整数倍是硬要求：多栏排版的列断点只落在行盒之间，
 * 页高比行高多出来的那截会被切进「下一行的一半」，读者看到的是被裁掉的字。
 * 取整余出的高度不当空白扔掉，而是上下均分给 padY——正文块因此居中，底部不会空一块。
 */
export function computePageBox(input: PageBoxInput): PageBox {
  const viewportW = positiveInt(input.stageW);
  const viewportH = positiveInt(input.stageH);
  const padX = Math.max(0, Math.floor(input.padX) || 0);
  const lineH = Number.isFinite(input.lineH) && input.lineH > 0 ? input.lineH : 0;
  const pageW = Math.max(0, viewportW - padX * 2);
  if (lineH === 0) {
    return { pageW, pageH: viewportH, padX, padY: 0, lineH: 0, viewportW, viewportH };
  }
  const rows = Math.floor(viewportH / lineH);
  const pageH = Math.max(lineH, rows * lineH);
  const padY = Math.max(0, Math.round((viewportH - pageH) / 2));
  return { pageW, pageH, padX, padY, lineH, viewportW, viewportH };
}

/**
 * 总页数 = 排版源的 scrollWidth / 页宽。
 * 多栏容器的 scrollWidth 是列宽的整数倍，这是「页数」的唯一真值——
 * 不数行、不估字，字号 / 字体 / 段距怎么变都不用改这里的口径。
 */
export function countPages(scrollWidth: number, pageW: number): number {
  if (!(pageW > 0) || !Number.isFinite(scrollWidth)) return 1;
  return Math.max(1, Math.round(scrollWidth / pageW));
}

/** 页码夹取：越界一律落到最近的有效页（首 / 末页），不返回 NaN。 */
export function clampPage(page: number, pageCount: number): number {
  if (!Number.isFinite(page)) return 0;
  const last = Math.max(0, pageCount - 1);
  return Math.min(last, Math.max(0, Math.round(page)));
}

/** 页序号 → 0..1 的进度（末页为 1；只有一页时为 0，细条不显示成半截）。 */
export function pagePercent(page: number, pageCount: number): number {
  if (!(pageCount > 1)) return 0;
  const ratio = clampPage(page, pageCount) / (pageCount - 1);
  return Math.min(1, Math.max(0, ratio));
}

/** 页码文案：分页进度浮标用「本章 第 p / P 页」。 */
export function formatPageLabel(page: number, pageCount: number): string {
  return `第 ${clampPage(page, pageCount) + 1} / ${Math.max(1, pageCount)} 页`;
}

/** 列边界的浮点容差（px）：只吸收 getBoundingClientRect 的亚像素误差，不改变列的归属。 */
const COLUMN_EPSILON = 0.01;

/** 横向偏移（相对排版源左缘）落在第几列：正好等于页宽才算下一列。 */
export function pageOfOffset(offsetX: number, pageW: number): number {
  if (!(pageW > 0) || !Number.isFinite(offsetX)) return 0;
  return Math.max(0, Math.floor((offsetX + COLUMN_EPSILON) / pageW));
}

/** 第 n 页的排版源横向位移（写进 translateX 的值，负向；0 页要的是 0 不是 -0）。 */
export function translateForPage(page: number, pageW: number): number {
  const offset = Math.max(0, page) * Math.max(0, pageW);
  return offset === 0 ? 0 : -offset;
}
