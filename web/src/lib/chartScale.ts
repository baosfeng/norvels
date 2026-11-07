/**
 * 图表几何内核：纯函数、无 DOM、无 React。
 *
 * 驾驶舱所有图（柱 / 条 / 折线 / 堆叠 / 热力图）共用这一份坐标算法，
 * 于是「柱宽 62%」「0 基线」「刻度取 1/2/5 × 10^n」「标签最小间距 44px」
 * 这些口径只在一处定义，改一次全图生效，也能单独单测。
 */

/** 数字收敛到区间内（NaN 一律当 lo）。 */
export function clampNumber(value: number, lo: number, hi: number): number {
  if (!Number.isFinite(value)) return lo;
  return Math.min(Math.max(value, lo), hi);
}

export interface BandScale {
  /** 一列的步长（含列间留白） */
  readonly step: number;
  /** 柱子可用宽度 = step × (1 − padding) */
  readonly bandwidth: number;
  /** 第 index 列的左边缘（柱子左缘） */
  readonly start: (index: number) => number;
  /** 第 index 列的中心 */
  readonly center: (index: number) => number;
}

/**
 * 分列刻度：把宽度均分给 count 列，每列留出 padding 比例的左右空隙。
 *
 * padding 默认 0.38 —— 柱宽因此是列宽的 62%，14 天的时间轴上相邻柱子不会糊在一起。
 */
export function bandScale(count: number, width: number, padding = 0.38): BandScale {
  const n = Math.max(0, Math.floor(Number.isFinite(count) ? count : 0));
  const w = Math.max(0, Number.isFinite(width) ? width : 0);
  if (n === 0 || w === 0) {
    return { step: 0, bandwidth: 0, start: () => 0, center: () => 0 };
  }
  const step = w / n;
  const bandwidth = step * (1 - clampNumber(padding, 0, 0.9));
  const inset = (step - bandwidth) / 2;
  return {
    step,
    bandwidth,
    start: (index) => index * step + inset,
    center: (index) => index * step + step / 2,
  };
}

export interface LinearScale {
  readonly domainMin: number;
  readonly domainMax: number;
  readonly rangeStart: number;
  readonly rangeEnd: number;
  /** 数值 → 像素 */
  readonly scale: (value: number) => number;
  /** 像素 → 数值 */
  readonly invert: (pixel: number) => number;
  /** 0 所在的像素位置（柱子的起点） */
  readonly zero: number;
}

/**
 * 线性刻度：域 [domainMin, domainMax] → 像素 [rangeStart, rangeEnd]。
 * SVG 的 y 轴通常传 rangeStart = 底、rangeEnd = 顶（像素往上变小）。
 * 域退化成 0 宽时给 1 的跨度，避免除零。
 */
export function linearScale(domainMin: number, domainMax: number, rangeStart: number, rangeEnd: number): LinearScale {
  const lo = Number.isFinite(domainMin) ? domainMin : 0;
  const hi = Number.isFinite(domainMax) && domainMax > lo ? domainMax : lo + 1;
  const span = hi - lo;
  return {
    domainMin: lo,
    domainMax: hi,
    rangeStart,
    rangeEnd,
    scale: (value) => rangeStart + ((clampNumber(value, lo, hi) - lo) / span) * (rangeEnd - rangeStart),
    invert: (pixel) => lo + ((pixel - rangeStart) / (rangeEnd - rangeStart || 1)) * span,
    zero: rangeStart + ((0 - lo) / span) * (rangeEnd - rangeStart),
  };
}

/**
 * 好看的刻度：步长取 1 / 2 / 5 × 10^n，返回从 0 开始、覆盖 max 的刻度数组。
 *
 * count 是「0 以上想要几格」；实际格数可能少一格（如 max = 838、count = 3 时步长 500，
 * 只要 500 / 1000 两条 —— 步长优先，不为了凑数去画 333 / 666 这种读不出来的刻度）。
 */
export function niceTicks(max: number, count = 3): number[] {
  if (!Number.isFinite(max) || max <= 0) return [0];
  const target = Math.max(1, Math.floor(Number.isFinite(count) ? count : 1));
  const rough = max / target;
  const magnitude = 10 ** Math.floor(Math.log10(rough));
  const norm = rough / magnitude;
  const step = (norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 5 ? 5 : 10) * magnitude;
  const top = Math.ceil(max / step) * step;
  const ticks: number[] = [];
  for (let i = 0; i * step <= top + step * 1e-6; i += 1) {
    ticks.push(Math.round(i * step * 1e6) / 1e6);
  }
  return ticks;
}

/**
 * y 轴上界：最高值再留 padRatio 的空气。
 *
 * 直接拿 niceTicks 的顶刻度当上界时，838 本会画到 1000 的刻度里 —— 最矮也要浪费 16% 的高度，
 * 图体填充率就掉到 80% 以下。这里改成「域 = 最大值 × (1+pad)」，刻度只画落在域内的那几条，
 * 最高一根柱稳定占 92% 的绘图区高度。
 */
export function domainTopOf(max: number, padRatio = 0.08): number {
  const value = Number.isFinite(max) && max > 0 ? max : 0;
  const pad = clampNumber(padRatio, 0, 0.6);
  return value > 0 ? value * (1 + pad) : 1;
}

