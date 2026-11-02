import type { ImportReview, ReviewBook, ReviewDecision, ReviewQuality } from "../../api/types";
import { formatCount, formatDate, formatWords } from "../../lib/format";

/** 人工能做的三个动作：decide 的两个取值 + 回滚。 */
export type ReviewAction = ReviewDecision | "rollback";

/** 状态徽标文案：与后端状态机一一对应，不把英文枚举甩给用户。 */
const STATUS_LABEL: Record<ImportReview["status"], string> = {
  pending: "等待 AI",
  ready: "AI 已给结论",
  needs_manual: "需要你判断",
  confirmed_replace: "已替换",
  confirmed_keep: "已保留",
  rolled_back: "已回滚",
};

/** 质量对比的三个维度（与后端的 garbage / noise / structure 三个键同源）。 */
const DIMENSIONS: { key: string; label: string; score: boolean }[] = [
  { key: "garbage", label: "乱码率", score: false },
  { key: "noise", label: "噪声率", score: false },
  { key: "structure", label: "结构分", score: true },
];

/**
 * 一张升级审核卡：两版书的对照 + 规则质量对比 + AI 结论 + 操作按钮。
 *
 * 纯展示组件：确认弹层与请求都在 ReviewPanel，卡片只把「点了哪个动作」抛回去。
 */
export function ReviewCard({
  review,
  aiEnabled,
  busy,
  onAction,
}: {
  review: ImportReview;
  aiEnabled: boolean;
  busy: boolean;
  onAction: (review: ImportReview, action: ReviewAction) => void;
}) {
  const canDecide = review.status === "ready" || review.status === "needs_manual";
  const canRollback = review.status === "confirmed_replace";
  const { quality } = review;

  return (
    <article className={`review-card is-${review.status}`} data-testid={`review-card-${review.id}`}>
      <header className="review-card-head">
        <span className={`pill review-badge is-${review.status}`} data-testid={`review-status-${review.id}`}>
          {STATUS_LABEL[review.status]}
        </span>
        <span className="meta" data-testid={`review-hit-${review.id}`}>
          命中率 {formatRatio(review.hitRatio)}
        </span>
        <span className="meta">创建于 {formatDate(review.createdAt)}</span>
      </header>

      <div className="review-books">
        <BookColumn book={review.oldBook} role="old" testId={`review-old-${review.id}`} />
        <span className="review-arrow" aria-hidden="true">
          →
        </span>
        <BookColumn book={review.newBook} role="new" testId={`review-new-${review.id}`} />
      </div>

      <div className="review-quality" data-testid={`review-quality-${review.id}`}>
        <table className="table">
          <thead>
            <tr>
              <th>维度</th>
              <th className="num">当前</th>
              <th className="num">新版</th>
              <th>对比</th>
            </tr>
          </thead>
          <tbody>
            {DIMENSIONS.map((dim) => (
              <QualityRow key={dim.key} dim={dim} quality={quality.old} next={quality.new} review={review} />
            ))}
          </tbody>
        </table>
        {quality.note ? (
          <p className="meta" data-testid={`review-quality-note-${review.id}`}>
            {quality.note}
          </p>
        ) : null}
      </div>

      <div className="review-ai" data-testid={`review-ai-${review.id}`}>
        <p className="review-ai-line">{aiSummary(review, aiEnabled)}</p>
        {review.ai.reasons.length > 0 ? (
          <ul className="review-reasons">
            {review.ai.reasons.map((reason) => (
              <li key={reason}>{reason}</li>
            ))}
          </ul>
        ) : null}
        {ruleReasonsOf(review).length > 0 ? (
          <p className="meta" data-testid={`review-rule-reasons-${review.id}`}>
            规则判断：{ruleReasonsOf(review).join("；")}
          </p>
        ) : null}
        {review.decidedAt ? (
          <p className="meta" data-testid={`review-decided-${review.id}`}>
            最后一次决策 {formatDate(review.decidedAt)}
            {review.decidedBy ? ` · ${review.decidedBy}` : ""}
          </p>
        ) : null}
      </div>

      <div className="review-actions">
        {canDecide ? (
          <>
            <button
              className="btn btn-primary"
              type="button"
              disabled={busy}
              data-testid={`review-replace-${review.id}`}
              onClick={() => onAction(review, "replace")}
            >
              替换为这一版
            </button>
            <button
              className="btn"
              type="button"
              disabled={busy}
              data-testid={`review-keep-${review.id}`}
              onClick={() => onAction(review, "keep")}
            >
              保留当前版本
            </button>
          </>
        ) : null}
        {canRollback ? (
          <button
            className="btn btn-danger"
            type="button"
            disabled={busy}
            data-testid={`review-rollback-${review.id}`}
            onClick={() => onAction(review, "rollback")}
          >
            回滚
          </button>
        ) : null}
        {review.status === "pending" ? (
          <span className="meta" data-testid={`review-waiting-${review.id}`}>
            {aiEnabled ? "等待 AI 裁决，稍后点刷新再看" : "等待人工处理"}
          </span>
        ) : null}
      </div>
    </article>
  );
}

