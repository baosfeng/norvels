import { useMemo, useState } from "react";
import { ApiError, api } from "../../api/client";
import type { BookDetail, ChapterBrief, Volume } from "../../api/types";
import { CHAPTER_PAGE, mergeVerdict, rowsOf } from "../../lib/chapterBatch";
import { formatCount, formatWords } from "../../lib/format";
import { bumpStatsRevision } from "../../lib/statsRevision";
import { showToast } from "../../lib/toastStore";
import { isLooseVolume, realVolumes } from "../../lib/volumeOps";
import { ChapterBatchBar } from "./ChapterBatchBar";
import { ChapterBodyDrawer } from "./ChapterBodyDrawer";
import { ChapterBatchDeleteDialog } from "./ChapterBatchDeleteDialog";
import { ChapterMergeDialog } from "./ChapterMergeDialog";
import { ChapterRenumberDialog } from "./ChapterRenumberDialog";
import { ChapterTable } from "./ChapterTable";
import { NewChapterDialog } from "./NewChapterDialog";
import { SplitChapterDialog } from "./SplitChapterDialog";
import { VolumeManageDialog } from "./VolumeManageDialog";
import { VolumeMoveDialog } from "./VolumeMoveDialog";

/**
 * 管理页的章节区：行内改标题 / 拆分 / 删除，加三处结构操作——勾选批量删除、
 * 同卷相邻合并、按序号重命名。这个组件只做编排，表格在 ChapterTable，各面板各一个文件。
 *
 * 为什么不进「保存」批次：章节结构是事实，不是草稿。把「删了 3 章」和
 * 元信息草稿混在一次保存里，最容易出的错是用户以为删掉的章又被存回去了。
 * 每个结构操作成功后都由父组件 reload 重新拉目录，不做乐观更新。
 */
