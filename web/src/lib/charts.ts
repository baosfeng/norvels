import type {
  Bucket,
  ImportTrendPoint,
  ReadingProgress,
  RecommendCategoryClicks,
  RecommendHotBook,
  RecommendInterest,
  RecommendInterestDim,
  RecommendReasonMix,
  RecommendStatsDay,
  RecommendStatsSummary,
  Stats,
} from "../api/types";

/**
 * 十二张图的数据转换：只做「后端数值 → 绘图几何 + 一句人话结论」，
 * 不碰 DOM，方便单测，也保证图与结论由同一份数字算出、不会对不上。
 *
 * 前六张是驾驶舱原有的库概览（导入 / 章节 / 作者 / 进度 / 体积 / 章节长度），
 * 中间四张是「推荐与埋点」（事件趋势 / 兴趣画像 / 热门书 / 漏斗），
 * 最后两张是同一区块的分布图（点击理由 / 分类点击）。
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
  /** 悬停/聚焦读数：作者 + 字数 + 占全库比例 */
  title: string;
}

export interface DonutSegment {
  key: "finished" | "reading" | "unread";
  label: string;
  value: number;
  color: string;
  dash: string;
  offset: number;
  /** 悬停/聚焦读数：本数 + 占比 */
  title: string;
}

/** 数字 → 「12.3 万 / 1.2 亿」，图表内联用，与列表页 formatCount 口径一致。 */
export function compact(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0";
  if (value >= 100_000_000) return `${(value / 100_000_000).toFixed(1)} 亿`.replace(".0 ", " ");
  if (value >= 10_000) return `${(value / 10_000).toFixed(1)} 万`.replace(".0 ", " ");
  return String(Math.round(value));
}

/**
 * 占比文案：非零但不足 1% 时写「<1」。
 *
 * 真实库里「在读 2 本 / 838 本」四舍五入是 0%，浮层写「占全库 0%」等于说没有这本书，
 * 所以 0.4% 这类比例统一写成「<1」。
 */
function shareText(value: number, total: number): string {
  if (value <= 0 || total <= 0) return "0";
  const rounded = Math.round((value / total) * 100);
  return rounded === 0 ? "<1" : String(rounded);
}

function pct(value: number, max: number, floor = 2): number {
  // 真正的 0 不画柱：给 0 值留 2px 基线，在「多数天没有导入」的场景会变成一片噪音横线。
  if (value <= 0) return 0;
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
    // 悬停/聚焦读数：没有导入的那天也说清楚「没有」，不留给用户猜；
    // 章数动辄几十万，按「万」量级写，与卡片里其它数字同一口径
    title:
      p.books > 0
        ? `${fullDate(p.date)}：导入 ${compact(p.books)} 本、${compact(p.chapters)} 章`
        : `${fullDate(p.date)}：没有导入`,
  }));
  const total = points.reduce((sum, p) => sum + p.books, 0);
  const max = Math.max(0, ...bars.map((b) => b.value));
  bars.forEach((b) => {
    b.heightPct = pct(b.value, max);
  });
  const days = points.length || 1;
  // 日均只在「每天都有导入」时才说：否则 838 本挤在一天也会被算成「日均 60 本」，
  // 读起来像持续导入，实际是一次性灌库。
  const activeDays = points.filter((p) => p.books > 0).length;
  const note = total > 0
    ? activeDays >= days
      ? `近 ${days} 天导入 ${total} 本，日均 ${Math.round(total / days)} 本`
      : `近 ${days} 天导入 ${total} 本，只有 ${activeDays} 天有导入`
    : `近 ${days} 天还没有导入记录`;
  return { empty: total === 0, note, data: { bars, total } };
}

function shortDate(date: string): string {
  const parts = date.split("-");
  if (parts.length !== 3) return date;
  return `${Number(parts[1])}/${Number(parts[2])}`;
}

/** 读数浮层用全日期（坐标轴空间不够，只能写 10/18）。 */
function fullDate(date: string): string {
  const parts = date.split("-");
  if (parts.length !== 3) return date;
  return `${Number(parts[1])} 月 ${Number(parts[2])} 日`;
}

