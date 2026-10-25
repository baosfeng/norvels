/**
 * 章节批量条：有勾选才渲染。合并按钮的可用性由 mergeVerdict 决定，
 * 不可用时把原因写在按钮左边——用户不用点了才知道为什么不行。
 */
export function ChapterBatchBar({
  count,
  mergeReason,
  onMerge,
  onDelete,
  onClear,
}: {
  count: number;
  /** 空串 = 可以合并；非空 = 禁用原因 */
  mergeReason: string;
  onMerge: () => void;
  onDelete: () => void;
  onClear: () => void;
}) {
  if (count === 0) return null;
  return (
    <div className="ch-batch" data-testid="admin-ch-batch" aria-live="polite">
      <span className="meta">
        已选 <strong className="num">{count}</strong> 章
      </span>
      {mergeReason ? (
        <span className="reason" role="status" data-testid="admin-ch-merge-reason">
          {mergeReason}
        </span>
      ) : null}
      <div className="actions">
        <button className="btn btn-sm" type="button" data-testid="admin-ch-clear" onClick={onClear}>
          取消选择
        </button>
        <button
          className="btn btn-sm"
          type="button"
          data-testid="admin-ch-merge"
          disabled={mergeReason !== ""}
          onClick={onMerge}
        >
          合并
        </button>
        <button className="btn btn-sm btn-danger" type="button" data-testid="admin-ch-batch-delete" onClick={onDelete}>
          删除所选
        </button>
      </div>
    </div>
  );
}
