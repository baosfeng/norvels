import { useState } from "react";
import { api } from "../../api/client";
import type { BookDetail } from "../../api/types";
import { showToast } from "../../lib/toastStore";

/** 评分档位数：0 星 = 未评（不参与 rating 筛选与 sort=rating），与后端同口径。 */
export const MAX_RATING = 5;

/**
 * 0–5 星控件（纯展示 + 回调）：用按钮而不是可拖拽元素，键盘 Tab 与 Enter 天然可用。
 * 点当前星级 = 取消评分（回到 0），与「再点一次取消收藏」的习惯一致。
 */
export function RatingStars({
  value,
  onRate,
  disabled = false,
  testId = "rating-stars",
}: {
  value: number;
  onRate: (next: number) => void;
  disabled?: boolean;
  testId?: string;
}) {
  const current = clampRating(value);
  return (
    <div className="chips" role="group" aria-label="评分" data-testid={testId}>
      {Array.from({ length: MAX_RATING }, (_, i) => i + 1).map((star) => (
        <button
          key={star}
          type="button"
          className={`chip${star <= current ? " is-active" : ""}`}
          aria-label={`${star} 星`}
          aria-pressed={star === current}
          disabled={disabled}
          data-testid={`${testId}-star-${star}`}
          onClick={() => onRate(star === current ? 0 : star)}
        >
          {star <= current ? "★" : "☆"}
        </button>
      ))}
      <span className="meta" data-testid={`${testId}-label`}>
        {current > 0 ? `${current} 星` : "未评分"}
      </span>
    </div>
  );
}

/**
 * 详情页的评分行：乐观更新（点完立刻亮），失败退回原值并 toast。
 * 只下发 rating 一个字段，不动别的元信息；成功后不再 reload（避免整页闪白）。
 */
export function BookRating({ book, onChanged }: { book: BookDetail; onChanged?: (rating: number) => void }) {
  const [rating, setRating] = useState(book.rating ?? 0);
  const [saving, setSaving] = useState(false);

  async function rate(next: number) {
    const previous = rating;
    if (next === previous || saving) return;
    setRating(next);
    setSaving(true);
    try {
      await api.updateBook(book.id, { rating: next });
      onChanged?.(next);
    } catch {
      setRating(previous);
      showToast("评分没能保存，稍后再试。", "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="rating-row" data-testid="book-rating">
      <RatingStars value={rating} onRate={(next) => void rate(next)} disabled={saving} testId="book-rating" />
    </div>
  );
}

function clampRating(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(Math.max(Math.round(value), 0), MAX_RATING);
}
