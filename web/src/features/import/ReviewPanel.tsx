import { useState } from "react";
import { ApiError, api } from "../../api/client";
import { useImportReviews } from "../../api/hooks";
import type { ImportReview } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { ReviewCard, type ReviewAction } from "./ReviewCard";

/** 一次拉多少条（后端默认 50、上限 200）：第一屏够用，暂不做翻页。 */
const REVIEW_LIMIT = 50;

interface ConfirmTarget {
  review: ImportReview;
  action: ReviewAction;
}

/**
 * 升级审核入口（导入页内联，不新开顶层路由）。
 *
 * 收起时只露一行：标题 + 「pending + ready」数量徽标 + 展开按钮，
 * 展开后才取整张列表 —— 待办为 0 的人在导入页看不到任何多余的空态。
 */
export function ReviewPanel() {
  const { data, loading, error, reload } = useImportReviews("all", REVIEW_LIMIT);
  const [open, setOpen] = useState(false);
  // 决策后直接用响应体更新那一行（契约 §前端要点 2），不为了一个状态再拉一次列表
  const [decided, setDecided] = useState<Record<number, ImportReview>>({});
  const [confirming, setConfirming] = useState<ConfirmTarget | null>(null);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");

  const items = mergeItems(data?.items ?? [], decided);
  const todoCount = items.filter((item) => item.status === "pending" || item.status === "ready").length;

  function retry() {
    setDecided({});
    reload();
  }

  async function submit() {
    if (!confirming || busy) return;
    const { review, action } = confirming;
    setBusy(true);
    setNote("");
    try {
      const updated =
        action === "rollback"
          ? await api.rollbackImportReview(review.id)
          : await api.decideImportReview(review.id, action);
      setDecided((prev) => ({ ...prev, [updated.id]: updated }));
      setNote(decidedNote(action, updated));
      setConfirming(null);
    } catch (err) {
      setNote(failureNote(err));
      setConfirming(null);
      // 状态机拒绝 / 记录消失：屏幕上的那一行已经过期，重新拉一次才对
      if (err instanceof ApiError && (err.status === 409 || err.status === 404)) retry();
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="import-jobs review-panel" data-testid="review-panel">
      <div className="head">
        <div>
          <h2>待确认的升级版</h2>
          <span className="meta">导入判出「新版显著更好」才进这里；替换 / 保留都要你点</span>
        </div>
        <div className="review-head-actions">
          <span className="pill review-badge is-todo" data-testid="review-todo-count">
            {loading ? "读取中…" : `${todoCount} 条待确认`}
          </span>
          <button
            className="btn"
            type="button"
            aria-expanded={open}
            data-testid="review-toggle"
            onClick={() => setOpen((value) => !value)}
          >
            {open ? "收起" : "展开"}
          </button>
        </div>
      </div>

      {open ? (
        <div className="review-body">
          {note ? (
            <p className="meta" role="status" data-testid="review-note">
              {note}
            </p>
          ) : null}
          {loading ? (
            <LoadingPanel
              title="正在读取升级审核"
              description="正在取回新旧版本的对照记录。"
              testId="review-loading"
            />
          ) : error ? (
            <ErrorPanel error={error} title="升级审核没能加载" onRetry={retry} testId="review-error" />
          ) : items.length === 0 ? (
            <EmptyPanel
              title="没有待确认的升级版"
              description="导入时判出新版本显著更好，这里才会出现一条新旧对比。"
              testId="review-empty"
            />
          ) : (
            <>
              {data?.aiEnabled === false ? (
                <p className="inline-note" data-testid="review-ai-off">
                  AI 未启用：只有规则结论，要不要换由你判断。
                </p>
              ) : null}
              <div className="review-body-head">
                <span className="meta">
                  共 {items.length} 条 · 其中 {todoCount} 条待确认
                </span>
                <button className="btn btn-ghost" type="button" data-testid="review-refresh" onClick={retry}>
                  刷新
                </button>
              </div>
              <div className="review-list">
                {items.map((review) => (
                  <ReviewCard
                    key={review.id}
                    review={review}
                    aiEnabled={data?.aiEnabled !== false}
                    busy={busy}
                    onAction={(target, action) => {
                      setNote("");
                      setConfirming({ review: target, action });
                    }}
                  />
                ))}
              </div>
            </>
          )}
        </div>
      ) : null}

      {confirming ? (
        <ReviewConfirmDialog
          target={confirming}
          busy={busy}
          onCancel={() => setConfirming(null)}
          onConfirm={() => void submit()}
        />
      ) : null}
    </div>
  );
}

/** 决策前的二次确认：三个动作共用一套弹层，文案各自说清后果。 */
function ReviewConfirmDialog({
  target,
  busy,
  onCancel,
  onConfirm,
}: {
  target: ConfirmTarget;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { review, action } = target;
  const verb = action === "replace" ? "替换" : action === "keep" ? "保留" : "回滚";
  return (
    <div className="confirm-backdrop" role="presentation" onClick={onCancel}>
      <div
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="review-confirm-title"
        data-testid="review-confirm"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="review-confirm-title" data-testid="review-confirm-title">
          {confirmTitle(review, action)}
        </h2>
        <p className="meta" data-testid="review-confirm-text">
          {confirmDescription(action)}
        </p>
        <div className="confirm-actions">
          <button className="btn btn-ghost" type="button" autoFocus data-testid="review-confirm-cancel" onClick={onCancel}>
            取消
          </button>
          <button
            className={action === "rollback" ? "btn btn-danger" : "btn btn-primary"}
            type="button"
            disabled={busy}
            data-testid="review-confirm-submit"
            onClick={onConfirm}
          >
            {busy ? "正在处理…" : verb}
          </button>
        </div>
      </div>
    </div>
  );
}

/** 列表项与本地决策结果合并：本地那份是刚拿到的响应体，优先于列表里的旧状态。 */
function mergeItems(items: ImportReview[], decided: Record<number, ImportReview>): ImportReview[] {
  return items.map((item) => decided[item.id] ?? item);
}

function confirmTitle(review: ImportReview, action: ReviewAction): string {
  if (action === "replace") return `替换为「${titleOf(review.newBook.title)}」？`;
  if (action === "keep") return `保留「${titleOf(review.oldBook.title)}」？`;
  return `回滚到「${titleOf(review.oldBook.title)}」？`;
}

function confirmDescription(action: ReviewAction): string {
  if (action === "replace") {
    return `当前版本会从书架隐藏，新版成为在用版本；之后还能回滚，正文都在库里。`;
  }
  if (action === "keep") {
    return `新版继续隐藏，不上架；保留是最终决定，之后不能再改。`;
  }
  return `新版重新隐藏，旧版回到书架；正文都没动过。`;
}

function decidedNote(action: ReviewAction, review: ImportReview): string {
  if (action === "replace") return `已替换为「${titleOf(review.newBook.title)}」。`;
  if (action === "keep") return `已保留「${titleOf(review.oldBook.title)}」。`;
  return `已回滚到「${titleOf(review.oldBook.title)}」。`;
}

function titleOf(title: string): string {
  return title || "这本书已被删除";
}

/** 失败文案：状态机 / 不存在 / 超时都翻成人话，不把后端错误原样甩出来。 */
function failureNote(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 409) return "这条记录刚刚已经变过了，列表已重新拉取。";
    if (error.status === 404) return "这条审核记录已经不在了。";
    if (error.kind === "timeout") return "请求超时了，稍后再试。";
  }
  return "操作没成功，稍后再试。";
}