export function AdminChapters({
  book,
  onChanged,
  onError,
  onReload,
}: {
  book: BookDetail;
  onChanged: (message: string) => void;
  onError: (message: string) => void;
  /**
   * 只重拉详情、不弹横幅。正文抽屉保存后走这条：反馈用 toast（原型如此），
   * 页面上的横幅留给元信息与其他结构操作。
   */
  onReload?: () => void;
}) {
  const volumes = useMemo(() => [...book.volumes].sort((a, b) => a.idx - b.idx), [book.volumes]);
  const [query, setQuery] = useState("");
  const [shown, setShown] = useState<Record<number, number>>(() =>
    Object.fromEntries(volumes.map((volume) => [volume.idx, CHAPTER_PAGE])),
  );
  const [editing, setEditing] = useState<number | null>(null);
  const [titleDraft, setTitleDraft] = useState("");
  const [savedTitles, setSavedTitles] = useState<Record<number, string>>({});
  const [savingId, setSavingId] = useState<number | null>(null);
  const [deleting, setDeleting] = useState<ChapterBrief | null>(null);
  const [splitting, setSplitting] = useState<ChapterBrief | null>(null);
  /** 正在改正文的那一章（null = 抽屉关着） */
  const [bodyEditing, setBodyEditing] = useState<ChapterBrief | null>(null);
  /** 勾选按 chapterId 集合维护：跨卷、跨「载入更多」都是同一套 */
  const [picked, setPicked] = useState<Set<number>>(() => new Set());
  const [batchDeleting, setBatchDeleting] = useState(false);
  const [merging, setMerging] = useState(false);
  const [renaming, setRenaming] = useState(false);
  /** 卷管理弹窗（新建 / 重命名 / 上移下移 / 删除） */
  const [managingVolumes, setManagingVolumes] = useState(false);
  /** 「移动到卷」弹窗（勾选的章跨卷移动） */
  const [movingToVolume, setMovingToVolume] = useState(false);
  /** 新建章节面板（标题 + 正文 + 插入位置） */
  const [creating, setCreating] = useState(false);

  const real = useMemo(() => realVolumes(volumes), [volumes]);
  const loose = useMemo(() => volumes.find(isLooseVolume) ?? null, [volumes]);
  const total = volumes.reduce((sum, volume) => sum + volume.chapters.length, 0);
  const filtered = query.trim().toLowerCase();
  const matchCount = countMatches(volumes, filtered);

  // 勾选的章一律从当前目录里取：删除/合并后 reload 换掉目录，旧 id 自然消失
  const pickedList = useMemo(
    () =>
      volumes
        .flatMap((volume) => volume.chapters)
        .filter((chapter) => picked.has(chapter.id))
        .sort((a, b) => a.idx - b.idx),
    [volumes, picked],
  );
  const verdict = useMemo(() => mergeVerdict(pickedList), [pickedList]);
  const currentVolumeId = pickedList[0]?.volumeId ?? volumes[0]?.id ?? 0;

  function rowsByVolume(volume: Volume): ChapterBrief[] {
    return rowsOf(volume, filtered, shown[volume.idx] ?? CHAPTER_PAGE);
  }

  function titleOf(chapter: ChapterBrief): string {
    return savedTitles[chapter.id] ?? chapter.title;
  }

  function pickRows(rows: ChapterBrief[], on: boolean) {
    setPicked((prev) => {
      const next = new Set(prev);
      for (const row of rows) {
        if (on) next.add(row.id);
        else next.delete(row.id);
      }
      return next;
    });
  }

  function togglePick(id: number) {
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
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
      bumpStatsRevision();
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
          {formatCount(total)} 章 · {real.length} 卷
          {loose ? ` · 未分卷 ${formatCount(loose.chapters.length)} 章` : ""}
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
            data-od-id="C-53"
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        {filtered ? (
          <span className="meta" data-testid="admin-chapter-match">
            匹配 {matchCount} 章
          </span>
        ) : null}
        <button
          className="btn btn-sm toolbar-end"
          type="button"
          data-testid="admin-chapter-new-open"
          data-od-id="C-54"
          onClick={() => setCreating(true)}
        >
          新建章节…
        </button>
        <button
          className="btn btn-sm"
          type="button"
          data-testid="admin-chapter-rename-open"
          data-od-id="C-55"
          disabled={total === 0}
          onClick={() => setRenaming(true)}
        >
          按序号重命名…
        </button>
        <button
          className="btn btn-sm"
          type="button"
          data-testid="admin-volume-manage-open"
          data-od-id="C-56"
          onClick={() => setManagingVolumes(true)}
        >
          卷管理…
        </button>
      </div>
      {total === 0 ? (
        <p className="inline-note" data-testid="admin-chapter-none">
          这本书还没有章节
        </p>
      ) : null}
      {filtered && matchCount === 0 ? (
        <p className="inline-note" data-testid="admin-chapter-nomatch">
          没有匹配的章节
        </p>
      ) : null}

      <ChapterBatchBar
        count={pickedList.length}
        mergeReason={verdict.reason}
        onMerge={() => setMerging(true)}
        onMove={() => setMovingToVolume(true)}
        onDelete={() => setBatchDeleting(true)}
        onClear={() => setPicked(new Set())}
      />

      <ChapterTable
        volumes={volumes}
        rowsByVolume={rowsByVolume}
        picked={picked}
        editing={editing}
        titleDraft={titleDraft}
        savingId={savingId}
        filtered={filtered}
        titleOf={titleOf}
        actions={{
          onTogglePick: togglePick,
          onPickRows: pickRows,
          onStartEdit: (chapter) => {
            setEditing(chapter.id);
            setTitleDraft(titleOf(chapter));
          },
          onCancelEdit: () => setEditing(null),
          onTitleChange: setTitleDraft,
          onSaveTitle: (chapter) => void saveTitle(chapter),
          onEditBody: setBodyEditing,
          onDelete: setDeleting,
          onSplit: setSplitting,
          onLoadMore: (volume) =>
            setShown((prev) => ({
              ...prev,
              [volume.idx]: Math.min((prev[volume.idx] ?? CHAPTER_PAGE) + CHAPTER_PAGE, volume.chapters.length),
            })),
        }}
      />

      <p className="meta">
        勾选后可批量删除、合并、移动到其他卷；同卷相邻的章可以合并；也可以按序号重命名。改动都即时生效，不进下面的「保存」。
        卷的新建 / 重命名 / 排序 / 删除在「卷管理」里。
      </p>

      {creating ? (
        <NewChapterDialog
          bookId={book.id}
          volumes={volumes}
          onClose={() => setCreating(false)}
          onCreated={(chapter) => {
            setCreating(false);
            // 反馈走 toast（与正文抽屉一致）：目录与左栏统计由 reload 换新
            showToast(`已新建章节「${chapter.title}」`);
            if (onReload) onReload();
            else onChanged(`已新建章节「${chapter.title}」。`);
          }}
          onFailed={(message) => showToast(message, "error")}
        />
      ) : null}

      {deleting ? (
        <ChapterDeleteDialog
          chapter={deleting}
          bookTitle={book.title}
          onCancel={() => setDeleting(null)}
          onConfirm={() => void removeChapter(deleting)}
        />
      ) : null}

      {bodyEditing ? (
        <ChapterBodyDrawer
          bookId={book.id}
          chapter={bodyEditing}
          onClose={() => setBodyEditing(null)}
          onSaved={({ words }) => {
            setBodyEditing(null);
            // 字数只是本地估算，真值在重拉后的目录里；toast 里仍用它，因为用户要的是「刚才改了多大」
            showToast(`正文已保存 · 第 ${bodyEditing.idx + 1} 章 ${formatWords(words)}`);
            if (onReload) onReload();
            else onChanged("正文已保存。");
          }}
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

      {batchDeleting ? (
        <ChapterBatchDeleteDialog
          bookId={book.id}
          bookTitle={book.title}
          chapters={pickedList}
          onClose={() => setBatchDeleting(false)}
          onDeleted={(count) => {
            setBatchDeleting(false);
            setPicked(new Set());
            onChanged(`已删除 ${formatCount(count)} 章，后面的章节序号自动补上。`);
          }}
        />
      ) : null}

      {merging ? (
        <ChapterMergeDialog
          bookId={book.id}
          chapters={pickedList}
          totalChapters={total}
          onClose={() => setMerging(false)}
          onMerged={(result) => {
            const count = pickedList.length;
            setMerging(false);
            setPicked(new Set());
            onChanged(`已把 ${count} 章合并为 1 章，全书现在 ${formatCount(result.chapters)} 章。`);
            if (!result.titleSaved) onError("合并已完成，但新标题没能保存，可以再用「改标题」改一次。");
          }}
        />
      ) : null}

      {renaming ? (
        <ChapterRenumberDialog
          bookId={book.id}
          volumes={volumes}
          defaultVolumeId={currentVolumeId}
          onClose={() => setRenaming(false)}
          onRenumbered={(renamed) => {
            setRenaming(false);
            setPicked(new Set());
            onChanged(`已按序号重命名 ${formatCount(renamed)} 章。`);
          }}
        />
      ) : null}

      {managingVolumes ? (
        <VolumeManageDialog
          bookId={book.id}
          volumes={volumes}
          onSaved={onChanged}
          onClose={() => setManagingVolumes(false)}
        />
      ) : null}

      {movingToVolume ? (
        <VolumeMoveDialog
          bookId={book.id}
          volumes={volumes}
          chapters={pickedList}
          onClose={() => setMovingToVolume(false)}
          onMoved={({ moved, volumeTitle }) => {
            setMovingToVolume(false);
            setPicked(new Set());
            onChanged(`已移动 ${formatCount(moved)} 章到「${volumeTitle}」。`);
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

/** 命中的章节数（搜索时显示「匹配 N 章」）。 */
function countMatches(volumes: Volume[], query: string): number {
  return volumes.reduce((sum, volume) => sum + rowsOf(volume, query, CHAPTER_PAGE).length, 0);
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
