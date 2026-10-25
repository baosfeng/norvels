import { useMemo, useState } from "react";
import { ApiError, api } from "../../api/client";
import type { ChapterBrief, Volume } from "../../api/types";
import { formatCount } from "../../lib/format";
import { bumpStatsRevision } from "../../lib/statsRevision";

/**
 * 新建章节：标题 + 正文 + 可选「插到某章之后」（缺省追加到末尾）。
 *
 * 走 POST /books/{id}/chapters 一次请求：正文只存在于这个面板里，失败必须留在面板上
 * 并把后端的人话原样带出来（空正文 / 超 1 MiB 都是 400 invalid_param），
 * 不能像元信息那样存在页面草稿里——否则用户点「保存」失败后以为已经写进去了。
 */
export function NewChapterDialog({
  bookId,
  volumes,
  onClose,
  onCreated,
  onFailed,
}: {
  bookId: number;
  volumes: Volume[];
  onClose: () => void;
  /** 成功回传新章（父组件负责刷新目录、左栏统计与提示） */
  onCreated: (chapter: ChapterBrief) => void;
  /** 失败时通知外层弹一条 toast：面板在弹窗里，页面横幅看不见 */
  onFailed?: (message: string) => void;
}) {
  const chapters = useMemo(
    () => volumes.flatMap((volume) => volume.chapters).sort((a, b) => a.idx - b.idx),
    [volumes],
  );
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  /** "" = 追加到末尾，否则是「插在这一章之后」的 chapterId */
  const [after, setAfter] = useState("");
  const [busy, setBusy] = useState(false);
  const [errorText, setErrorText] = useState("");

  async function submit() {
    if (busy) return;
    const nextTitle = title.trim();
    if (!nextTitle) {
      setErrorText("章节标题不能为空。");
      return;
    }
    if (content.trim() === "") {
      setErrorText("正文不能为空：空章节没有任何意义，先写点内容。");
      return;
    }
    setBusy(true);
    setErrorText("");
    try {
      const created = await api.createChapter(bookId, {
        title: nextTitle,
        content,
        ...(after === "" ? {} : { afterChapterId: Number(after) }),
      });
      // 章数与字数都变了：左栏统计与侧栏库存量一起刷新
      bumpStatsRevision();
      onCreated(created);
    } catch (error) {
      const message = createErrorText(error);
      setErrorText(message);
      onFailed?.(message);
      setBusy(false);
    }
  }

  return (
    <div className="confirm-backdrop" role="presentation" onClick={() => !busy && onClose()}>
      <div
        className="confirm-dialog is-wide"
        role="dialog"
        aria-modal="true"
        aria-labelledby="new-chapter-title"
        data-testid="new-chapter-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="new-chapter-title">新建章节</h2>

        <div className="field">
          <label htmlFor="new-chapter-name">章节标题</label>
          <input
            className="input"
            id="new-chapter-name"
            value={title}
            disabled={busy}
            placeholder="例如 第一章 开端"
            data-testid="new-chapter-name"
            onChange={(event) => {
              setTitle(event.target.value);
              setErrorText("");
            }}
          />
        </div>

        <div className="field">
          <label htmlFor="new-chapter-after">插入位置</label>
          <select
            className="select"
            id="new-chapter-after"
            value={after}
            disabled={busy}
            data-testid="new-chapter-after"
            onChange={(event) => setAfter(event.target.value)}
          >
            <option value="">追加到末尾</option>
            {chapters.map((chapter) => (
              <option key={chapter.id} value={chapter.id}>
                插在第 {chapter.idx + 1} 章「{chapter.title}」之后
              </option>
            ))}
          </select>
          <span className="field-hint">
            全书 {formatCount(chapters.length)} 章；插在中间时后面的章节序号由后端顺延。
          </span>
        </div>

        <div className="field">
          <label htmlFor="new-chapter-content">正文</label>
          <textarea
            className="input"
            id="new-chapter-content"
            rows={12}
            value={content}
            disabled={busy}
            placeholder="空行分段，纯文本；单个章节最多 1 MiB"
            spellCheck={false}
            data-testid="new-chapter-content"
            onChange={(event) => {
              setContent(event.target.value);
              setErrorText("");
            }}
          />
        </div>

        {errorText ? (
          <p className="meta" role="alert" data-testid="new-chapter-error">
            {errorText}
          </p>
        ) : null}

        <div className="confirm-actions">
          <button className="btn btn-primary" type="button" disabled={busy} data-testid="new-chapter-save" onClick={() => void submit()}>
            {busy ? "正在新建…" : "新建章节"}
          </button>
          <button className="btn btn-ghost" type="button" disabled={busy} data-testid="new-chapter-cancel" onClick={onClose}>
            取消
          </button>
        </div>
      </div>
    </div>
  );
}

/** 失败文案：400 的 message 后端已写成人话（含 1 MiB 上限的具体值），直接展示。 */
function createErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400) return error.message || "这一章没通过检查，检查一下标题与正文。";
    if (error.status === 404) return "这本书已经不在了，刷新页面试试。";
    if (error.kind === "timeout") return "新建超时了，正文还留在面板里，稍后再点一次。";
  }
  return "章节没能新建，正文还留在面板里，稍后再试。";
}
