import { useMemo, useState } from "react";
import { ApiError, api } from "../../api/client";
import type { BookDetail, ChapterBrief, Volume } from "../../api/types";
import { formatCount } from "../../lib/format";
import { SplitChapterDialog } from "./SplitChapterDialog";

/** 每卷默认展开的章数：6000 章的书不能一次铺满 DOM。 */
const PAGE = 20;

/**
 * 管理页的章节表：改标题 / 拆分 / 删除三个动作都在行内，即时生效。
 *
 * 为什么不进「保存」批次：章节结构是事实，不是草稿。把「删了 3 章」和
 * 元信息草稿混在一次保存里，最容易出的错是用户以为删掉的章又被存回去了。
 */
export function AdminChapters({
  book,
  onChanged,
  onError,
}: {
  book: BookDetail;
  onChanged: (message: string) => void;
  onError: (message: string) => void;
}) {
  const volumes = useMemo(() => [...book.volumes].sort((a, b) => a.idx - b.idx), [book.volumes]);
  const [query, setQuery] = useState("");
  const [shown, setShown] = useState<Record<number, number>>(() => initShown(volumes));
  const [editing, setEditing] = useState<number | null>(null);
  const [titleDraft, setTitleDraft] = useState("");
  const [savedTitles, setSavedTitles] = useState<Record<number, string>>({});
  const [savingId, setSavingId] = useState<number | null>(null);
  const [deleting, setDeleting] = useState<ChapterBrief | null>(null);
  const [splitting, setSplitting] = useState<ChapterBrief | null>(null);

  const total = volumes.reduce((sum, volume) => sum + volume.chapters.length, 0);
  const filtered = query.trim().toLowerCase();

  function titleOf(chapter: ChapterBrief): string {
    return savedTitles[chapter.id] ?? chapter.title;
  }

  function startEdit(chapter: ChapterBrief) {
    setEditing(chapter.id);
    setTitleDraft(titleOf(chapter));
  }

  async function saveTitle(chapter: ChapterBrief) {
    const next = titleDraft.trim();
    if (!next) {
      onError("章节标题不能为空。");
      return;
    }
    if (next === titleOf(chapter)) {
      setEditing(null);
      return;
    }
    setSavingId(chapter.id);
    try {
      await api.updateChapter(book.id, chapter.id, next);
      setSavedTitles((prev) => ({ ...prev, [chapter.id]: next }));
      onChanged("章节标题已保存。");
    } catch (error) {
      onError(chapterErrorText(error, "章节标题没能保存"));
    } finally {
      setSavingId(null);
      setEditing(null);
    }
  }

  async function removeChapter(chapter: ChapterBrief) {
    try {
      await api.deleteChapter(book.id, chapter.id);
      setDeleting(null);
      onChanged(`已删除「${titleOf(chapter)}」，后面的章节序号自动补上。`);
    } catch (error) {
      onError(chapterErrorText(error, "这一章没能删除"));
      setDeleting(null);
    }
  }

  return (
    <section className="card admin-chapters" data-testid="admin-chapters">
      <div className="card-head">
        <h2>章节</h2>
        <span className="meta" data-testid="admin-chapters-meta">
          {formatCount(total)} 章 · {volumes.length} 卷
        </span>
      </div>

      <div className="table-toolbar">
        <label className="search-wrap">
          <span className="sr-only">搜索章节</span>
          <input
            className="input"
            type="search"
            value={query}
            placeholder="搜章节标题或章号"
            data-testid="admin-chapter-search"
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        {filtered ? (
          <span className="meta" data-testid="admin-chapter-match">
            匹配 {countMatches(volumes, filtered)} 章
          </span>
        ) : null}
      </div>

      <div className="chapter-wrap">
        <table className="chapter-table table">
          <thead>
            <tr>
              <th className="is-num" style={{ width: 56 }}>
                序号
              </th>
              <th>标题</th>
              <th className="is-num" style={{ width: 88 }}>
                字数
              </th>
              <th className="c-actions" style={{ width: 200 }}>
                操作
              </th>
            </tr>
          </thead>
          {volumes.map((volume) => (
            <tbody key={volume.id} data-testid={`admin-volume-${volume.idx}`}>
              <tr className="vol-row">
                <td colSpan={4}>
                  <span className="vol-title">{volumeLabel(volume)}</span>
                  <span className="meta num">{formatCount(volume.chapters.length)} 章</span>
                </td>
              </tr>
              {rowsOf(volume, filtered, shown[volume.idx] ?? PAGE).map((chapter) => (
                <tr key={chapter.id} className="ch-row" data-testid={`admin-chapter-${chapter.id}`}>
                  <td className="is-num">{chapter.idx + 1}</td>
                  <td className="c-title">
                    {editing === chapter.id ? (
                      <form
                        className="ch-edit"
                        onSubmit={(event) => {
                          event.preventDefault();
                          void saveTitle(chapter);
                        }}
                      >
                        <input
                          className="input"
                          value={titleDraft}
                          autoFocus
                          aria-label={`第 ${chapter.idx + 1} 章标题`}
                          data-testid="admin-chapter-title-input"
                          onChange={(event) => setTitleDraft(event.target.value)}
                        />
                        <button className="btn btn-sm btn-primary" type="submit" disabled={savingId === chapter.id}>
                          {savingId === chapter.id ? "保存中…" : "保存"}
                        </button>
                        <button className="btn btn-sm" type="button" onClick={() => setEditing(null)}>
                          取消
                        </button>
                      </form>
                    ) : (
                      <span className="ch-title-text">{titleOf(chapter)}</span>
                    )}
                  </td>
                  <td className="c-words is-num">{formatCount(chapter.wordCount)}</td>
                  <td className="c-actions">
                    <button
                      className="btn btn-sm"
                      type="button"
                      data-testid={`admin-chapter-edit-${chapter.id}`}
                      onClick={() => startEdit(chapter)}
                    >
                      改标题
                    </button>
                    <button
                      className="btn btn-sm"
                      type="button"
                      data-testid={`admin-chapter-split-${chapter.id}`}
                      onClick={() => setSplitting(chapter)}
                    >
                      拆分
                    </button>
                    <button
                      className="btn btn-sm btn-ghost"
                      type="button"
                      data-testid={`admin-chapter-delete-${chapter.id}`}
                      onClick={() => setDeleting(chapter)}
                    >
                      删除
                    </button>
                  </td>
                </tr>
              ))}
              {!filtered && (shown[volume.idx] ?? PAGE) < volume.chapters.length ? (
                <tr className="more-row">
                  <td colSpan={4}>
                    <button
                      className="btn btn-sm"
                      type="button"
                      onClick={() =>
                        setShown((prev) => ({
                          ...prev,
                          [volume.idx]: Math.min((prev[volume.idx] ?? PAGE) + PAGE, volume.chapters.length),
                        }))
                      }
                    >
                      载入更多
                    </button>
                    <span className="meta">本卷还有 {formatCount(volume.chapters.length - (shown[volume.idx] ?? PAGE))} 章</span>
                  </td>
                </tr>
              ) : null}
            </tbody>
          ))}
        </table>
      </div>
      <p className="meta">
        章节的改标题、拆分、删除都是即时生效，不进下面的「保存」；元信息改动才需要保存。
      </p>

      {deleting ? (
        <ChapterDeleteDialog
          chapter={deleting}
          bookTitle={book.title}
          onCancel={() => setDeleting(null)}
          onConfirm={() => void removeChapter(deleting)}
        />
      ) : null}

      {splitting ? (
        <SplitChapterDialog
          bookId={book.id}
          chapter={splitting}
          onClose={() => setSplitting(null)}
          onSplit={(pieces) => {
            setSplitting(null);
            onChanged(`已拆分为 ${pieces} 章。`);
          }}
        />
      ) : null}
    </section>
  );
}

