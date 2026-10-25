import { useState } from "react";
import { api } from "../../api/client";
import type { ChapterBrief, Volume } from "../../api/types";
import { formatCount } from "../../lib/format";
import {
  LOOSE_VOLUME_ID,
  LOOSE_VOLUME_TITLE,
  moveHint,
  movePlan,
  realVolumes,
  volumeErrorText,
  volumeMetaText,
} from "../../lib/volumeOps";

/**
 * 「移动到卷」弹窗：把勾选的章节移到目标卷末尾（volumeId=0 = 未分卷）。
 *
 * 目标卷是单选列表而不是原生 select：每项要带「N 章 · 区间」，
 * 只看卷名选目标容易选错（同名卷在前端就被挡了，但卷名相近是常态）。
 */
export function VolumeMoveDialog({
  bookId,
  volumes,
  chapters,
  onClose,
  onMoved,
}: {
  bookId: number;
  volumes: Volume[];
  /** 已勾选的章节（已按章号排好） */
  chapters: ChapterBrief[];
  onClose: () => void;
  onMoved: (result: { moved: number; volumeId: number; volumeTitle: string }) => void;
}) {
  const targets = [...realVolumes(volumes), looseTarget(volumes)];
  const [targetId, setTargetId] = useState<number>(() => targets[0]?.id ?? LOOSE_VOLUME_ID);
  const [busy, setBusy] = useState(false);
  const [errorText, setErrorText] = useState("");

  const target = targets.find((volume) => volume.id === targetId) ?? targets[0];
  const targetTitle = target && target.id !== LOOSE_VOLUME_ID ? target.title : LOOSE_VOLUME_TITLE;
  const plan = movePlan(chapters, targetId);

  async function confirm() {
    if (busy) return;
    if (plan.moved === 0) {
      setErrorText(`已选的 ${formatCount(chapters.length)} 章已经在这个卷里了。`);
      return;
    }
    setBusy(true);
    setErrorText("");
    try {
      const result = await api.moveChapters(bookId, chapters.map((chapter) => chapter.id), targetId);
      onMoved({ moved: result.moved, volumeId: targetId, volumeTitle: targetTitle });
    } catch (error) {
      setErrorText(volumeErrorText(error));
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
        className="confirm-dialog volume-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="volume-move-title"
        data-testid="volume-move-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="volume-move-title">移动到卷</h2>
        <p className="meta" data-testid="volume-move-sub">{moveHint(targetTitle, plan, chapters.length)}</p>

        <div className="vol-move-list" role="radiogroup" aria-label="目标卷">
          {targets.map((volume) => (
            <label className="vol-move-opt" key={volume.id} data-testid={`volume-move-opt-${volume.id}`}>
              <input
                type="radio"
                name="volume-move-target"
                value={volume.id}
                checked={targetId === volume.id}
                onChange={() => {
                  setTargetId(volume.id);
                  setErrorText("");
                }}
              />
              <span>{volume.id === LOOSE_VOLUME_ID ? LOOSE_VOLUME_TITLE : volume.title}</span>
              <span className="meta">{volumeMetaText(volume)}</span>
            </label>
          ))}
        </div>

        <p className="meta">章节会排在目标卷末尾；全书章号、正文与字数都不变，只是归属的卷变了。</p>

        {errorText ? (
          <p className="meta volume-error" role="alert" data-testid="volume-move-error">
            {errorText}
          </p>
        ) : null}

        <div className="confirm-actions">
          <button className="btn btn-ghost" type="button" autoFocus disabled={busy} data-testid="volume-move-cancel" onClick={onClose}>
            取消
          </button>
          <button className="btn btn-primary" type="button" disabled={busy} data-testid="volume-move-confirm" onClick={() => void confirm()}>
            {busy ? "移动中…" : `移动到「${targetTitle}」`}
          </button>
        </div>
      </div>
    </div>
  );
}

/**
 * 「未分卷」永远在列表里（契约：volumeId=0 就是移到未分卷）。
 * 详情里有这个伪卷就沿用它的章节，没有就补一个空壳，避免列表里少一项。
 */
function looseTarget(volumes: Volume[]): Volume {
  return (
    volumes.find((volume) => volume.id === LOOSE_VOLUME_ID) ?? {
      id: LOOSE_VOLUME_ID,
      idx: -1,
      title: LOOSE_VOLUME_TITLE,
      chapters: [],
    }
  );
}
