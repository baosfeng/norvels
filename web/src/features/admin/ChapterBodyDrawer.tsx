import { useState } from "react";
import { ApiError, api } from "../../api/client";
import { useChapter } from "../../api/hooks";
import type { ChapterBrief } from "../../api/types";
import { bodyIsDirty, bodyStateText, countBodyParagraphs, countBodyWords } from "../../lib/chapterBody";
import { renamedTitle } from "../../lib/chapterBatch";
import { formatWords } from "../../lib/format";
import { bumpStatsRevision } from "../../lib/statsRevision";

/**
 * 章节正文抽屉：从右侧滑入盖住右栏（可切全屏），改完点「保存」才下发。
 *
 * 正文是**纯文本整章替换**（PUT …/content，204）：不做乐观更新，保存成功由
 * AdminChapters 关抽屉并重拉目录，字数以服务端 wordCount 为准。
 *
 * 三条刻意的取舍：
 *   - 草稿放在 `draft: string | null`：null 表示没动过，显示值直接跟服务端正文走，
 *     所以失败重试拿回新正文时不会被旧草稿盖住；一旦动手就整段保留。
 *   - 有未保存改动时关闭要拦一次（「继续编辑 / 放弃修改」），因为这一版正文只存在于
 *     浏览器里，关掉就是真的没了。
 *   - 取不到正文（404 / 网络错）在抽屉里给错误态 + 重试，不留空白抽屉。
 */
