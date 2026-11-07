import { formatDurationShort } from "./format";
import type {
  Bucket,
  DaySeconds,
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
  value: number;
  title: string;
}

export interface HorizontalBar {
  name: string;
  value: number;
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

export interface ChartModel<T> {
  empty: boolean;
  note: string;
  data: T;
}

/* ── 1 近 14 天导入量 ── */
export function buildImportTrend(points: ImportTrendPoint[]): ChartModel<{ bars: VerticalBar[]; total: number }> {
  const bars = points.map((p) => ({
    label: shortDate(p.date),
    value: p.books,
    // 悬停/聚焦读数：没有导入的那天也说清楚「没有」，不留给用户猜；
    // 章数动辄几十万，按「万」量级写，与卡片里其它数字同一口径
    title:
      p.books > 0
        ? `${fullDate(p.date)}：导入 ${compact(p.books)} 本、${compact(p.chapters)} 章`
        : `${fullDate(p.date)}：没有导入`,
  }));
  const total = points.reduce((sum, p) => sum + p.books, 0);
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
    value: b.count,
    title: b.count > 0 ? `${b.label} 章：${b.count} 本` : `${b.label} 章：没有这样的书`,
  }));
  const total = buckets.reduce((sum, b) => sum + b.count, 0);
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
  const rows = top.map((a) => ({
    name: a.name || "佚名",
    value: a.wordCount,
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
  /** 真实占比：段宽就是它的长度，不做最小宽度归一 */
  sharePct: number;
  /** 占比文案：非零但不足 1% 写「<1%」 */
  pctText: string;
  display: string;
  color: string;
  /** 段内文字颜色（正文段是深底，配浅字） */
  labelColor: string;
  /** 悬停/聚焦读数：档位 + 体积 + 占比 */
  title: string;
}

export interface StorageData {
  parts: StoragePart[];
  totalBytes: number;
  compositionPending: boolean;
  /** 三段真实占比之和（有数据时应为 100） */
  shareSum: number;
  /** 正文压缩率（压缩后 / 原始，整数百分比）；没有原始字节时为 null */
  compressionPct: number | null;
  /** 压缩省下的字节 */
  savedBytes: number;
}

// 三档颜色都由语义变量派生：夜间主题下 --surface-warm 会变成近底色，所以第三档改用 muted 派生，两套主题都可辨
const STORAGE_STYLE = [
  { key: "text", label: "正文", color: "var(--accent)", labelColor: "var(--accent-on)" },
  { key: "index", label: "书目信息", color: "color-mix(in oklab, var(--accent), var(--surface) 55%)", labelColor: "var(--fg)" },
  { key: "other", label: "其他", color: "color-mix(in oklab, var(--muted), var(--surface) 45%)", labelColor: "var(--fg)" },
];

/**
 * 占比文案：非零但不足 1% 写「<1%」。
 *
 * 段宽按真实占比画（0.1% 的「其他」就是一条 0.7px 的细线），所以「这一段多小」只能由文案说清：
 * 四舍五入成 0% 等于说没有这一段，写成 1% 又虚报十倍，两种都不如「<1%」诚实。
 */
export function sharePctText(pct: number): string {
  if (!Number.isFinite(pct) || pct <= 0) return "0%";
  if (pct < 1) return "<1%";
  return `${pct >= 10 ? Math.round(pct) : Math.round(pct * 10) / 10}%`;
}

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
  const parts = STORAGE_STYLE.map((style, index) => ({
    key: style.key,
    label: style.label,
    bytes: raw[style.key],
    sharePct: sharePct[index],
    pctText: sharePctText(sharePct[index]),
    display: formatStorage(raw[style.key]),
    color: style.color,
    labelColor: style.labelColor,
    title: `${style.label} ${formatStorage(raw[style.key])}，占 ${sharePctText(sharePct[index])}`,
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
      compressionPct,
      savedBytes,
    },
  };
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
    value: b.count,
    title: "",
  }));
  const total = buckets.reduce((sum, b) => sum + b.count, 0);
  // 占比要等 total 算完才填：读数是「本桶章数 + 占全部章节的比例」
  cols.forEach((c) => {
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

/**
 * 级间进度文案：下一级不超过上一级时给转化率（百分比），超过时改给人均次数。
 *
 * 阅读次数可以高于点击次数（同一本书点进去读几次都算多次进入阅读），
 * 这时拿点击做分母会算出 12800% 这种读不懂的数。改写成「每点击 128 次」，
 * 意思与「1 次点击对应 128 次阅读」完全一样，且不会把 100% 当上限骗人。
 * 上一级是 0 而这一级有值时不编比例（除零），给「—」，说明写在浮层里。
 */
export function stepText(current: number, previous: number, previousLabel: string): string {
  if (!(previous > 0) && current > 0) return "—";
  if (previous > 0 && current > previous) {
    const times = current / previous;
    const shown = times >= 10 ? String(Math.round(times)) : (Math.round(times * 10) / 10).toFixed(1);
    return `每${previousLabel} ${shown} 次`;
  }
  return rateText(current, previous);
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
  impressions: number;
  clicks: number;
  reads: number;
  /** 当天阅读时长（分钟，四舍五入）：与曝光 / 点击不同量纲，画在下半面 */
  minutes: number;
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
  const days: TrendDay[] = points.map((p) => ({
    date: p.date,
    label: shortDate(p.date),
    impressions: p.impressions,
    clicks: p.clicks,
    reads: p.reads,
    minutes: Math.round((p.seconds ?? 0) / 60),
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
  /** 组标题（中文短标签：「分类·大」「分类·小」「作者」「标签」「系列」） */
  label: string;
  rows: HorizontalBar[];
}

/**
 * 兴趣维度的展示表：固定顺序 + 组标题 + 浮层用词。
 *
 * 顺序是大分类 → 小分类 → 作者 → 标签 → 系列（与后端 `interests` 的 dim 取值、文档同序）：
 * 先看口味大类，再看细分，再看人，最后看内容自身的标记。
 * 各组的权重不在同一尺度上（作者权重来自作者维度累计、分类来自分类维度、标签/系列同理），
 * 所以条按**组内峰值**归一，组间不可比；这一点由组标题分开表达，不混榜。
 *
 * `titleStyle` 决定浮层怎么念这一行：作者维度的 name 是「id → 作者名」的翻译，
 * 念成「作者 演示作者己」；其余维度的 name 就是 key（分类名 / 标签名 / 系列名是同一个串），
 * 念成「标签「武侠」」——套用作者那种句式会变成「作者 武侠」这种错误归属。
 */
interface InterestGroupSpec {
  dim: RecommendInterestDim;
  label: string;
  noun: string;
  titleStyle: "bare" | "quoted";
}

const INTEREST_GROUPS: InterestGroupSpec[] = [
  { dim: "category_l1", label: "分类·大", noun: "大分类", titleStyle: "quoted" },
  { dim: "category_l2", label: "分类·小", noun: "小分类", titleStyle: "quoted" },
  { dim: "author", label: "作者", noun: "作者", titleStyle: "bare" },
  { dim: "tag", label: "标签", noun: "标签", titleStyle: "quoted" },
  { dim: "series", label: "系列", noun: "系列", titleStyle: "quoted" },
];

/** 浮层文案：有翻译名的维度（作者）不加引号，其余维度「名词「名字」」。 */
function interestTitle(spec: InterestGroupSpec, name: string, weight: string): string {
  return spec.titleStyle === "bare"
    ? `${spec.noun} ${name}：兴趣 ${weight}`
    : `${spec.noun}「${name}」：兴趣 ${weight}`;
}

/** 结论里说「最重的那个」：作者维度念「最重的是作者「X」」，其余维度说清它在哪一组。 */
function interestTopNote(spec: InterestGroupSpec, name: string, weight: string): string {
  return spec.titleStyle === "bare"
    ? `最重的是${spec.noun}「${name}」（兴趣 ${weight}）`
    : `最重的是「${name}」（${spec.label}，兴趣 ${weight}）`;
}

/**
 * 兴趣画像：大分类 / 小分类 / 作者 / 标签 / 系列五组，各自排序、各自归一、各自取 top N。
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
  const groups: InterestGroup[] = INTEREST_GROUPS.map((spec) => {
    const rows = interests
      .filter((item) => item.dim === spec.dim)
      .filter((item) => item.weight > 0 && item.key.trim() !== "")
      .sort((a, b) => b.weight - a.weight)
      .slice(0, limit);
    return {
      dim: spec.dim,
      label: spec.label,
      rows: rows.map<HorizontalBar>((row) => {
        const name = (row.name ?? "").trim() || row.key;
        const display = row.weight.toFixed(1);
        return {
          name,
          value: row.weight,
          display,
          title: interestTitle(spec, name, display),
        };
      }),
    };
  }).filter((group) => group.rows.length > 0);

  const best = groups
    .flatMap((group) => {
      const spec = INTEREST_GROUPS.find((item) => item.dim === group.dim);
      return spec ? group.rows.map((row) => ({ spec, row })) : [];
    })
    .reduce<{ spec: InterestGroupSpec; row: HorizontalBar } | null>(
      (top, item) => (!top || item.row.value > top.row.value ? item : top),
      null,
    );
  // 多组并排时「最重的那个」要说清它在哪一组：分类名可能同名出现在大分类与小分类里
  const note = best
    ? interestTopNote(best.spec, best.row.name, best.row.display)
    : "还没有形成兴趣画像：点击、阅读几本之后这里会填上分类、作者、标签与系列";

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
  const total = scored.reduce((sum, book) => sum + book.score, 0);
  const rows = scored.map<HorizontalBar>((book) => ({
    name: book.title || "未命名",
    value: book.score,
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
  /** 相对上一级的转化率；第一级写「起始」；下一级超过上一级时改成人均次数（每点击 1.3 次） */
  stepRate: string;
  title: string;
}

export interface FunnelData {
  steps: FunnelStep[];
  /** 曝光 → 收藏的整体转化率 */
  overall: string;
}

/**
 * 漏斗浮层文案：百分比时写「相对点击 33%」，超过 100% 时直接复用格子里那句
 *「每点击 128 次」——同一件事在两处用同一句话，读起来才不会自相矛盾。
 */
function funnelTitle(
  level: { label: string; value: number },
  previous: { label: string; value: number } | null,
  stepRate: string,
): string {
  const value = `${level.label} ${compact(level.value)} 次`;
  if (previous === null) return `${value}（漏斗起点）`;
  if (previous.value > 0 && level.value > previous.value) return `${value}，${stepRate}`;
  return `${value}，相对${previous.label} ${stepRate}`;
}

export function buildFunnel(summary: RecommendStatsSummary): ChartModel<FunnelData> {
  const levels: { key: FunnelStep["key"]; label: string; value: number }[] = [
    { key: "impression", label: "曝光", value: Math.max(0, summary.impressions) },
    { key: "click", label: "点击", value: Math.max(0, summary.clicks) },
    { key: "read", label: "阅读", value: Math.max(0, summary.reads) },
    { key: "shelf", label: "收藏", value: Math.max(0, summary.shelfAdds) },
  ];
  const base = levels[0].value;
  const steps = levels.map<FunnelStep>((level, index) => {
    const previous = index === 0 ? null : levels[index - 1];
    const stepRate = previous === null ? "起始" : stepText(level.value, previous.value, previous.label);
    return {
      ...level,
      stepRate,
      title: funnelTitle(level, previous, stepRate),
    };
  });
  const overall = stepText(summary.shelfAdds, summary.impressions, "曝光");
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
  const totalClicks = sorted.reduce((sum, row) => sum + row.clicks, 0);
  const bars = sorted.map<HorizontalBar>((row) => {
    const impressions = row.impressions ?? 0;
    // 分组键是 code，展示文案取 label（旧服务端没有 label 时回落到 code 本身）。
    const name = row.label ?? row.reason;
    return {
      name,
      value: row.clicks,
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
  const totalClicks = sorted.reduce((sum, row) => sum + row.clicks, 0);
  const bars = sorted.map<HorizontalBar>((row) => ({
    name: row.label,
    value: row.clicks,
    display: `${compact(row.clicks)} 点`,
    title: `${row.label}：${row.clicks} 次点击，占 ${shareText(row.clicks, totalClicks)}%`,
  }));
  const first = sorted[0];
  const note = first
    ? `点得最多的分类是「${first.label}」：${first.clicks} 次，占 ${shareText(first.clicks, totalClicks)}%`
    : "还没有分类维度的点击记录";
  return { empty: bars.length === 0, note, data: { rows: bars, top: bars[0]?.name ?? "", totalClicks } };
}

/* ── 13 近 14 天阅读时长（折线）：与热力图同一份 /api/stats/reading 的 days ── */

export interface ReadingTrendPoint {
  label: string;
  /** 当天阅读分钟数（四舍五入）：折线的纵轴单位就是分钟 */
  value: number;
  /** 悬停读数：日期 + 时长 */
  title: string;
  date: string;
}

/**
 * 近 N 天阅读时长：按天出点，缺数据的日子补 0（不跳过，否则 14 天会被画成 8 个点）。
 *
 * 纵轴用分钟而不是秒：一天读 2 小时是 120，读 3 分钟是 3，刻度读得出来；
 * 秒数会让刻度变成 3600 / 7200 这种只能靠 hover 才看懂的数。
 */
export function buildReadingTrend(
  days: DaySeconds[],
  limit = 14,
): ChartModel<{ points: ReadingTrendPoint[]; totalSeconds: number; activeDays: number; peak: ReadingTrendPoint | null }> {
  const sorted = [...days].sort((a, b) => a.date.localeCompare(b.date)).slice(-Math.max(1, limit));
  const points = sorted.map<ReadingTrendPoint>((day) => ({
    date: day.date,
    label: shortDate(day.date),
    value: Math.max(0, Math.round((day.seconds ?? 0) / 60)),
    title: `${fullDate(day.date)}：读了 ${formatDurationShort(day.seconds ?? 0)}`,
  }));
  const totalSeconds = sorted.reduce((sum, day) => sum + Math.max(0, day.seconds ?? 0), 0);
  const activeDays = sorted.filter((day) => (day.seconds ?? 0) > 0).length;
  const peak = points.reduce<ReadingTrendPoint | null>(
    (best, point) => (!best || point.value > best.value ? point : best),
    null,
  );
  const note =
    totalSeconds > 0
      ? `近 ${points.length} 天读了 ${formatDurationShort(totalSeconds)}，${activeDays} 天有记录，单日最长 ${formatDurationShort(peak?.value ? peak.value * 60 : 0)}`
      : `近 ${points.length} 天还没有阅读记录`;
  return {
    empty: totalSeconds === 0,
    note,
    data: { points, totalSeconds, activeDays, peak },
  };
}

/* ── 14 导入累计曲线（折线）：import_daily 逐日累加 ── */

export interface CumulativePoint {
  date: string;
  label: string;
  /** 截至当天的累计导入本数 */
  value: number;
  title: string;
}

/**
 * 累计导入曲线：单日柱状图看「哪天进了多少」，累计曲线看「这 14 天整体进得多快」。
 *
 * 两点口径：
 *   - 逐日累加而不是平均：曲线只增不减，斜率就是当天导入量；
 *   - 纵轴从 0 起：从窗口起点开始累加，0 是「这 14 天还没开始导入」的真实含义。
 */
export function buildImportCumulative(
  points: ImportTrendPoint[],
): ChartModel<{ points: CumulativePoint[]; total: number; last: number }> {
  const sorted = [...points].sort((a, b) => a.date.localeCompare(b.date));
  let running = 0;
  const rows = sorted.map<CumulativePoint>((point) => {
    running += Math.max(0, point.books);
    return {
      date: point.date,
      label: shortDate(point.date),
      value: running,
      title: `${fullDate(point.date)}：累计 ${running} 本（当天 +${Math.max(0, point.books)} 本）`,
    };
  });
  const total = running;
  const last = Math.max(0, sorted[sorted.length - 1]?.books ?? 0);
  const note =
    total > 0
      ? `近 ${rows.length} 天累计导入 ${total} 本，最后一天 +${last} 本`
      : `近 ${rows.length} 天还没有导入记录`;
  return { empty: total === 0, note, data: { points: rows, total, last } };
}
