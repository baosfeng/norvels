import type { Bucket, ImportTrendPoint, ReadingProgress, Stats } from "../api/types";

/**
 * 六张图的数据转换：只做「后端数值 → 绘图几何 + 一句人话结论」，
 * 不碰 DOM，方便单测，也保证图与结论由同一份数字算出、不会对不上。
 */

export interface VerticalBar {
  label: string;
  /** 是否绘制轴标签：日期多时按原型口径稀疏显示，避免标签互相压住 */
  labelVisible: boolean;
  value: number;
  /** 相对最大值的高度百分比，最小 2，保证 0 值也有可见基线 */
  heightPct: number;
  title: string;
}

export interface HorizontalBar {
  name: string;
  value: number;
  widthPct: number;
  display: string;
}

export interface DonutSegment {
  key: "finished" | "reading" | "unread";
  label: string;
  value: number;
  color: string;
  dash: string;
  offset: number;
}

/** 数字 → 「12.3 万 / 1.2 亿」，图表内联用，与列表页 formatCount 口径一致。 */
export function compact(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0";
  if (value >= 100_000_000) return `${(value / 100_000_000).toFixed(1)} 亿`.replace(".0 ", " ");
  if (value >= 10_000) return `${(value / 10_000).toFixed(1)} 万`.replace(".0 ", " ");
  return String(Math.round(value));
}

function pct(value: number, max: number, floor = 2): number {
  if (!max || max <= 0) return floor;
  return Math.max(floor, Math.round((value / max) * 100));
}

export interface ChartModel<T> {
  empty: boolean;
  note: string;
  data: T;
}

/* ── 1 近 14 天导入量 ── */
export function buildImportTrend(points: ImportTrendPoint[]): ChartModel<{ bars: VerticalBar[]; total: number }> {
  const bars = points.map((p, index) => ({
    label: shortDate(p.date),
    // 原型口径：每 3 根标一次 + 最后一根；若倒数第二根也是「每 3 根」的位置会与最后一根贴住，则让位给最后一根
    labelVisible:
      index === points.length - 1 ||
      (index % 3 === 0 && points.length - 1 - index > 1),
    value: p.books,
    heightPct: 0,
    title: `${shortDate(p.date)}：${p.books} 本 / ${p.chapters} 章`,
  }));
  const total = points.reduce((sum, p) => sum + p.books, 0);
  const max = Math.max(0, ...bars.map((b) => b.value));
  bars.forEach((b) => {
    b.heightPct = pct(b.value, max);
  });
  const days = points.length || 1;
  const note = total > 0
    ? `近 ${days} 天导入 ${total} 本，日均 ${Math.round(total / days)} 本`
    : `近 ${days} 天还没有导入记录`;
  return { empty: total === 0, note, data: { bars, total } };
}

function shortDate(date: string): string {
  const parts = date.split("-");
  if (parts.length !== 3) return date;
  return `${Number(parts[1])}/${Number(parts[2])}`;
}

/* ── 2 章节数分布（6 桶直方图） ── */
export function buildChapterBuckets(buckets: Bucket[]): ChartModel<{ bars: VerticalBar[]; total: number; peak: string }> {
  const bars = buckets.map((b) => ({
    label: b.label,
    labelVisible: true,
    value: b.count,
    heightPct: 0,
    title: `${b.label} 章：${b.count} 本`,
  }));
  const total = buckets.reduce((sum, b) => sum + b.count, 0);
  const max = Math.max(0, ...bars.map((b) => b.value));
  bars.forEach((b) => {
    b.heightPct = pct(b.value, max);
  });
  const peak = bars.reduce<VerticalBar | null>((best, b) => (!best || b.value > best.value ? b : best), null);
  const note =
    total > 0 && peak
      ? `${total} 本书里，${peak.label} 章的最多（${peak.value} 本）`
      : "还没有可统计的书";
  return { empty: total === 0, note, data: { bars, total, peak: peak?.label ?? "" } };
}