export function ChapterBodyDrawer({
  bookId,
  chapter,
  onClose,
  onSaved,
}: {
  bookId: number;
  chapter: Pick<ChapterBrief, "id" | "idx" | "title" | "wordCount">;
  onClose: () => void;
  /** 保存成功后回传本章本地字数（父组件负责 toast + 重拉目录） */
  onSaved: (summary: { words: number }) => void;
}) {
  const { data, loading, error, reload } = useChapter(bookId, chapter.id);
  const [draft, setDraft] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [full, setFull] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [errorText, setErrorText] = useState("");

  const savedText = data?.content ?? "";
  const text = draft ?? savedText;
  const dirty = data !== null && bodyIsDirty(text, savedText);

  async function save() {
    if (busy || !data) return;
    // 空正文后端也会 400，但让用户先在自己按下保存的地方得到提示，不必绕一圈网络
    if (text.trim() === "") {
      setErrorText("正文不能为空：先写点内容，或者用「删除」把这一章去掉。");
      return;
    }
    setBusy(true);
    setErrorText("");
    try {
      await api.updateChapterContent(bookId, chapter.id, text);
      // 本章字数变了 → 左栏「章节 / 字数」要跟着重取（见 lib/statsRevision）
      bumpStatsRevision();
      onSaved({ words: countBodyWords(text) });
    } catch (error) {
      setErrorText(saveErrorText(error));
      setBusy(false);
    }
  }

  function requestClose() {
    if (busy) return;
    if (dirty) {
      setLeaving(true);
      return;
    }
    onClose();
  }

  return (
    <>
      <aside
        className={`ch-editor${full ? " is-full" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby="chapter-body-title"
        data-testid="chapter-body-drawer"
      >
        <div className="ch-editor-head">
          <div className="ch-editor-head-text">
            <p className="eyebrow">正文编辑</p>
            <h2 id="chapter-body-title" data-testid="chapter-body-title">
              {renamedTitle(chapter.title, chapter.idx + 1)}
            </h2>
            <p className="meta" data-testid="chapter-body-meta">
              本章 <b data-testid="chapter-body-words">{formatWords(countBodyWords(text))}</b> ·{" "}
              <span data-testid="chapter-body-paras">{countBodyParagraphs(text)}</span> 段 ·{" "}
              <span data-testid="chapter-body-state">{bodyStateText(dirty)}</span>
            </p>
          </div>
          <div className="ch-editor-tools">
            <button
              className="btn btn-sm"
              type="button"
              aria-pressed={full}
              data-testid="chapter-body-full"
              data-od-id="C-71"
              disabled={!data}
              onClick={() => setFull((prev) => !prev)}
            >
              {full ? "退出全屏" : "全屏"}
            </button>
            <button
              className="ch-editor-x"
              type="button"
              aria-label="关闭正文编辑"
              data-testid="chapter-body-close"
              onClick={requestClose}
            >
              ×
            </button>
          </div>
        </div>
        <div>
          {!data ? <p className="inline-note">等正文读完就能全屏</p> : null}
        </div>

        <div className="ch-editor-notes" data-testid="chapter-body-notes">
          <p>保存会替换本章正文，正文长度变化会影响本章字数。</p>
          <p>本章已存的书签位置可能偏移。</p>
        </div>

        {loading ? (
          <p className="meta ch-editor-state" data-testid="chapter-body-loading">
            正在取回本章正文…
          </p>
        ) : null}

        {error ? (
          <div className="ch-editor-state is-error" role="alert" data-testid="chapter-body-error">
            <p>{loadErrorText(error)}</p>
            <button className="btn btn-sm" type="button" data-testid="chapter-body-retry" onClick={reload}>
              重试
            </button>
          </div>
        ) : null}

        {data ? (
          <textarea
            className="ch-editor-area"
            aria-label="本章正文，空行分段"
            spellCheck={false}
            data-testid="chapter-body-input"
            value={text}
            onChange={(event) => {
              setDraft(event.target.value);
              setErrorText("");
            }}
          />
        ) : null}

        {errorText ? (
          <p className="meta ch-editor-error" role="alert" data-testid="chapter-body-save-error">
            {errorText}
          </p>
        ) : null}

        <div className="ch-editor-foot">
          <span className="meta">空行分段 · 纯文本，不做富文本格式</span>
          <button className="btn" type="button" disabled={busy} data-testid="chapter-body-cancel" onClick={requestClose}>
            取消
          </button>
          <button
            className="btn btn-primary"
            type="button"
            disabled={busy || !data}
            data-testid="chapter-body-save"
            data-od-id="C-72"
            onClick={() => void save()}
          >
            {busy ? "保存中…" : "保存"}
          </button>
        </div>
      </aside>

      {leaving ? (
        <div className="confirm-backdrop" role="presentation" onClick={() => setLeaving(false)}>
          <div
            className="confirm-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="chapter-body-leave-title"
            data-testid="chapter-body-leave"
            onClick={(event) => event.stopPropagation()}
          >
            <h2 id="chapter-body-leave-title">本章正文还没保存</h2>
            <p className="meta">关掉正文面板会丢掉刚才改的这一版。</p>
            <p className="meta">离开后正文回到上次保存的版本，改过的内容不会留在书里。</p>
            <div className="confirm-actions">
              <button
                className="btn btn-primary"
                type="button"
                autoFocus
                data-testid="chapter-body-leave-stay"
                onClick={() => setLeaving(false)}
              >
                继续编辑
              </button>
              <button className="btn btn-ghost" type="button" data-testid="chapter-body-leave-discard" onClick={onClose}>
                放弃修改
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}

/** 保存失败文案：400 的 message 后端已经写成人话（空正文 / 超 1 MiB 的具体上限），直接用。 */
function saveErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400) return error.message || "这段正文没能通过检查，检查一下再保存。";
    if (error.status === 404) return "这一章已经不在了，关掉面板刷新页面。";
    if (error.kind === "timeout") return "保存超时了，稍后再试。";
  }
  return "正文没能保存，稍后再试。";
}

/** 取正文失败文案：404 说明章没了，其余按网络 / 超时给可重试的提示。 */
function loadErrorText(error: ApiError): string {
  if (error.status === 404) return "这一章已经不在了，关掉面板刷新页面。";
  if (error.kind === "timeout") return "取正文超时了，重试一次。";
  return "正文没能取回来，检查一下网络再重试。";
}