/** 一侧书的摘要；supersededBy=0 表示这一版当前在用，非 0 表示已被另一版顶掉（仍在库里）。 */
function BookColumn({ book, role, testId }: { book: ReviewBook; role: "old" | "new"; testId: string }) {
  const title = book.title || "（这本书已被删除）";
  return (
    <div className="review-book" data-testid={testId}>
      <span className="meta">{columnLabel(book, role)}</span>
      <strong>{title}</strong>
      <span className="meta">
        {formatCount(book.chapterCount)} 章 · {formatWords(book.wordCount)}
      </span>
      <span className="meta">{book.title ? (book.supersededBy === 0 ? "在用" : "隐藏中") : "已删除"}</span>
    </div>
  );
}

/** 列标题跟着真实状态走：替换完成后，新版才是「当前版本」，旧版变成「旧版本」。 */
function columnLabel(book: ReviewBook, role: "old" | "new"): string {
  if (!book.title) return role === "old" ? "旧版本" : "新版";
  if (book.supersededBy === 0) return "当前版本";
  return role === "old" ? "旧版本" : "升级候选";
}

/** 一行质量对比：值按「是否算过」分别表示，更优 / 更差来自后端的 better / worse。 */
function QualityRow({
  dim,
  quality,
  next,
  review,
}: {
  dim: { key: string; label: string; score: boolean };
  quality: ReviewQuality;
  next: ReviewQuality;
  review: ImportReview;
}) {
  const oldValue = valueOf(quality, dim.key);
  const newValue = valueOf(next, dim.key);
  const mark = review.quality.better.some((item) => item.key === dim.key)
    ? "better"
    : review.quality.worse.some((item) => item.key === dim.key)
      ? "worse"
      : "";
  return (
    <tr data-testid={`review-dim-${review.id}-${dim.key}`}>
      <td>{dim.label}</td>
      <td className="num">{quality.computed ? formatRatio(oldValue, dim.score) : "未计算"}</td>
      <td className="num">{next.computed ? formatRatio(newValue, dim.score) : "未计算"}</td>
      <td className={mark ? `review-dim is-${mark}` : "review-dim"}>
        {mark === "better" ? "更优" : mark === "worse" ? "更差" : "—"}
      </td>
    </tr>
  );
}

/** 维度取值：比率用对应字段，结构分是 0–1 的分数。 */
function valueOf(quality: ReviewQuality, key: string): number {
  if (key === "garbage") return quality.garbageRatio;
  if (key === "noise") return quality.noiseRatio;
  return quality.structureScore;
}

function aiSummary(review: ImportReview, aiEnabled: boolean): string {
  if (review.status === "pending") {
    return aiEnabled ? "等待 AI 裁决" : "等待人工处理";
  }
  if (review.status === "needs_manual") return "AI 没能给出结论，需要你判断";
  if (review.status === "confirmed_replace") return "已替换成新版，旧版已隐藏";
  if (review.status === "confirmed_keep") return "已保留当前版本，新版继续隐藏";
  if (review.status === "rolled_back") return "已回滚到旧版，新版重新隐藏";
  const winner = review.ai.winner;
  const confidence = review.ai.confidence > 0 ? `（把握 ${Math.round(review.ai.confidence * 100)}%）` : "";
  if (winner === "new") return `AI 建议换成这一版${confidence}`;
  if (winner === "old") return `AI 建议保留当前版本${confidence}`;
  if (winner === "equal") return "AI 认为两版质量相当";
  return "AI 没有给出倾向，按规则对比判断";
}

/**
 * 规则理由的展示时机：needs_manual 时它是人工判断的直接依据（要展示）；
 * 其余状态只在 AI 没有给出理由时兜底，避免同一段文案出现两次。
 */
function ruleReasonsOf(review: ImportReview): string[] {
  if (review.status === "needs_manual") return review.ruleReasons;
  return review.ai.reasons.length === 0 ? review.ruleReasons : [];
}

/**
 * 比率 → 人话百分比：0 写 0%；1% 以上保留一位小数，更小的保留三位（乱码率常有 0.016%）。
 * 结构分是 0–1 的分数，不做百分比，固定四位小数。
 */
function formatRatio(value: number, score = false): string {
  if (!Number.isFinite(value) || value <= 0) return score ? "0" : "0%";
  if (score) return value.toFixed(4);
  const pct = value * 100;
  const text = pct >= 1 ? pct.toFixed(1) : pct.toFixed(3);
  return `${text.replace(/\.?0+$/, "")}%`;
}
