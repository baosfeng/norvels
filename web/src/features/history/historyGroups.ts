import type { ReadingHistoryRow } from "../../api/types";

/**
 * 阅读历史按「今天 / 昨天 / 更早」分组。
 *
 * 口径用本机日历日（不是「24 小时前」）：用户说「昨天读的」指的是日历上的昨天，
 * 凌晨读的书不该在今天 00:30 还显示成「今天」。
 * 组内保持后端给的顺序（最近读过的在前），分组只切分不重排。
 */
export type HistoryBucket = "today" | "yesterday" | "earlier";

export interface HistoryGroup {
  bucket: HistoryBucket;
  label: string;
  rows: ReadingHistoryRow[];
}

const LABELS: Record<HistoryBucket, string> = {
  today: "今天",
  yesterday: "昨天",
  earlier: "更早",
};

/** 本地零点的时间戳；用年月日构造，避免 DST 与「减 86400 秒」的漂移。 */
function startOfDay(t: number): number {
  const d = new Date(t);
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/** 一条记录落在哪一组：解析失败的时间算「更早」，不丢记录。 */
export function bucketOf(iso: string | null | undefined, now: number): HistoryBucket {
  const t = iso ? Date.parse(iso) : NaN;
  if (Number.isNaN(t)) return "earlier";
  const today = startOfDay(now);
  const oneDay = 24 * 60 * 60 * 1000;
  if (t >= today) return "today";
  if (t >= today - oneDay) return "yesterday";
  return "earlier";
}

/** 分组：空组不出现（页面上不留一个只有组头的空块）。 */
export function groupHistory(rows: ReadingHistoryRow[], now = Date.now()): HistoryGroup[] {
  const buckets: HistoryBucket[] = ["today", "yesterday", "earlier"];
  const byBucket = new Map<HistoryBucket, ReadingHistoryRow[]>(buckets.map((b) => [b, []]));
  for (const row of rows) {
    const bucket = bucketOf(row.lastReadAt, now);
    byBucket.get(bucket)?.push(row);
  }
  return buckets
    .map((bucket) => ({ bucket, label: LABELS[bucket], rows: byBucket.get(bucket) ?? [] }))
    .filter((group) => group.rows.length > 0);
}
