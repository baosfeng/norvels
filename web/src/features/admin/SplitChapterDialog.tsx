import { useMemo, useState } from "react";
import { ApiError, api } from "../../api/client";
import { useChapter } from "../../api/hooks";
import type { ChapterBrief } from "../../api/types";
import {
  autoCuts,
  canSplit,
  paragraphsOf,
  splitPieces,
  splitTitles,
  toCutAfter,
} from "../../lib/splitPlan";
import { formatCount } from "../../lib/format";

/**
 * 拆分章节面板：左边按段落列正文（段落之间点「从这里拆开」），右边实时给出拆成几章。
 *
 * 只在段落边界切——按任意字符切会把一句话劈成两半，那种坏数据没法修。
 * 正文取一次（GET chapter），切点全部在前端算，只有「确定拆分」才发一个请求。
 */
export function SplitChapterDialog({
  bookId,
  chapter,
  onClose,
  onSplit,
}: {
  bookId: number;
  chapter: Pick<ChapterBrief, "id" | "idx" | "title" | "wordCount">;
  onClose: () => void;
  onSplit: (pieces: number) => void;
}) {
  const { data, loading, error, reload } = useChapter(bookId, chapter.id);
  const [cuts, setCuts] = useState<Set<number>>(() => new Set());
  const [target, setTarget] = useState("8000");
  const [busy, setBusy] = useState(false);
  const [submitError, setSubmitError] = useState("");

  const paragraphs = useMemo(() => (data ? paragraphsOf(data.content) : []), [data]);
  const pieces = useMemo(() => splitPieces(paragraphs, cuts), [paragraphs, cuts]);
  const titles = useMemo(() => splitTitles(chapter.title, pieces.length), [chapter.title, pieces.length]);
  const totalWords = useMemo(() => paragraphs.reduce((sum, p) => sum + p.words, 0), [paragraphs]);
  const ready = canSplit(pieces);

  function toggleCut(boundary: number) {
    setCuts((prev) => {
      const next = new Set(prev);
      if (next.has(boundary)) next.delete(boundary);
      else next.add(boundary);
      return next;
    });
    setSubmitError("");
  }

  function applyAuto() {
    const parsed = Number(target);
    if (!Number.isFinite(parsed) || parsed < 500) {
      setSubmitError("每章目标字数至少 500 字。");
      return;
    }
    setCuts(new Set(autoCuts(paragraphs, parsed)));
    setSubmitError("");
  }

  async function submit() {
    if (busy || !ready) return;
    setBusy(true);
    setSubmitError("");
    try {
      const result = await api.splitChapter(bookId, chapter.id, toCutAfter(cuts));
      onSplit(result.chapters);
    } catch (err) {
      setSubmitError(splitErrorText(err));
      setBusy(false);
    }
  }

  return (
    <div className="confirm-backdrop" role="presentation" onClick={() => !busy && onClose()}>
      <div
        className="confirm-dialog is-wide"
        role="dialog"
        aria-modal="true"
        aria-labelledby="split-title"
        data-testid="split-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="split-title">拆分「{chapter.title}」</h2>

        {loading ? (
          <p className="meta" data-testid="split-loading">
            正在读取这一章正文…
          </p>
        ) : error ? (
          <div data-testid="split-error">
            <p className="meta" role="alert">
              {splitErrorText(error)}
            </p>
            <button className="btn btn-sm" type="button" onClick={reload}>
              重新读取
            </button>
          </div>
        ) : (
          <>
            <p className="meta" data-testid="split-sub">
              这一章 {formatCount(totalWords)} 字 · {paragraphs.length} 段。在段落之间点「从这里拆开」，右侧实时给出拆完的结果。
            </p>
            <div className="split-wrap">
              <div className="split-paras" data-testid="split-paras" tabIndex={0} aria-label="章节正文段落">
                {paragraphs.map((paragraph, i) => (
                  <div key={paragraph.index}>
                    <div className="split-para">
                      <span className="pno">{paragraph.index + 1}</span>
                      <span className="ptext">{paragraph.text || "（空行）"}</span>
                      <span className="pwords">{formatCount(paragraph.words)} 字</span>
                    </div>
                    {i + 1 < paragraphs.length ? (
                      <button
                        className={`split-cut-btn${cuts.has(i + 1) ? " is-cut" : ""}`}
                        type="button"
                        data-testid={`split-cut-${i + 1}`}
                        aria-pressed={cuts.has(i + 1)}
                        onClick={() => toggleCut(i + 1)}
                      >
                        <span className="line" aria-hidden="true" />
                        {cuts.has(i + 1) ? "拆开处 · 点一下取消" : "从这里拆开"}
                        <span className="line" aria-hidden="true" />
                      </button>
                    ) : null}
                  </div>
                ))}
              </div>

              <div className="split-side">
                <div className="split-auto">
                  <div className="field">
                    <label htmlFor="split-target">每章目标字数</label>
                    <input
                      className="input"
                      id="split-target"
                      type="number"
                      min={500}
                      step={500}
                      value={target}
                      data-testid="split-target"
                      onChange={(event) => setTarget(event.target.value)}
                    />
                  </div>
                  <button className="btn btn-sm" type="button" data-testid="split-auto" onClick={applyAuto}>
                    按目标切
                  </button>
                </div>

                <p className="split-side-label">拆完是这样</p>
                <div className="split-preview" data-testid="split-preview">
                  {pieces.map((piece, i) => (
                    <div className="split-piece" key={`${piece.from}-${piece.to}`}>
                      <b>{titles[i]}</b>
                      <span className="meta num">
                        {formatCount(piece.words)} 字 · 第 {piece.from + 1}–{piece.to} 段
                      </span>
                    </div>
                  ))}
                </div>
                <p className="meta" data-testid="split-summary">
                  {ready
                    ? `将拆成 ${pieces.length} 章，各章字数之和仍是 ${formatCount(totalWords)} 字。`
                    : "还没有拆分点：确认后目录不变。"}
                </p>
              </div>
            </div>
          </>
        )}

        {submitError ? (
          <p className="meta" role="alert" data-testid="split-submit-error">
            {submitError}
          </p>
        ) : null}

        <div className="confirm-actions">
          <button className="btn btn-primary" type="button" disabled={!ready || busy} data-testid="split-confirm" onClick={() => void submit()}>
            {busy ? "正在拆分…" : ready ? `拆分为 ${pieces.length} 章` : "拆分为多章"}
          </button>
          <button
            className="btn btn-ghost"
            type="button"
            disabled={busy || cuts.size === 0}
            data-testid="split-clear"
            onClick={() => setCuts(new Set())}
          >
            清空拆分点
          </button>
          <button className="btn" type="button" disabled={busy} onClick={onClose}>
            取消
          </button>
        </div>
      </div>
    </div>
  );
}

/** 拆分失败文案：后端把原因写在 message 里（「拆分点超出 0~22」这类），直接展示。 */
function splitErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400) return error.message || "拆分点不合法，调整一下再试。";
    if (error.status === 404) return "这一章已经不在了，刷新页面试试。";
    if (error.kind === "timeout") return "拆分超时了，稍后重试。";
  }
  return "拆分没能完成，稍后再试。";
}