/* ── 2 章节数分布（6 桶直方图） ── */
export function buildChapterBuckets(buckets: Bucket[]): ChartModel<{ bars: VerticalBar[]; total: number; peak: string }> {
  const bars = buckets.map((b) => ({
    label: b.label,
    labelVisible: true,
    value: b.count,
    heightPct: 0,
    title: b.count > 0 ? `${b.label} 章：${b.count} 本` : `${b.label} 章：没有这样的书`,
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
    title:
      totalWords > 0
        ? `${a.name || "佚名"}：${compact(a.wordCount)} 字，占全库 ${shareText(a.wordCount, totalWords)}%`
        : `${a.name || "佚名"}：${compact(a.wordCount)} 字`,
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
      const readPct = shareText(finished + reading, total);
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
  const readPct = total > 0 ? shareText(finished + reading, total) : "0";
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
      title: `${style.label} ${counts[style.key]} 本${total > 0 ? `，占全库 ${shareText(counts[style.key], total)}%` : ""}`,
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
  /** 真实占比（未做最小宽度处理），用于说明口径与自检 */
  sharePct: number;
  /** 渲染宽度：三段之和恒为 100，堆叠条既不溢出也不留缝 */
  widthPct: number;
  display: string;
  color: string;
  /** 悬停/聚焦读数：档位 + 体积 + 占比 */
  title: string;
}

export interface StorageData {
  parts: StoragePart[];
  totalBytes: number;
  compositionPending: boolean;
  /** 三段真实占比之和（有数据时应为 100） */
  shareSum: number;
  /** 三段渲染宽度之和（恒为 100，无数据时为 0） */
  widthSum: number;
  /** 正文压缩率（压缩后 / 原始，整数百分比）；没有原始字节时为 null */
  compressionPct: number | null;
  /** 压缩省下的字节 */
  savedBytes: number;
}

// 三档颜色都由语义变量派生：夜间主题下 --surface-warm 会变成近底色，所以第三档改用 muted 派生，两套主题都可辨
const STORAGE_STYLE = [
  { key: "text", label: "正文", color: "var(--accent)" },
  { key: "index", label: "书目信息", color: "color-mix(in oklab, var(--accent), var(--surface) 55%)" },
  { key: "other", label: "其他", color: "color-mix(in oklab, var(--muted), var(--surface) 45%)" },
];

/** 占比极小的段（如书目信息 0.1 MB）按真实比例画出来是一根看不见的线，给它一个最小可见宽度。 */
const MIN_SEGMENT_PCT = 1.2;

/**
 * 库体积构成（堆叠条）。
 *
 * 口径：三段之和 = dbBytes（后端按实际文件大小兜底过）。正文是**压缩后**的实际占用，
 * 所以这里直接用后端的三段值算占比，不再自己推导 —— 少一次推导就少一次对不上的机会。
 * 压缩前后的对比走 textBytesRaw，只用于那句「省了多少」，不参与百分比。
 */
export function buildStorage(stats: Stats | null): ChartModel<StorageData> {
  const raw: Record<string, number> = {
    text: stats?.textBytes ?? 0,
    index: stats?.indexBytes ?? 0,
    // 后端保证 otherBytes 不为负；这里再钳一次，避免脏数据把堆叠条画成负宽度
    other: Math.max(0, stats?.otherBytes ?? 0),
  };
  const totalBytes = raw.text + raw.index + raw.other;
  const sharePct = STORAGE_STYLE.map((style) => (totalBytes > 0 ? (raw[style.key] / totalBytes) * 100 : 0));
  const widthPct = normalizeWidths(
    sharePct.map((pct, i) => (raw[STORAGE_STYLE[i].key] > 0 ? Math.max(MIN_SEGMENT_PCT, pct) : 0)),
  );
  const parts = STORAGE_STYLE.map((style, index) => ({
    key: style.key,
    label: style.label,
    bytes: raw[style.key],
    sharePct: sharePct[index],
    widthPct: widthPct[index],
    display: formatStorage(raw[style.key]),
    color: style.color,
    title: `${style.label} ${formatStorage(raw[style.key])}，占 ${sharePct[index].toFixed(1)}%`,
  }));

  // 后端把统计改成查预计算表后，可能出现「总量有了、正文/索引还没拆出来」的中间状态
  // （textBytes=0 且 indexBytes=0 但 otherBytes 有值）。这时不谎报「正文占 0%」，而是说明正在统计。
  const compositionPending = totalBytes > 0 && raw.text === 0 && raw.index === 0;
  const textShare = totalBytes > 0 ? Math.round((raw.text / totalBytes) * 100) : 0;
  const note = totalBytes === 0
    ? "还没有可统计的内容"
    : compositionPending
      ? `全库约 ${formatStorage(totalBytes)}，体积构成正在统计`
      : `全库约 ${formatStorage(totalBytes)}，其中正文占 ${textShare}%`;

  const rawText = stats?.textBytesRaw ?? 0;
  // 保留一位小数：真实库实测是 34.7%，取整会显示成 35%，与「压缩率」这个数对不上。
  // 一位小数足够表达「省了多少」，也不会把 34.7 说成 35 这种明显错位的数。
  const compressionPct = rawText > 0 && raw.text > 0 ? Math.round((raw.text / rawText) * 1000) / 10 : null;
  const savedBytes = compressionPct === null ? 0 : Math.max(0, rawText - raw.text);

  return {
    empty: totalBytes === 0,
    note,
    data: {
      parts,
      totalBytes,
      compositionPending,
      shareSum: sharePct.reduce((sum, pct) => sum + pct, 0),
      widthSum: widthPct.reduce((sum, pct) => sum + pct, 0),
      compressionPct,
      savedBytes,
    },
  };
}

/**
 * 把一组宽度等比缩放到总和精确等于 100：堆叠条永远铺满，不溢出也不留缝。
 *
 * 两步：① 等比归一；② 保留 4 位小数，并把舍入残差补到最后一段非零段上。
 * 不补残差的话，写进 DOM 的三个百分比相加会是 99.9999 或 100.0001，
 * 「三段之和 = 100%」这句话就不再是照着 DOM 能验证的事实。
 */
function normalizeWidths(widths: number[]): number[] {
  const total = widths.reduce((sum, w) => sum + w, 0);
  if (total <= 0) return widths;
  const rounded = widths.map((w) => Math.round((w / total) * 10_000) / 100);
  const lastPositive = rounded.reduce((last, w, i) => (w > 0 ? i : last), -1);
  if (lastPositive < 0) return rounded;
  const others = rounded.reduce((sum, w, i) => (i === lastPositive ? sum : sum + w), 0);
  rounded[lastPositive] = Math.round((100 - others) * 100) / 100;
  return rounded;
}

/**
 * 压缩率那句话：用户关心的是「省了多少」，不是算法细节。
 *
 * 刻意不复述正文的绝对体积——那一行紧挨着的图例里已经有了（「正文 1.7 GB」），
 * 同一张卡片里同一个数字出现两次属于重复信息。这里只给两个新信息：比例、省下的量。
 */
export function storageRatioNote(data: StorageData): string {
  if (data.compressionPct === null || data.compressionPct >= 100) return "";
  if (data.savedBytes <= 0) return "";
  return `正文约为原始文字的 ${data.compressionPct}%，比不压缩省下 ${formatStorage(data.savedBytes)}`;
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
    title: "",
  }));
  const total = buckets.reduce((sum, b) => sum + b.count, 0);
  const max = Math.max(0, ...cols.map((c) => c.value));
  // 占比要等 total 算完才填：读数是「本桶章数 + 占全部章节的比例」
  cols.forEach((c) => {
    c.heightPct = pct(c.value, max, 3);
    c.title = c.value > 0
      ? `${c.label}：${compact(c.value)} 章，占 ${shareText(c.value, total)}%`
      : `${c.label}：没有这样的章节`;
  });
  const peak = cols.reduce<VerticalBar | null>((best, c) => (!best || c.value > best.value ? c : best), null);
  const peakShare = peak && total > 0 ? shareText(peak.value, total) : "0";
  const note = total > 0 && peak
    ? `共 ${compact(total)} 章，${peak.label}最多（${peakShare}%）`
    : "还没有章节数据";
  return { empty: total === 0, note, data: { cols, total, peak: peak?.label ?? "" } };
}

