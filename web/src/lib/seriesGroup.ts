import type { BookSummary, DaySeconds, SeriesSummary } from "../api/types";
import { formatCount, formatDate, formatDuration } from "./format";

/**
 * 系列聚合页与阅读热力图共用的纯整形逻辑（无 React、无网络）：组件只负责排版。
 * 后端已把聚合值算好（书数 / 字数 / 最近更新 / 封面书），这里只做「字段 → 人话标签」与排序，
 * 不在前端重算聚合，避免与后端两份口径。
 */

export interface SeriesCardModel {
  /** 链接用的系列名（调用方负责 URL 编码） */
  name: string;
  /** 「12 本」 */
  bookCountLabel: string;
  /** 「1.2 万字」 */
  wordCountLabel: string;
  /** 「3 天前」；系列内没有章节时是「还没有章节」 */
  updatedLabel: string;
  /** coverBookId > 0 才去取封面，否则用系列名首字占位 */
  coverBookId: number;
  hasCover: boolean;
  /** 书名列表为空时页面上要能分辨「空系列」 */
  empty: boolean;
}

export interface SeriesCardsModel {
  cards: SeriesCardModel[];
  empty: boolean;
  /** 当前页这些系列一共多少本书（页头「本页 N 个系列 · M 本书」用） */
  bookTotal: number;
}

/** 单个系列 → 卡片文案。字段缺失（老后端）一律按「没有」处理，不猜。 */
export function seriesCardModel(item: SeriesSummary, now = Date.now()): SeriesCardModel {
  const name = (item.name ?? "").trim();
  const bookCount = Number.isFinite(item.bookCount) ? Math.max(0, item.bookCount) : 0;
  const wordCount = Number.isFinite(item.wordCount) ? Math.max(0, item.wordCount) : 0;
  const coverBookId = Number.isFinite(item.coverBookId) ? Math.max(0, item.coverBookId) : 0;
  return {
    name,
    bookCountLabel: `${formatCount(bookCount)} 本`,
    wordCountLabel: `${formatCount(wordCount)} 字`,
    updatedLabel: item.lastUpdatedAt ? formatDate(item.lastUpdatedAt, now) : "还没有章节",
    coverBookId,
    hasCover: coverBookId > 0,
    empty: bookCount === 0,
  };
}

/** 列表整形：顺序即后端顺序（最近更新倒序），前端不再排。 */
export function buildSeriesCards(items: SeriesSummary[], now = Date.now()): SeriesCardsModel {
  const cards = (items ?? []).map((item) => seriesCardModel(item, now));
  return {
    cards,
    empty: cards.length === 0,
    bookTotal: (items ?? []).reduce((sum, item) => sum + Math.max(0, item.bookCount ?? 0), 0),
  };
}

/**
 * 系列详情里的书目排序：seriesIndex 升序（0 / 缺省 = 没标序号，排最后），
 * 同序号按章数降序、再按书名——同一个系列多部作品的阅读顺序可预期。
 */
export function sortSeriesBooks(books: BookSummary[]): BookSummary[] {
  return [...(books ?? [])].sort((a, b) => {
    const ai = seriesIndexOf(a);
    const bi = seriesIndexOf(b);
    if (ai !== bi) return ai - bi;
    if (a.chapterCount !== b.chapterCount) return b.chapterCount - a.chapterCount;
    return (a.title ?? "").localeCompare(b.title ?? "", "zh-Hans-CN");
  });
}

function seriesIndexOf(book: BookSummary): number {
  const index = book.seriesIndex ?? 0;
  if (!Number.isFinite(index) || index <= 0) return Number.MAX_SAFE_INTEGER;
  return index;
}

/* ── 阅读热力图（近 N 天） ── */

export interface HeatmapDay {
  /** YYYY-MM-DD（后端口径，本地库时区） */
  date: string;
  seconds: number;
  /** 0 = 没读，1–4 由秒数档位给出（配色只需 5 档，不按最大值归一） */
  level: 0 | 1 | 2 | 3 | 4;
  /** 0 = 周日 … 6 = 周六 */
  weekday: number;
  /** 悬浮提示：「10 月 24 日 · 1 小时 20 分」 */
  label: string;
}

export interface HeatmapModel {
  /** 按列（一周一列）排好的格子，null 是补位（月底/月初的空格） */
  columns: (HeatmapDay | null)[][];
  days: HeatmapDay[];
  /** 有阅读记录的天数 */
  activeDays: number;
  maxSeconds: number;
  empty: boolean;
}

/** 档位阈值（秒）：15 分 / 30 分 / 60 分，超出即最高档。 */
export const HEATMAP_THRESHOLDS = [900, 1800, 3600] as const;

export function heatLevel(seconds: number): 0 | 1 | 2 | 3 | 4 {
  if (!Number.isFinite(seconds) || seconds <= 0) return 0;
  if (seconds < HEATMAP_THRESHOLDS[0]) return 1;
  if (seconds < HEATMAP_THRESHOLDS[1]) return 2;
  if (seconds < HEATMAP_THRESHOLDS[2]) return 3;
  return 4;
}

/** 日期串 → 周几；解析不了时返回 0（补位到第一行，不抛错）。 */
function weekdayOf(date: string): number {
  const time = Date.parse(`${date}T00:00:00Z`);
  return Number.isNaN(time) ? 0 : new Date(time).getUTCDay();
}

/** 「10 月 24 日」：热力图的悬浮与 aria 文案都用它，不显示原始 ISO。 */
function dayLabel(date: string): string {
  const time = Date.parse(`${date}T00:00:00Z`);
  if (Number.isNaN(time)) return date;
  const d = new Date(time);
  return `${d.getUTCMonth() + 1} 月 ${d.getUTCDate()} 日`;
}

/**
 * 近 N 天 → 7 行 × 周列的日历热力图数据。
 * 只保留后端给的天数（不做补零扩展）：后端已按天补齐 0 值，前端多补会与 streak 口径打架。
 */
export function buildHeatmap(days: DaySeconds[]): HeatmapModel {
  const list: HeatmapDay[] = (days ?? []).map((day) => {
    const seconds = Number.isFinite(day.seconds) ? Math.max(0, Math.trunc(day.seconds)) : 0;
    return {
      date: day.date,
      seconds,
      level: heatLevel(seconds),
      weekday: weekdayOf(day.date),
      label: `${dayLabel(day.date)} · ${seconds > 0 ? formatDuration(seconds) : "没有阅读"}`,
    };
  });

  const columns: (HeatmapDay | null)[][] = [];
  if (list.length > 0) {
    // 第一列顶部按首日周几补空，之后每 7 天换列
    const first: (HeatmapDay | null)[] = Array.from({ length: list[0].weekday }, () => null);
    for (const day of list) first.push(day);
    while (first.length % 7 !== 0) first.push(null);
    for (let i = 0; i < first.length; i += 7) columns.push(first.slice(i, i + 7));
  }

  return {
    columns,
    days: list,
    activeDays: list.filter((day) => day.level > 0).length,
    maxSeconds: list.reduce((max, day) => Math.max(max, day.seconds), 0),
    empty: list.length === 0 || list.every((day) => day.level === 0),
  };
}
