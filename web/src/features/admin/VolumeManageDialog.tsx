import { useState } from "react";
import { api } from "../../api/client";
import type { Volume } from "../../api/types";
import {
  checkVolumeName,
  realVolumes,
  reorderPreview,
  volumeDeleteHint,
  volumeErrorText,
  volumeMetaText,
} from "../../lib/volumeOps";
import { bumpStatsRevision } from "../../lib/statsRevision";

type Busy = null | "create" | "delete" | "order" | "rename";

/**
 * 卷管理弹窗：重命名（内联）/ 上移 / 下移 / 删除 / 新建。
 *
 * 所有写操作成功后由父组件重拉详情（不整页刷新），弹窗拿到新的 volumes 重新渲染——
 * 所以这里不做乐观更新：界面始终是服务端真相，失败时天然不需要回滚，
 * 只在弹窗内就地给出失败原因（失败原因贴着操作，比页面顶部横幅好找）。
 */
export function VolumeManageDialog({
  bookId,
  volumes,
  onSaved,
  onClose,
}: {
  bookId: number;
  volumes: Volume[];
  /** 写成功：父组件用它弹横幅 + 重拉详情 */
  onSaved: (message: string) => void;
  onClose: () => void;
}) {
  const ordered = realVolumes(volumes);
  const [busy, setBusy] = useState<Busy>(null);
  const [errorText, setErrorText] = useState("");
  const [newName, setNewName] = useState("");
  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [nameDraft, setNameDraft] = useState("");
  const [pendingDelete, setPendingDelete] = useState<Volume | null>(null);

  function startRename(volume: Volume) {
    setRenamingId(volume.id);
    setNameDraft(volume.title);
    setErrorText("");
  }

  async function create() {
    const check = checkVolumeName(volumes, newName);
    if (!check.ok) {
      setErrorText(check.message);
      return;
    }
    setBusy("create");
    setErrorText("");
    try {
      await api.createVolume(bookId, check.name);
      bumpStatsRevision();
      setNewName("");
      onSaved(`已新建空卷「${check.name}」`);
    } catch (error) {
      setErrorText(volumeErrorText(error));
    } finally {
      setBusy(null);
    }
  }

  async function rename(volume: Volume) {
    const check = checkVolumeName(volumes, nameDraft, volume.id);
    if (!check.ok) {
      setErrorText(check.message);
      return;
    }
    if (check.name === volume.title) {
      setRenamingId(null);
      setErrorText("");
      return;
    }
    setBusy("rename");
    setErrorText("");
    try {
      await api.renameVolume(bookId, volume.id, check.name);
      bumpStatsRevision();
      setRenamingId(null);
      onSaved(`卷名已改为「${check.name}」`);
    } catch (error) {
      setErrorText(volumeErrorText(error));
    } finally {
      setBusy(null);
    }
  }

  async function reorder(volume: Volume, dir: -1 | 1) {
    const ids = reorderPreview(volumes, volume.id, dir);
    if (!ids) return;
    setBusy("order");
    setErrorText("");
    try {
      await api.reorderVolumes(bookId, ids);
      bumpStatsRevision();
      onSaved(dir < 0 ? `「${volume.title}」已上移。` : `「${volume.title}」已下移。`);
    } catch (error) {
      setErrorText(volumeErrorText(error));
    } finally {
      setBusy(null);
    }
  }

  async function remove(volume: Volume) {
    setBusy("delete");
    setErrorText("");
    try {
      await api.deleteVolume(bookId, volume.id);
      bumpStatsRevision();
      setPendingDelete(null);
      onSaved(`已删除「${volume.title}」，${volume.chapters.length} 章回到「未分卷」。`);
    } catch (error) {
      setErrorText(volumeErrorText(error));
      setPendingDelete(null);
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
      <div className="confirm-backdrop" role="presentation" onClick={onClose}>
        <div
          className="confirm-dialog volume-dialog"
          role="dialog"
          aria-modal="true"
          aria-labelledby="volume-manage-title"
          data-testid="volume-manage-dialog"
          onClick={(event) => event.stopPropagation()}
        >
          <h2 id="volume-manage-title">卷管理</h2>
          <p className="meta">
            共 <strong className="num" data-testid="volume-manage-count">{ordered.length}</strong> 卷 · 卷名同书唯一，改完即时生效。
          </p>

          <div className="vol-manage" data-testid="volume-manage-list">
            {ordered.length === 0 ? <p className="meta" data-testid="volume-manage-empty">还没有卷，新建一个再往里面放章节。</p> : null}
            {ordered.map((volume) => (
              <div className="vol-manage-row" key={volume.id} data-testid={`volume-row-${volume.id}`}>
                {renamingId === volume.id ? (
                  <>
                    <input
                      className="input input-sm"
                      value={nameDraft}
                      autoFocus
                      aria-label={`「${volume.title}」的新卷名`}
                      data-testid="volume-rename-input"
                      onChange={(event) => setNameDraft(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") void rename(volume);
                        if (event.key === "Escape") setRenamingId(null);
                      }}
                    />
                    <div className="vol-manage-acts">
                      <button
                        className="btn btn-sm btn-primary"
                        type="button"
                        disabled={busy !== null}
                        data-testid="volume-rename-save"
                        onClick={() => void rename(volume)}
                      >
                        {busy === "rename" ? "保存中…" : "保存"}
                      </button>
                      <button className="btn btn-sm" type="button" data-testid="volume-rename-cancel" onClick={() => setRenamingId(null)}>
                        取消
                      </button>
                    </div>
                  </>
                ) : (
                  <>
                    <span className="vol-manage-name" data-testid={`volume-name-${volume.id}`}>
                      {volume.title || `第 ${volume.idx + 1} 卷`}
                    </span>
                    <span className="meta num" data-testid={`volume-meta-${volume.id}`}>{volumeMetaText(volume)}</span>
                    <div className="vol-manage-acts">
                      <button
                        className="btn btn-sm"
                        type="button"
                        data-testid={`volume-rename-open-${volume.id}`}
                        onClick={() => startRename(volume)}
                      >
                        重命名
                      </button>
                      <button
                        className="btn btn-sm"
                        type="button"
                        disabled={busy !== null || reorderPreview(volumes, volume.id, -1) === null}
                        data-testid={`volume-up-${volume.id}`}
                        onClick={() => void reorder(volume, -1)}
                      >
                        上移
                      </button>
                      <button
                        className="btn btn-sm"
                        type="button"
                        disabled={busy !== null || reorderPreview(volumes, volume.id, 1) === null}
                        data-testid={`volume-down-${volume.id}`}
                        onClick={() => void reorder(volume, 1)}
                      >
                        下移
                      </button>
                      <button
                        className="btn btn-sm btn-ghost"
                        type="button"
                        data-testid={`volume-delete-${volume.id}`}
                        onClick={() => {
                          setErrorText("");
                          setPendingDelete(volume);
                        }}
                      >
                        删除
                      </button>
                    </div>
                  </>
                )}
              </div>
            ))}
          </div>

          <div className="vol-new">
            <input
              className="input input-sm"
              value={newName}
              placeholder="新卷名，如 卷四 · 归途"
              aria-label="新卷名"
              data-testid="volume-new-input"
              onChange={(event) => setNewName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") void create();
              }}
            />
            <button
              className="btn btn-sm btn-primary"
              type="button"
              disabled={busy !== null}
              data-testid="volume-create"
              onClick={() => void create()}
            >
              {busy === "create" ? "新建中…" : "新建卷"}
            </button>
          </div>

          <p className="meta">
            新建出来是空卷，章节要在章节表里勾选后用「移动到卷…」移进去。删卷不会删章节，卷内章节会回到「未分卷」。
          </p>

          {errorText ? (
            <p className="meta volume-error" role="alert" data-testid="volume-error">
              {errorText}
            </p>
          ) : null}

          <div className="confirm-actions">
            <button className="btn" type="button" data-testid="volume-manage-close" onClick={onClose}>
              完成
            </button>
          </div>
        </div>
      </div>

      {pendingDelete ? (
        <VolumeDeleteConfirm
          volume={pendingDelete}
          busy={busy !== null}
          onCancel={() => setPendingDelete(null)}
          onConfirm={() => void remove(pendingDelete)}
        />
      ) : null}
    </>
  );
}

/** 删卷二次确认：写明卷内几章会回到「未分卷」，默认焦点在「取消」。 */
function VolumeDeleteConfirm({
  volume,
  busy,
  onCancel,
  onConfirm,
}: {
  volume: Volume;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="confirm-backdrop" role="presentation" onClick={onCancel}>
      <div
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="volume-delete-title"
        data-testid="volume-delete-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="volume-delete-title">删除「{volume.title}」？</h2>
        <p className="meta" data-testid="volume-delete-text">
          {volumeDeleteHint(volume)}章节正文都还在，但卷名与这个卷本身会消失，无法撤销。
        </p>
        <div className="confirm-actions">
          <button className="btn btn-ghost" type="button" autoFocus data-testid="volume-delete-cancel" onClick={onCancel}>
            取消
          </button>
          <button className="btn btn-danger" type="button" disabled={busy} data-testid="volume-delete-confirm" onClick={onConfirm}>
            删除卷
          </button>
        </div>
      </div>
    </div>
  );
}