/* ── 7 推荐与埋点：近 N 天事件趋势（三色柱） ── */

/**
 * 百分比人话：非零但不足 1% 写「<1%」。
 * 与 shareText 同一口径（那里写「<1」不带符号，因为它后面还跟别的字）。
 */
function rateText(part: number, total: number): string {
  if (part <= 0 || total <= 0) return "0%";
  const rounded = Math.round((part / total) * 100);
  return rounded === 0 ? "<1%" : `${rounded}%`;
}

/** 秒数 → 「6 分钟」；不足 1 分钟但有读数时写「<1 分钟」。 */
function minutesText(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "0 分钟";
  const minutes = Math.round(seconds / 60);
  return minutes < 1 ? "<1 分钟" : `${minutes} 分钟`;
}

export interface TrendDay {
  date: string;
  label: string;
  labelVisible: boolean;
  impressions: number;
  clicks: number;
  reads: number;
  /** 三色柱各自按自身峰值归一：曝光动辄上千、阅读个位，同一把尺子会把点击与阅读压成一条线 */
  impressionPct: number;
  clickPct: number;
  readPct: number;
  /** 浮层读数：当天三个绝对次数 + 当天点击率 */
  title: string;
}

export interface EventTrendData {
  days: TrendDay[];
  impressions: number;
  clicks: number;
  reads: number;
  /** 窗口整体点击率（点击 / 曝光），人话文本 */
  ctr: string;
  readMinutes: number;
}

