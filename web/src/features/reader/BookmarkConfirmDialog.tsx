import { useRef } from "react";
import type { Bookmark } from "../../api/types";
import { bookmarkConfirmBody } from "../../lib/bookmark";
import { useFocusTrap } from "../../lib/focusTrap";

/**
 * 删除书签二次确认框（C-58 / C-59 / C-60）：默认焦点在「取消」，Esc 与点遮罩等价于取消。
 * 删除按钮在请求期间禁用并显示「正在删除…」，不给撤销条（删除不可恢复，已二次确认）。
 */
export function BookmarkConfirmDialog({
  open,
  bookmark,
  deleting,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  bookmark: Bookmark | null;
  deleting: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  useFocusTrap(open && Boolean(bookmark), rootRef, onCancel);
  if (!open || !bookmark) return null;
  return (
    <div className="confirm-backdrop" onClick={onCancel} data-testid="bookmark-confirm">
      <div
        ref={rootRef}
        className="confirm"
        role="dialog"
        aria-modal="true"
        aria-labelledby="bookmark-confirm-title"
        data-od-id="C-58"
        onClick={(event) => event.stopPropagation()}
      >
        <strong id="bookmark-confirm-title">删除这条书签？</strong>
        <p>{bookmarkConfirmBody(bookmark)}</p>
        <div className="confirm-row">
          <button className="btn" type="button" data-od-id="C-59" onClick={onCancel}>
            取消
          </button>
          <button className="btn btn-danger" type="button" disabled={deleting} data-od-id="C-60" onClick={onConfirm}>
            {deleting ? "正在删除…" : "删除书签"}
          </button>
        </div>
      </div>
    </div>
  );
}