/** 1/2/5 × 10ⁿ 的刻度步长：把 [0, top] 分成约 count 格。 */
export function tickStep(max: number, count = 3): number {
  if (!Number.isFinite(max) || max <= 0) return 1;
  const target = Math.max(1, Math.floor(Number.isFinite(count) ? count : 1));
  const rough = max / target;
  const magnitude = 10 ** Math.floor(Math.log10(rough));
  const norm = rough / magnitude;
  return (norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 5 ? 5 : 10) * magnitude;
}

/** [0, top] 内的刻度（含 0 与最靠近 top 的那一条），步长由 tickStep 决定。 */
export function ticksWithin(step: number, top: number): number[] {
  if (!Number.isFinite(step) || step <= 0 || !Number.isFinite(top) || top <= 0) return [0];
  const ticks: number[] = [0];
  const last = Math.floor(top / step + 1e-9);
  for (let i = 1; i <= last; i += 1) ticks.push(Math.round(i * step * 1e6) / 1e6);
  return ticks;
}

/**
 * 堆叠条的切点：起点按真实占比累加，不做最小宽度归一。
 *
 * 26px 高的堆叠条上「1.2% 最小宽度」看不出问题，但铺满图体之后会被读成真实占比，
 * 于是把 0.1% 的「其他」画成 1.2% 就是虚报。比例一律取真实值，太窄的那段由图例写「<1%」解释。
 */
export function stackSlices(values: number[], width: number): { start: number; end: number }[] {
  const total = values.reduce((sum, value) => (Number.isFinite(value) && value > 0 ? sum + value : sum), 0);
  const usable = Number.isFinite(width) && width > 0 ? width : 0;
  let cursor = 0;
  return values.map((value) => {
    const share = total > 0 && Number.isFinite(value) && value > 0 ? value / total : 0;
    const length = share * usable;
    const slice = { start: cursor, end: cursor + length };
    cursor += length;
    return slice;
  });
}

/** 标签最小间距（px）：日期串「10/18」加左右余量，再近就互相压住。 */
export const LABEL_MIN_GAP = 44;

/** 每隔几列标一次：至少隔 minGap 像素一次，最少 1（每列都标）。 */
export function labelStride(count: number, step: number, minGap = LABEL_MIN_GAP): number {
  const n = Math.max(0, Math.floor(Number.isFinite(count) ? count : 0));
  if (n <= 1) return 1;
  if (!Number.isFinite(step) || step <= 0) return n;
  return Math.max(1, Math.ceil(minGap / step));
}

/**
 * 哪些列要画标签：首尾必标（不标首尾就不知道这 14 天是哪 14 天），中间按 stride 抽稀。
 * 抽稀点离末尾不足一个 stride 时让位给末尾：否则最后两个日期会挨在一起（实测 14 天 350px 宽时
 * 「11/6」「11/7」只隔 23px，比最小间距 44px 还近）。
 */
export function labelVisible(count: number, stride: number): boolean[] {
  const n = Math.max(0, Math.floor(Number.isFinite(count) ? count : 0));
  const flags = new Array<boolean>(n).fill(false);
  if (n === 0) return flags;
  flags[0] = true;
  if (n === 1) return flags;
  flags[n - 1] = true;
  const gap = Math.max(1, Math.floor(Number.isFinite(stride) ? stride : 1));
  for (let index = gap; index < n - gap; index += gap) {
    flags[index] = true;
  }
  return flags;
}

/** 柱宽：列宽的 62%，但不小于 4px（再细就看不出是柱子）、不大于 28px（再宽就成色块）。 */
export function barWidth(bandwidth: number, ratio = 0.62, min = 4, max = 28): number {
  const raw = (Number.isFinite(bandwidth) ? bandwidth : 0) * ratio;
  if (raw <= 0) return 0;
  return clampNumber(raw, min, max);
}

/**
 * 柱高：0 值给 1px 基线。
 * 真实库里「14 天只导入 8 天」，0 值那天既不能画成没有（读者以为缺数据），
 * 也不能给可见高度（看起来像有量），1px 基线正好表达「这天是 0」。
 */
export const ZERO_BAR_PX = 1;

export function barHeight(value: number, scale: LinearScale, minPx = ZERO_BAR_PX): number {
  if (!Number.isFinite(value) || value <= 0) return minPx;
  const h = Math.abs(scale.zero - scale.scale(value));
  return Math.max(minPx, h);
}

/** 单个字符的宽度估算（em 倍数）：中文/全角 1em，数字与字母 0.55em，空格 0.28em。 */
function charEm(code: number): number {
  if (code > 0x2e80) return 1;
  if (code === 32) return 0.28;
  if (code >= 48 && code <= 57) return 0.58;
  if (code >= 0x21 && code <= 0x7e) return 0.55;
  return 0.9;
}

/** 文本宽度估算：SVG 里排字前先知道要多宽（不测真实排版，够用来避让与截断）。 */
export function estimateTextWidth(text: string, fontSize: number): number {
  const size = Number.isFinite(fontSize) && fontSize > 0 ? fontSize : 11;
  let em = 0;
  for (const char of text) em += charEm(char.codePointAt(0) ?? 0);
  return em * size;
}

/** 按估算宽度截断并补「…」：标签列放不下的长名字用它，全文仍可由浮层给出。 */
export function clipText(text: string, maxWidth: number, fontSize: number): string {
  if (estimateTextWidth(text, fontSize) <= maxWidth) return text;
  const chars = [...text];
  while (chars.length > 1 && estimateTextWidth(chars.join("") + "…", fontSize) > maxWidth) {
    chars.pop();
  }
  return chars.join("") + "…";
}