/**
 * 事件趋势：每天三根柱（曝光 / 点击 / 阅读）。
 *
 * 归一化口径写在卡片里（「三色柱各自按自身峰值归一」）：曝光与阅读差两三个数量级，
 * 共用一把尺子时点击 / 阅读会趴在 0 上，图就没有信息量了；绝对次数由浮层给出。
 */
export function buildEventTrend(daily: RecommendStatsDay[]): ChartModel<EventTrendData> {
  const points = [...daily].sort((a, b) => a.date.localeCompare(b.date));
  const peak = {
    impressions: Math.max(0, ...points.map((p) => p.impressions)),
    clicks: Math.max(0, ...points.map((p) => p.clicks)),
    reads: Math.max(0, ...points.map((p) => p.reads)),
  };
  const days: TrendDay[] = points.map((p, index) => ({
    date: p.date,
    label: shortDate(p.date),
    // 与导入趋势同一口径：每 3 根标一次 + 最后一根，倒数第二根让位给最后一根
    labelVisible: index === points.length - 1 || (index % 3 === 0 && points.length - 1 - index > 1),
    impressions: p.impressions,
    clicks: p.clicks,
    reads: p.reads,
    impressionPct: pct(p.impressions, peak.impressions),
    clickPct: pct(p.clicks, peak.clicks),
    readPct: pct(p.reads, peak.reads),
    title: `${fullDate(p.date)}：曝光 ${compact(p.impressions)}、点击 ${compact(p.clicks)}、阅读 ${compact(p.reads)}${p.seconds > 0 ? `，读了 ${minutesText(p.seconds)}` : ""}${p.impressions > 0 ? `（点击率 ${rateText(p.clicks, p.impressions)}）` : ""}`,
  }));

  const impressions = points.reduce((sum, p) => sum + p.impressions, 0);
  const clicks = points.reduce((sum, p) => sum + p.clicks, 0);
  const reads = points.reduce((sum, p) => sum + p.reads, 0);
  const seconds = points.reduce((sum, p) => sum + p.seconds, 0);
  const ctr = rateText(clicks, impressions);
  const readMinutes = Math.round(seconds / 60);
  const note =
    points.length === 0
      ? "还没有埋点数据"
      : `${points.length} 天曝光 ${compact(impressions)}、点击 ${compact(clicks)}（点击率 ${ctr}）、读了 ${minutesText(seconds)}`;
  return {
    empty: points.length === 0 || impressions + clicks + reads === 0,
    note,
    data: { days, impressions, clicks, reads, ctr, readMinutes },
  };
}

/* ── 8 推荐与埋点：兴趣画像 ── */

export interface InterestGroup {
  dim: RecommendInterestDim;
  /** 组标题（中文短标签：「分类·大」「分类·小」「作者」） */
  label: string;
  rows: HorizontalBar[];
}

