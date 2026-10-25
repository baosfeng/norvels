/** 数字与时间的「人话」格式化：UI 只显示这些结果，不显示实现单位。 */

/** 字数/章节数转中文量级：12345 → 1.2 万。 */
export function formatCount(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "0";
  if (n < 10_000) return String(Math.round(n));
  if (n < 100_000_000) return `${(n / 10_000).toFixed(n < 100_000 ? 1 : 1)} 万`.replace(".0 ", " ");
  return `${(n / 100_000_000).toFixed(1)} 亿`.replace(".0 ", " ");
}

/**
 * 单章字数 → 「1,234 字」。目录里每章都要显示，所以不用 Intl（构造器有开销），
 * 直接给整数加千分位；四舍五入到整数。
 */
export function formatWords(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return "0 字";
  return `${String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ",")} 字`;
}

/** 字节转 MB/GB，用于库体积展示。 */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "—";
  const gb = bytes / 1024 ** 3;
  if (gb >= 1) return `${gb.toFixed(1)} GB`;
  const mb = bytes / 1024 ** 2;
  if (mb >= 1) return `${mb.toFixed(0)} MB`;
  return `${Math.max(1, Math.round(bytes / 1024))} KB`;
}

/** ISO 时间转「10 月 24 日」/「2 小时前」。设计口径：只给人话，不外露时区与格式。 */
export function formatDate(iso: string | null | undefined, now = Date.now()): string {
  if (!iso) return "—";
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return "—";
  const diff = now - t;
  const minute = 60_000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (diff >= 0 && diff < minute) return "刚刚";
  if (diff >= 0 && diff < hour) return `${Math.floor(diff / minute)} 分钟前`;
  if (diff >= 0 && diff < day) return `${Math.floor(diff / hour)} 小时前`;
  if (diff >= 0 && diff < 7 * day) return `${Math.floor(diff / day)} 天前`;
  const d = new Date(t);
  return `${d.getMonth() + 1} 月 ${d.getDate()} 日`;
}

/** 章节进度换算成 0–100 的整数百分比。 */
export function percentOf(chapterIdx: number, total: number): number {
  if (!Number.isFinite(chapterIdx) || !Number.isFinite(total) || total <= 0) return 0;
  const clamped = Math.min(Math.max(chapterIdx, 0), total);
  return Math.round((clamped / total) * 100);
}

/** 单章字数 → 阅读时长（按每分钟 500 字，给人话提示）。 */
export function readingMinutes(words: number): number {
  if (!Number.isFinite(words) || words <= 0) return 1;
  return Math.max(1, Math.round(words / 500));
}

/**
 * 秒数 → 阅读时长的「人话」：热力图与阅读历史共用同一口径，避免两处各写一套四舍五入。
 * 不足 1 分钟按「不到 1 分钟」，1 小时以上带小时（≥10 小时不显示分钟，读数更稳）。
 */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "0 分钟";
  if (seconds < 60) return "不到 1 分钟";
  const minutes = Math.round(seconds / 60);
  if (minutes < 1) return "不到 1 分钟";
  if (minutes < 60) return `${minutes} 分钟`;
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  if (hours >= 10 || rest === 0) return `${hours} 小时`;
  return `${hours} 小时 ${rest} 分`;
}

/** 秒数 → 「2 小时 5 分」的短口径（卡片数值用），与 formatDuration 同一套取整。 */
export function formatDurationShort(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "0 分";
  if (seconds < 60) return "<1 分";
  const minutes = Math.round(seconds / 60);
  if (minutes < 1) return "<1 分";
  if (minutes < 60) return `${minutes} 分`;
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest === 0 ? `${hours} 小时` : `${hours} 小时 ${rest} 分`;
}