/** 删除单章的二次确认：默认焦点在「取消」，写明书名 + 章节名。 */
function ChapterDeleteDialog({
  chapter,
  bookTitle,
  onCancel,
  onConfirm,
}: {
  chapter: ChapterBrief;
  bookTitle: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="confirm-backdrop" role="presentation" onClick={onCancel}>
      <div
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="chapter-delete-title"
        data-testid="chapter-delete-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="chapter-delete-title">删除「{chapter.title}」？</h2>
        <p className="meta">
          将从《{bookTitle}》中删除第 {chapter.idx + 1} 章及其正文，后面的章节序号自动补上。无法撤销。
        </p>
        <div className="confirm-actions">
          <button className="btn btn-ghost" type="button" autoFocus data-testid="chapter-delete-cancel" onClick={onCancel}>
            取消
          </button>
          <button className="btn btn-danger" type="button" data-testid="chapter-delete-confirm" onClick={onConfirm}>
            确认删除
          </button>
        </div>
      </div>
    </div>
  );
}

/** 每卷默认展开数：短卷全展开，长卷先给一页。 */
function initShown(volumes: Volume[]): Record<number, number> {
  const map: Record<number, number> = {};
  for (const volume of volumes) map[volume.idx] = Math.min(PAGE, Math.max(PAGE, volume.chapters.length));
  return map;
}

/** 当前要渲染哪些行：搜索时只留命中的（不受分页限制），否则取前 shown 条。 */
function rowsOf(volume: Volume, query: string, shown: number): ChapterBrief[] {
  if (query) {
    return volume.chapters.filter(
      (chapter) =>
        chapter.title.toLowerCase().includes(query) || String(chapter.idx + 1).includes(query),
    );
  }
  return volume.chapters.slice(0, shown);
}

/** 命中的章节数（搜索时显示「匹配 N 章」）。 */
function countMatches(volumes: Volume[], query: string): number {
  return volumes.reduce((sum, volume) => sum + rowsOf(volume, query, PAGE).length, 0);
}

function volumeLabel(volume: Volume): string {
  if (volume.idx < 0) return "未分卷";
  return volume.title || `第 ${volume.idx + 1} 卷`;
}

/** 章节写失败文案：400 的 message 已经是人话（「标题不能为空」等），直接用。 */
function chapterErrorText(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    if (error.status === 400) return error.message || fallback;
    if (error.status === 404) return "这一章已经不在了，刷新页面试试。";
    if (error.kind === "timeout") return `${fallback}（超时），稍后重试。`;
  }
  return `${fallback}，稍后再试。`;
}
