/** 未选章时的常驻原因（契约 C-60 / C-61 边界列原话）。 */
export const NO_PICK_HINT = "先勾选章节，才能移动或删除";

/**
 * 章节批量条：三个动作都要求先勾选，所以未勾选时按钮禁用、原因常驻在左边——
 * 用户不用点一次才知道为什么不能点。合并另外要求「同一卷且连续」，
 * 不可合并的原因由 mergeVerdict 给出，与本条并排显示。
 */
export function ChapterBatchBar({
  count,
  mergeReason,
  onMerge,
  onMove,
  onDelete,
  onClear,
}: {
  count: number;
  /** 空串 = 可以合并；非空 = 禁用原因 */
  mergeReason: string;
  onMerge: () => void;
  onMove: () => void;
  onDelete: () => void;
  onClear: () => void;
}) {
  const empty = count === 0;
  return (
    <div className="ch-batch" data-testid="admin-ch-batch" aria-live="polite">
      <span className="meta">
        已选 <strong className="num">{count}</strong> 章
      </span>
      {empty ? (
        <span className="inline-note" data-testid="admin-ch-batch-hint">
          {NO_PICK_HINT}
        </span>
      ) : mergeReason ? (
        <span className="reason" role="status" data-testid="admin-ch-merge-reason">
          {mergeReason}
        </span>
      ) : null}
      <div className="actions">
        <button className="btn btn-sm" type="button" data-testid="admin-ch-clear" disabled={empty} onClick={onClear}>
          取消选择
        </button>
        <button
          className="btn btn-sm"
          type="button"
          data-testid="admin-ch-merge"
          disabled={empty || mergeReason !== ""}
          data-od-id="C-59"
          onClick={onMerge}
        >
          合并
        </button>
        <button className="btn btn-sm" type="button" data-testid="admin-ch-move" disabled={empty} data-od-id="C-60" onClick={onMove}>
          移动到卷…
        </button>
        <button
          className="btn btn-sm btn-danger"
          type="button"
          data-testid="admin-ch-batch-delete"
          disabled={empty}
          data-od-id="C-61"
          onClick={onDelete}
        >
          删除所选
        </button>
      </div>
    </div>
  );
}
