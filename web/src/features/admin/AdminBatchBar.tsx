import { MAX_BATCH } from "./batchLimit";

/**
 * 书籍批量条：有勾选才出现，四个动作都要求「已选至少 1 本」。
 * 数量只有一个来源（调用方传进来的已选集合大小），所以条上与各弹层标题永远是同一个数。
 */
export function AdminBatchBar({
  count,
  onAuthor,
  onTag,
  onSerial,
  onDelete,
  onClear,
}: {
  count: number;
  onAuthor: () => void;
  onTag: () => void;
  onSerial: () => void;
  onDelete: () => void;
  onClear: () => void;
}) {
  if (count === 0) return null;
  const over = count > MAX_BATCH;
  return (
    <div className="batch-bar" data-testid="admin-batch-bar">
      <span className="meta">
        已选 <strong className="num">{count}</strong> 本
      </span>
      {over ? <span className="inline-note">一次最多 {MAX_BATCH} 本，分批再改</span> : null}
      <div className="actions">
        <button className="btn btn-sm" type="button" data-testid="admin-batch-author" data-od-id="C-38" onClick={onAuthor}>
          批量改作者
        </button>
        <button className="btn btn-sm" type="button" data-testid="admin-batch-tag" data-od-id="C-39" onClick={onTag}>
          批量改标签
        </button>
        <button className="btn btn-sm" type="button" data-testid="admin-batch-serial" data-od-id="C-40" onClick={onSerial}>
          批量改连载状态
        </button>
        <button className="btn btn-sm btn-danger" type="button" data-testid="admin-batch-delete" data-od-id="C-41" onClick={onDelete}>
          批量删除
        </button>
        <button className="btn btn-sm btn-ghost" type="button" data-testid="admin-batch-clear" data-od-id="C-42" onClick={onClear}>
          取消选择
        </button>
      </div>
    </div>
  );
}
