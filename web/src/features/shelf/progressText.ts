import { formatCount } from "../../lib/format";

/**
 * 阅读进度的**唯一文案口径**：书架卡、书籍详情、阅读器顶栏三处共用这一句。
 *
 * 「第 N 章 / 共 M 章」；只有总数（没有读到哪）时退化成「共 M 章」。
 * 传进来的 chapterIdx 是 0 起下标（后端用指针区分「没有」与「第 0 章」），展示前统一 +1，
 * 所以「读到第 1 章」显示 1，不是 0。章数用 formatCount，与详情页目录头的「共 N 章」同一个格式化。
 */
export function chapterPosition(
  chapterIdx: number | null | undefined,
  total: number,
): string {
  if (!Number.isFinite(total) || total <= 0) return "";
  if (chapterIdx === null || chapterIdx === undefined || !Number.isFinite(chapterIdx) || chapterIdx < 0) {
    return `共 ${formatCount(total)} 章`;
  }
  return `第 ${chapterIdx + 1} 章 / 共 ${formatCount(total)} 章`;
}

/**
 * 已读百分比：0–100 的整数，读过就至少显示 1%（0% 的进度条等于没有，读者会以为坏了）。
 * 没有进度记录（percent 为 null / NaN / ≤0）时返回 null，调用方不显示百分比。
 */
export function readPercent(progress: { percent: number } | null | undefined): number | null {
  const raw = progress?.percent;
  if (raw === null || raw === undefined || !Number.isFinite(raw) || raw <= 0) return null;
  return Math.min(100, Math.max(1, Math.round(raw * 100)));
}