/* ── 3 字数 Top 10 作者 ── */
export function buildTopAuthors(
  authors: { name: string; wordCount: number }[],
  totalWords: number,
): ChartModel<{ rows: HorizontalBar[]; sharePct: number }> {
  const top = [...authors].sort((a, b) => b.wordCount - a.wordCount).slice(0, 10);
  const max = top[0]?.wordCount ?? 0;
  const rows = top.map((a) => ({
    name: a.name || "佚名",
    value: a.wordCount,
    widthPct: pct(a.wordCount, max),
    display: compact(a.wordCount),
  }));
  const sum = top.reduce((acc, a) => acc + a.wordCount, 0);
  const sharePct = totalWords > 0 ? Math.round((sum / totalWords) * 100) : 0;
  const note = rows.length
    ? `Top 10 占全库 ${sharePct}% 的字数，${rows[0].name} 以 ${rows[0].display} 字居首`
    : "还没有作者数据";
  return { empty: sum === 0, note, data: { rows, sharePct } };
}

/* ── 4 阅读进度分布（环形） ── */
export interface ReadingDistribution {
  finished: number;
  reading: number;
  unread: number;
  trackable: number;
}

/**
 * 由「最近阅读记录 + 书库总书数」聚合三态。
 * 记录条数达到接口上限时，未读数只能算作「其余的书」，图上会标注这一点，不假装精确。
 */
export function buildReadingDist(
  progress: ReadingProgress[],
  totalBooks: number,
  limitReached = false,
  /** 后端聚合结果：有就以它为准，没有再回退到按最近记录估算 */
  buckets?: { unread: number; reading: number; finished: number } | null,
): ChartModel<ReadingDistribution & { segments: DonutSegment[]; partial: boolean }> {
  if (buckets) {
    const finished = Math.max(0, buckets.finished);
    const reading = Math.max(0, buckets.reading);
    const unread = Math.max(0, buckets.unread);
    const total = finished + reading + unread;
    if (total > 0) {
      const readPct = Math.round(((finished + reading) / total) * 100);
      return {
        empty: false,
        note: `翻过 ${finished + reading} 本，占全库 ${readPct}%${unread > 0 ? `；${unread} 本还没开始` : ""}`,
        data: {
          finished,
          reading,
          unread,
          trackable: finished + reading,
          segments: buildDonutSegments({ finished, reading, unread }),
          partial: false,
        },
      };
    }
  }
  const byBook = new Map<number, ReadingProgress>();
  for (const p of progress) {
    const prev = byBook.get(p.bookId);
    if (!prev || Date.parse(p.updatedAt) > Date.parse(prev.updatedAt)) byBook.set(p.bookId, p);
  }
  const tracked = [...byBook.values()];
  const finished = tracked.filter((p) => p.percent >= 0.999).length;
  const reading = tracked.length - finished;
  const unread = Math.max(0, totalBooks - tracked.length);
  const total = finished + reading + unread;

  const segments = buildDonutSegments({ finished, reading, unread });
  const readPct = total > 0 ? Math.round(((finished + reading) / total) * 100) : 0;
  const note = total > 0
    ? `翻过 ${finished + reading} 本，占全库 ${readPct}%${unread > 0 ? `；${unread} 本还没开始` : ""}`
    : "还没有阅读记录";

  return {
    empty: total === 0,
    note,
    data: { finished, reading, unread, trackable: tracked.length, segments, partial: limitReached },
  };
}

const RADIUS = 52;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;

const DONUT_STYLE: { key: DonutSegment["key"]; label: string; color: string }[] = [
  { key: "finished", label: "已读完", color: "var(--accent)" },
  { key: "reading", label: "在读", color: "color-mix(in oklab, var(--accent), var(--surface) 55%)" },
  { key: "unread", label: "未开始", color: "color-mix(in oklab, var(--muted), var(--surface) 72%)" },
];

function buildDonutSegments(counts: Record<DonutSegment["key"], number>): DonutSegment[] {
  const total = DONUT_STYLE.reduce((sum, s) => sum + counts[s.key], 0);
  let offset = 0;
  return DONUT_STYLE.map((style) => {
    const length = total > 0 ? (counts[style.key] / total) * CIRCUMFERENCE : 0;
    const segment: DonutSegment = {
      key: style.key,
      label: style.label,
      value: counts[style.key],
      color: style.color,
      dash: `${length} ${CIRCUMFERENCE - length}`,
      offset: -offset,
    };
    offset += length;
    return segment;
  });
}

