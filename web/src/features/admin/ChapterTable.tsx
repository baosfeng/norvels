import type { ChapterBrief, Volume } from "../../api/types";
import { volumeLabel } from "../../lib/chapterBatch";
import { formatCount } from "../../lib/format";

/** 章节表上的一行要触发的动作；表格本身不碰 state，全部由 AdminChapters 提供。 */
export interface ChapterRowActions {
  onTogglePick: (id: number) => void;
  onPickRows: (rows: ChapterBrief[], on: boolean) => void;
  onStartEdit: (chapter: ChapterBrief) => void;
  onCancelEdit: () => void;
  onTitleChange: (value: string) => void;
  onSaveTitle: (chapter: ChapterBrief) => void;
  onDelete: (chapter: ChapterBrief) => void;
  onSplit: (chapter: ChapterBrief) => void;
  onLoadMore: (volume: Volume) => void;
}

/**
 * 章节表（勾选列 + 序号 + 标题 + 字数 + 行内操作），按卷分组。
 *
 * 勾选列是独立单元格、点击时 stopPropagation：行内「改标题 / 拆分 / 删除」不该顺手勾中一行。
 * 表头勾选 = 全选/反选当前已载入的行；每卷行第一格再给一个「全选本卷」。
 */
export function ChapterTable({
  volumes,
  rowsByVolume,
  picked,
  shown,
  editing,
  titleDraft,
  savingId,
  filtered,
  titleOf,
  actions,
}: {
  volumes: Volume[];
  rowsByVolume: (volume: Volume) => ChapterBrief[];
  picked: Set<number>;
  shown: Record<number, number>;
  editing: number | null;
  titleDraft: string;
  savingId: number | null;
  filtered: string;
  titleOf: (chapter: ChapterBrief) => string;
  actions: ChapterRowActions;
}) {
  const loaded = volumes.flatMap((volume) => rowsByVolume(volume));
  const loadedHit = loaded.filter((chapter) => picked.has(chapter.id)).length;

  return (
    <div className="chapter-wrap">
      <table className="chapter-table table">
        <thead>
          <tr>
            <th className="c-check" style={{ width: 40 }}>
              <input
                type="checkbox"
                aria-label="全选已载入章节"
                data-testid="admin-chapter-pick-all"
                checked={loaded.length > 0 && loadedHit === loaded.length}
                ref={(node) => {
                  if (node) node.indeterminate = loadedHit > 0 && loadedHit < loaded.length;
                }}
                onChange={(event) => actions.onPickRows(loaded, event.target.checked)}
              />
            </th>
            <th className="is-num" style={{ width: 56 }}>
              序号
            </th>
            <th>标题</th>
            <th className="is-num" style={{ width: 88 }}>
              字数
            </th>
            <th className="c-actions" style={{ width: 190 }}>
              操作
            </th>
          </tr>
        </thead>
        {volumes.map((volume) => {
          const rows = rowsByVolume(volume);
          const hit = rows.filter((chapter) => picked.has(chapter.id)).length;
          return (
            <tbody key={volume.id} data-testid={`admin-volume-${volume.idx}`}>
              <tr className="vol-row">
                <td colSpan={5}>
                  <span className="vol-head">
                    <input
                      type="checkbox"
                      aria-label={`全选「${volumeLabel(volume)}」已载入章节`}
                      data-testid={`admin-chapter-pick-vol-${volume.idx}`}
                      checked={rows.length > 0 && hit === rows.length}
                      ref={(node) => {
                        if (node) node.indeterminate = hit > 0 && hit < rows.length;
                      }}
                      onChange={(event) => actions.onPickRows(rows, event.target.checked)}
                    />
                    <span className="vol-title">{volumeLabel(volume)}</span>
                    <span className="meta num">{formatCount(volume.chapters.length)} 章</span>
                  </span>
                </td>
              </tr>
              {rows.map((chapter) => (
                <tr key={chapter.id} className="ch-row" data-testid={`admin-chapter-${chapter.id}`}>
                  <td className="c-check" onClick={(event) => event.stopPropagation()}>
                    <input
                      type="checkbox"
                      aria-label={`选择第 ${chapter.idx + 1} 章`}
                      data-testid={`admin-chapter-pick-${chapter.id}`}
                      checked={picked.has(chapter.id)}
                      onChange={() => actions.onTogglePick(chapter.id)}
                    />
                  </td>
                  <td className="is-num">{chapter.idx + 1}</td>
                  <td className="c-title">
                    {editing === chapter.id ? (
                      <form
                        className="ch-edit"
                        onSubmit={(event) => {
                          event.preventDefault();
                          actions.onSaveTitle(chapter);
                        }}
                      >
                        <input
                          className="input"
                          value={titleDraft}
                          autoFocus
                          aria-label={`第 ${chapter.idx + 1} 章标题`}
                          data-testid="admin-chapter-title-input"
                          onChange={(event) => actions.onTitleChange(event.target.value)}
                        />
                        <button className="btn btn-sm btn-primary" type="submit" disabled={savingId === chapter.id}>
                          {savingId === chapter.id ? "保存中…" : "保存"}
                        </button>
                        <button className="btn btn-sm" type="button" onClick={actions.onCancelEdit}>
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
                      onClick={() => actions.onStartEdit(chapter)}
                    >
                      改标题
                    </button>
                    <button
                      className="btn btn-sm"
                      type="button"
                      data-testid={`admin-chapter-split-${chapter.id}`}
                      onClick={() => actions.onSplit(chapter)}
                    >
                      拆分
                    </button>
                    <button
                      className="btn btn-sm btn-ghost"
                      type="button"
                      data-testid={`admin-chapter-delete-${chapter.id}`}
                      onClick={() => actions.onDelete(chapter)}
                    >
                      删除
                    </button>
                  </td>
                </tr>
              ))}
              {!filtered && (shown[volume.idx] ?? 0) < volume.chapters.length ? (
                <tr className="more-row">
                  <td colSpan={5}>
                    <button className="btn btn-sm" type="button" onClick={() => actions.onLoadMore(volume)}>
                      载入更多
                    </button>
                    <span className="meta">
                      本卷还有 {formatCount(volume.chapters.length - (shown[volume.idx] ?? 0))} 章
                    </span>
                  </td>
                </tr>
              ) : null}
            </tbody>
          );
        })}
      </table>
    </div>
  );
}