/**
 * 三个维度的固定展示顺序与组标题。
 *
 * 顺序是「大分类 → 小分类 → 作者」：先看他的口味大类，再看细分，最后看人。
 * 三组的权重不在同一尺度上（作者权重来自作者维度累计，分类来自分类维度），
 * 所以各自的条按**组内峰值**归一，组间不可比；这一点由组标题分开表达，不混榜。
 */
const INTEREST_GROUPS: { dim: RecommendInterestDim; label: string; noun: string }[] = [
  { dim: "category_l1", label: "分类·大", noun: "大分类" },
  { dim: "category_l2", label: "分类·小", noun: "小分类" },
  { dim: "author", label: "作者", noun: "作者" },
];

/**
 * 兴趣画像：大分类 / 小分类 / 作者三组，各自排序、各自归一、各自取 top N。
 *
 * 展示名取 `name ?? key`：后端给名字（作者名 / 分类名）就用名字，没给或给了空串
 * （作者被删、库还是旧版）就退回 key —— key 是稳定标识（作者 id 的十进制文本），
 * 所以「名字缺了」不会让这一行消失或显示成一串空白。
 * 某个维度没有数据时该组整组不渲染（空组不占位置）；不认识的 dim 一律忽略。
 */
export function buildInterestBars(
  interests: RecommendInterest[],
  limit = 5,
): ChartModel<{ groups: InterestGroup[]; top: string }> {
  const groups: InterestGroup[] = INTEREST_GROUPS.map(({ dim, label }) => {
    const rows = interests
      .filter((item) => item.dim === dim)
      .filter((item) => item.weight > 0 && item.key.trim() !== "")
      .sort((a, b) => b.weight - a.weight)
      .slice(0, limit);
    const max = rows[0]?.weight ?? 0;
    return {
      dim,
      label,
      rows: rows.map<HorizontalBar>((row) => {
        const name = (row.name ?? "").trim() || row.key;
        const group = INTEREST_GROUPS.find((group) => group.dim === dim);
        return {
          name,
          value: row.weight,
          widthPct: pct(row.weight, max, 4),
          display: row.weight.toFixed(1),
          title:
            dim === "author"
              ? `作者 ${name}：兴趣 ${row.weight.toFixed(1)}`
              : `${group?.noun ?? "分类"}「${name}」：兴趣 ${row.weight.toFixed(1)}`,
        };
      }),
    };
  }).filter((group) => group.rows.length > 0);

  const best = groups
    .flatMap((group) => group.rows.map((row) => ({ dim: group.dim, label: group.label, row })))
    .reduce<{ dim: InterestGroup["dim"]; label: string; row: HorizontalBar } | null>(
      (top, item) => (!top || item.row.value > top.row.value ? item : top),
      null,
    );
  // 三组并排时「最重的那个」要说清它在哪一组：分类名可能同名出现在大分类与小分类里
  const note = best
    ? best.dim === "author"
      ? `最重的是作者「${best.row.name}」（兴趣 ${best.row.display}）`
      : `最重的是「${best.row.name}」（${best.label}，兴趣 ${best.row.display}）`
    : "还没有形成兴趣画像：点击、阅读几本之后这里会填上分类与作者";

  return { empty: groups.length === 0, note, data: { groups, top: best?.row.name ?? "" } };
}

/* ── 9 推荐与埋点：热门书 Top N（横条） ── */

/**
 * 热门书打分：`读过 1 次 = 2 分，点击 1 次 = 1 分`。
 *
 * 权重复用设计 §2 的「读比点值钱」：点开只是有兴趣，读完才算真读过。
 * 只看点击会把「点开就关」的书排到第一，只看阅读又会让刚上线的书永远上不了榜。
 */
export const HOT_READ_WEIGHT = 2;

