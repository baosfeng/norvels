import { useState } from "react";
import { ApiError, api } from "../../api/client";
import type { ChapterBrief } from "../../api/types";
import { spanOf } from "../../lib/chapterBatch";
import { formatCount, formatWords } from "../../lib/format";
import { bumpStatsRevision } from "../../lib/statsRevision";

/**
 * 批量删除章节的二次确认：写明书名、删几章、章号区间与总字数，说明无法撤销，
 * 默认焦点落在「取消」——误触 Enter 不该删书。
 */
export function ChapterBatchDeleteDialog({
  bookId,
  bookTitle,
  chapters,
  onClose,
  onDeleted,
}: {
  bookId: number;
  bookTitle: string;
  chapters: ChapterBrief[];
  onClose: () => void;
  onDeleted: (count: number) => void;
}) {
  const span = spanOf(chapters);
  const [busy, setBusy] = useState(false);
  const [errorText, setErrorText] = useState("");

  async function confirm() {
    if (busy) return;
    setBusy(true);
    setErrorText("");
    try {
      await api.batchDeleteChapters(bookId, chapters.map((chapter) => chapter.id));
      bumpStatsRevision();
      onDeleted(span.count);
    } catch (error) {
      setErrorText(batchDeleteErrorText(error));
      setBusy(false);
    }
  }

  return (
    <div
      className="confirm-backdrop"
      role="presentation"
      onClick={() => {
        if (!busy) onClose();
      }}
    >
      <div
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="chapter-batch-delete-title"
        data-testid="chapter-batch-delete-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="chapter-batch-delete-title">
          删除《{bookTitle}》的 {span.count} 章？
        </h2>
        <p className="meta" data-testid="chapter-batch-delete-text">
          将删除 {span.count} 章（第 {span.first}–{span.last} 章，共 {formatWords(span.words)}），后面的章节序号自动补上。无法撤销。
        </p>
        {errorText ? (
          <p className="meta" role="alert" data-testid="chapter-batch-delete-error">
            {errorText}
          </p>
        ) : null}
        <div className="confirm-actions">
          <button
            className="btn btn-ghost"
            type="button"
            autoFocus
            disabled={busy}
            data-testid="chapter-batch-delete-cancel"
            onClick={onClose}
          >
            取消
          </button>
          <button
            className="btn btn-danger"
            type="button"
            disabled={busy}
            data-testid="chapter-batch-delete-confirm"
            onClick={() => void confirm()}
          >
            {busy ? "正在删除…" : `确认删除 ${formatCount(span.count)} 章`}
          </button>
        </div>
      </div>
    </div>
  );
}

/** 批量删除失败文案：409 不会发生（merge 才有），其余按状态码翻人话。 */
function batchDeleteErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400) return error.message || "选中的章节不合法，刷新页面再试。";
    if (error.status === 404) return "有些章已经不在了，刷新页面再选一次。";
    if (error.kind === "timeout") return "删除请求超时了，稍后重试。";
  }
  return "删除没能完成，稍后再试。";
}