export { RADIUS, CIRCUMFERENCE };

/* ── 5 库体积构成（堆叠条） ── */
export interface StoragePart {
  key: string;
  label: string;
  bytes: number;
  widthPct: number;
  display: string;
  color: string;
}

// 三档颜色都由语义变量派生：夜间主题下 --surface-warm 会变成近底色，所以第三档改用 muted 派生，两套主题都可辨
const STORAGE_STYLE = [
  { key: "text", label: "正文", color: "var(--accent)" },
  { key: "index", label: "书目信息", color: "color-mix(in oklab, var(--accent), var(--surface) 55%)" },
  { key: "other", label: "其他", color: "color-mix(in oklab, var(--muted), var(--surface) 45%)" },
];

export function buildStorage(
  stats: Stats | null,
): ChartModel<{ parts: StoragePart[]; totalBytes: number; compositionPending: boolean }> {
  const raw: Record<string, number> = {
    text: stats?.textBytes ?? 0,
    index: stats?.indexBytes ?? 0,
    other: stats?.otherBytes ?? 0,
  };
  const totalBytes = raw.text + raw.index + raw.other;
  // 占比极小的段（如书目信息 0.1 MB）按真实比例画出来是一根看不见的线，
  // 给它一个最小可见宽度，并在图例里保留真实数值 —— 图形用于分辨比例，数字仍以图例为准。
  const MIN_SEGMENT_PCT = 1.2;
  const raw1 = STORAGE_STYLE.map((style) => (totalBytes > 0 ? (raw[style.key] / totalBytes) * 100 : 0));
  const parts = STORAGE_STYLE.map((style, index) => {
    const bytes = raw[style.key];
    return {
      key: style.key,
      label: style.label,
      bytes,
      widthPct: bytes > 0 ? Math.max(MIN_SEGMENT_PCT, raw1[index]) : 0,
      display: formatStorage(bytes),
      color: style.color,
    };
  });
  // 后端把统计改成查预计算表后，可能出现「总量有了、正文/索引还没拆出来」的中间状态
  // （textBytes=0 且 indexBytes=0 但 otherBytes 有值）。这时不谎报「正文占 0%」，而是说明正在统计。
  const compositionPending = totalBytes > 0 && raw.text === 0 && raw.index === 0;
  const textShare = totalBytes > 0 ? Math.round((raw.text / totalBytes) * 100) : 0;
  const note = totalBytes === 0
    ? "还没有可统计的内容"
    : compositionPending
      ? `全库约 ${formatStorage(totalBytes)}，体积构成正在统计`
      : `全库约 ${formatStorage(totalBytes)}，其中正文占 ${textShare}%`;
  return {
    empty: totalBytes === 0,
    note,
    data: { parts, totalBytes, compositionPending },
  };
}

function formatStorage(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 MB";
  const gb = bytes / 1024 ** 3;
  if (gb >= 1) return `${gb.toFixed(1)} GB`;
  const mb = bytes / 1024 ** 2;
  return `${mb >= 10 ? Math.round(mb) : mb.toFixed(1)} MB`;
}

/* ── 6 章节长度分布 ── */
export function buildChapterLength(buckets: Bucket[]): ChartModel<{ cols: VerticalBar[]; total: number; peak: string }> {
  const cols = buckets.map((b) => ({
    label: b.label,
    labelVisible: true,
    value: b.count,
    heightPct: 0,
    title: `${b.label}：${b.count} 章`,
  }));
  const total = buckets.reduce((sum, b) => sum + b.count, 0);
  const max = Math.max(0, ...cols.map((c) => c.value));
  cols.forEach((c) => {
    c.heightPct = pct(c.value, max, 3);
  });
  const peak = cols.reduce<VerticalBar | null>((best, c) => (!best || c.value > best.value ? c : best), null);
  const peakShare = peak && total > 0 ? Math.round((peak.value / total) * 100) : 0;
  const note = total > 0 && peak
    ? `共 ${compact(total)} 章，${peak.label}最多（${peakShare}%）`
    : "还没有章节数据";
  return { empty: total === 0, note, data: { cols, total, peak: peak?.label ?? "" } };
}