export function buildHotBooks(
  books: RecommendHotBook[],
  limit = 10,
): ChartModel<{ rows: HorizontalBar[]; top: string; total: number }> {
  const scored = books
    .map((book) => ({ ...book, score: book.reads * HOT_READ_WEIGHT + book.clicks }))
    .filter((book) => book.score > 0)
    .sort((a, b) => b.score - a.score || b.reads - a.reads || a.bookId - b.bookId)
    .slice(0, limit);
  const max = scored[0]?.score ?? 0;
  const total = scored.reduce((sum, book) => sum + book.score, 0);
  const rows = scored.map<HorizontalBar>((book) => ({
    name: book.title || "未命名",
    value: book.score,
    widthPct: pct(book.score, max, 4),
    display: `${compact(book.reads)} 读 · ${compact(book.clicks)} 点`,
    title: `《${book.title}》：${book.reads} 次阅读、${book.clicks} 次点击、读了 ${minutesText(book.seconds)}（热度占 ${shareText(book.score, total)}%）`,
  }));
  const first = scored[0];
  const note = first
    ? `最热的是《${first.title}》：${first.reads} 次阅读、${first.clicks} 次点击`
    : "还没有阅读或点击记录";
  return { empty: rows.length === 0, note, data: { rows, top: rows[0]?.name ?? "", total } };
}

/* ── 10 推荐与埋点：曝光 → 点击 → 阅读 → 收藏（漏斗） ── */

export interface FunnelStep {
  key: "impression" | "click" | "read" | "shelf";
  label: string;
  value: number;
  /** 相对第一级（曝光）的宽度百分比：漏斗要能一眼看出「越往下越窄」 */
  widthPct: number;
  /** 相对上一级的转化率；第一级写「起始」 */
  stepRate: string;
  title: string;
}

export interface FunnelData {
  steps: FunnelStep[];
  /** 曝光 → 收藏的整体转化率 */
  overall: string;
}

/** 漏斗最小可见宽度：0.4% 的收藏按真实比例画是一条看不见的线。 */
const FUNNEL_MIN_PCT = 2;

export function buildFunnel(summary: RecommendStatsSummary): ChartModel<FunnelData> {
  const levels: { key: FunnelStep["key"]; label: string; value: number }[] = [
    { key: "impression", label: "曝光", value: Math.max(0, summary.impressions) },
    { key: "click", label: "点击", value: Math.max(0, summary.clicks) },
    { key: "read", label: "阅读", value: Math.max(0, summary.reads) },
    { key: "shelf", label: "收藏", value: Math.max(0, summary.shelfAdds) },
  ];
  const base = levels[0].value;
  const steps = levels.map<FunnelStep>((level, index) => {
    const previous = index === 0 ? null : levels[index - 1].value;
    const stepRate = previous === null ? "起始" : rateText(level.value, previous);
    return {
      ...level,
      widthPct: level.value > 0 ? Math.max(FUNNEL_MIN_PCT, pct(level.value, base)) : 0,
      stepRate,
      title:
        previous === null
          ? `${level.label} ${compact(level.value)} 次（漏斗起点）`
          : `${level.label} ${compact(level.value)} 次，相对${levels[index - 1].label} ${stepRate}`,
    };
  });
  const overall = rateText(summary.shelfAdds, summary.impressions);
  const note =
    base > 0
      ? `曝光 ${compact(base)} → 收藏 ${compact(summary.shelfAdds)}，整体转化 ${overall}`
      : "还没有曝光记录";
  return {
    empty: levels.every((level) => level.value === 0),
    note,
    data: { steps, overall },
  };
}

/* ── 指标卡行（区块顶部四张卡，数字与上面四张图同源） ── */

export interface RecommendMetric {
  key: "impressions" | "ctr" | "reads" | "shelf";
  label: string;
  value: string;
  hint: string;
}

/**
 * 顶部四个指标：今日曝光 / 点击率 / 阅读分钟 / 收藏。
 *
 * 「今日」按 **UTC 天**匹配（后端 daily 的日期就是 UTC 的 YYYY-MM-DD，见 internal/recstore）；
 * 匹配不到（窗口里没有今天）就退回到窗口内最近的一天，绝不显示一个凭空算出的 0。
 */
