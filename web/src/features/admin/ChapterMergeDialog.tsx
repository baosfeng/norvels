import { useState } from "react";
import { ApiError, api } from "../../api/client";
import type { ChapterBrief } from "../../api/types";
import { MERGE_RULE_TEXT, spanOf } from "../../lib/chapterBatch";
import { formatCount, formatWords } from "../../lib/format";

/**
 * 合并章节面板：列出被合并的章、可编辑合并后标题（默认首章标题）、合并后字数，
 * 并写明「阅读进度与书签会失效」。确认后调 merge；标题改过就再 PATCH 一次首章。
 *
 * 标题与正文合并是两个端点：合并成功后再补标题，失败只提示、不回滚合并——
 * 回滚要把正文重新切开，那种「撤销」比留着旧标题更危险。
 */
export function ChapterMergeDialog({
  bookId,
  chapters,
  totalChapters,
  onClose,
  onMerged,
}: {
  bookId: number;
  /** 已按章号排好序的待合并章节（≥2） */
  chapters: ChapterBrief[];
  /** 当前全书章数（面板上写「总章数 N → N-k」） */
  totalChapters: number;
  onClose: () => void;
  onMerged: (result: { chapters: number; titleSaved: boolean }) => void;
}) {
  const first = chapters[0];
  const [title, setTitle] = useState(first?.title ?? "");
  const [busy, setBusy] = useState(false);
  const [errorText, setErrorText] = useState("");
  const span = spanOf(chapters);

  async function confirm() {
    if (busy) return;
    const next = title.trim();
    if (!next) {
      setErrorText("合并后的标题不能为空。");
      return;
    }
    setBusy(true);
    setErrorText("");
    try {
      const result = await api.mergeChapters(bookId, chapters.map((chapter) => chapter.id));
      let titleSaved = true;
      if (next !== first.title) {
        try {
          await api.updateChapter(bookId, first.id, next);
        } catch {
          titleSaved = false;
        }
      }
      onMerged({ chapters: result.chapters, titleSaved });
    } catch (error) {
      setErrorText(mergeErrorText(error));
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
        aria-labelledby="merge-title"
        data-testid="chapter-merge-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="merge-title">合并章节</h2>
        <p className="meta" data-testid="chapter-merge-sub">
          把选中的 {span.count} 章合并成 1 章，正文按顺序接在一起。
        </p>

        <div className="merge-list" data-testid="chapter-merge-list">
          {chapters.map((chapter) => (
            <div className="merge-item" key={chapter.id}>
              <span className="seq">第 {chapter.idx + 1} 章</span>
              <span>{chapter.title}</span>
              <span className="words">{formatWords(chapter.wordCount)}</span>
            </div>
          ))}
        </div>

        <div className="field">
          <label htmlFor="merge-title-input">合并后的标题</label>
          <input
            className="input"
            id="merge-title-input"
            value={title}
            autoComplete="off"
            data-testid="chapter-merge-title"
            onChange={(event) => {
              setTitle(event.target.value);
              setErrorText("");
            }}
          />
          <span className="field-hint">默认取首章标题，可以改成新的名字。</span>
        </div>

        <div className="merge-sum" data-testid="chapter-merge-sum">
          <span>合并后字数</span>
          <b>{formatWords(span.words)}</b>
          <span className="meta">
            {span.count} 章字数之和 · 总章数 {formatCount(totalChapters)} → {formatCount(totalChapters - span.count + 1)}
          </span>
        </div>

        <p className="meta" data-testid="chapter-merge-note">
          被合并章节的阅读进度与书签会失效。
        </p>

        {errorText ? (
          <p className="meta" role="alert" data-testid="chapter-merge-error">
            {errorText}
          </p>
        ) : null}

        <div className="confirm-actions">
          <button className="btn btn-primary" type="button" disabled={busy} data-testid="chapter-merge-confirm" onClick={() => void confirm()}>
            {busy ? "正在合并…" : "合并为 1 章"}
          </button>
          <button className="btn" type="button" disabled={busy} data-testid="chapter-merge-cancel" onClick={onClose}>
            取消
          </button>
        </div>
      </div>
    </div>
  );
}

/**
 * 合并失败文案：409 not_mergeable 的 message 后端已经写成人话（哪一章不连续、跨了哪卷），
 * 直接展示；没有 message 时退回规则本身，用户至少知道该往哪改。
 */
function mergeErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "not_mergeable") return error.message || MERGE_RULE_TEXT;
    if (error.status === 409) return error.message || MERGE_RULE_TEXT;
    if (error.status === 400) return error.message || "选中的章节不合法，刷新页面再选一次。";
    if (error.status === 404) return "有些章已经不在了，刷新页面再选一次。";
    if (error.kind === "timeout") return "合并超时了，稍后重试。";
  }
  return "合并没能完成，稍后再试。";
}