export function buildRecommendMetrics(
  summary: RecommendStatsSummary,
  daily: RecommendStatsDay[],
  today: string,
): { metrics: RecommendMetric[]; todayImpressions: number } {
  const sorted = [...daily].sort((a, b) => a.date.localeCompare(b.date));
  const todayRow = sorted.find((day) => day.date === today) ?? sorted[sorted.length - 1] ?? null;
  const todayImpressions = Math.max(0, todayRow?.impressions ?? 0);
  const days = summary.days || sorted.length;
  return {
    todayImpressions,
    metrics: [
      { key: "impressions", label: "今日曝光", value: compact(todayImpressions), hint: "次" },
      { key: "ctr", label: "点击率", value: rateText(summary.clicks, summary.impressions), hint: "点击 / 曝光" },
      {
        key: "reads",
        label: "阅读分钟",
        value: compact(Math.round((summary.readSeconds ?? 0) / 60)),
        hint: `近 ${days} 天`,
      },
      { key: "shelf", label: "收藏", value: compact(summary.shelfAdds), hint: "加入书架" },
    ],
  };
}

/* ── 11 推荐与埋点：点击理由分布 ── */

/**
 * 点击理由分布：哪个推荐理由真的把人点进来了。
 *
 * 条长按点击量（「有效」的口径）；有曝光时浮层额外给点击率——
 * 只看点击会偏爱「曝光本来就多」的理由，点击率才说明理由本身的说服力。
 */
export function buildReasonMix(
  rows: RecommendReasonMix[],
  limit = 10,
): ChartModel<{ rows: HorizontalBar[]; top: string; totalClicks: number }> {
  const sorted = rows
    .filter((row) => row.clicks > 0 && row.reason.trim() !== "")
    .sort((a, b) => b.clicks - a.clicks)
    .slice(0, limit);
  const max = sorted[0]?.clicks ?? 0;
  const totalClicks = sorted.reduce((sum, row) => sum + row.clicks, 0);
  const bars = sorted.map<HorizontalBar>((row) => {
    const impressions = row.impressions ?? 0;
    // 分组键是 code，展示文案取 label（旧服务端没有 label 时回落到 code 本身）。
    const name = row.label ?? row.reason;
    return {
      name,
      value: row.clicks,
      widthPct: pct(row.clicks, max, 4),
      display: `${compact(row.clicks)} 点`,
      title:
        impressions > 0
          ? `「${name}」：点击 ${row.clicks} / 曝光 ${impressions}（点击率 ${rateText(row.clicks, impressions)}）`
          : `「${name}」：点击 ${row.clicks} 次`,
    };
  });
  const first = sorted[0];
  const firstName = first ? (first.label ?? first.reason) : "";
  const note = first
    ? `最抓人的是「${firstName}」：${first.clicks} 次点击${first.impressions ? `（点击率 ${rateText(first.clicks, first.impressions)}）` : ""}`
    : "还没有带理由的点击记录";
  return { empty: bars.length === 0, note, data: { rows: bars, top: bars[0]?.name ?? "", totalClicks } };
}

/* ── 12 推荐与埋点：分类点击分布 ── */

/** 分类点击分布：点击都落在哪些大分类上（空串归到「未分类」）。 */
export function buildCategoryClicks(
  rows: RecommendCategoryClicks[],
  limit = 10,
): ChartModel<{ rows: HorizontalBar[]; top: string; totalClicks: number }> {
  const sorted = rows
    .filter((row) => row.clicks > 0)
    .map((row) => ({ ...row, label: row.categoryL1.trim() || "未分类" }))
    .sort((a, b) => b.clicks - a.clicks)
    .slice(0, limit);
  const max = sorted[0]?.clicks ?? 0;
  const totalClicks = sorted.reduce((sum, row) => sum + row.clicks, 0);
  const bars = sorted.map<HorizontalBar>((row) => ({
    name: row.label,
    value: row.clicks,
    widthPct: pct(row.clicks, max, 4),
    display: `${compact(row.clicks)} 点`,
    title: `${row.label}：${row.clicks} 次点击，占 ${shareText(row.clicks, totalClicks)}%`,
  }));
  const first = sorted[0];
  const note = first
    ? `点得最多的分类是「${first.label}」：${first.clicks} 次，占 ${shareText(first.clicks, totalClicks)}%`
    : "还没有分类维度的点击记录";
  return { empty: bars.length === 0, note, data: { rows: bars, top: bars[0]?.name ?? "", totalClicks } };
}

