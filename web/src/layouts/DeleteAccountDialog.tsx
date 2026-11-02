import { useRef } from "react";
import { useFocusTrap } from "../lib/focusTrap";

/**
 * 删除账号的二次确认框。
 *
 * 三条必须说清楚的事（删错了没有撤销条，只能靠事先说清）：
 * 1. 删的是哪个账号（名字写进标题，多账号时不会点错行）；
 * 2. **个人记录会一起删掉**（书架 / 进度 / 书签 / 书单），且不可恢复；
 * 3. 书本身不会删，别人和书库不受影响——否则用户会以为「删号 = 删书」而不敢点。
 *
 * 默认焦点在「取消」（与书签删除确认一致）：Esc、点遮罩、点取消都是取消。
 */
export function DeleteAccountDialog({
  open,
  name,
  deleting,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  name: string;
  deleting: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  useFocusTrap(open, rootRef, onCancel);
  if (!open) return null;
  return (
    <div className="confirm-backdrop" onClick={onCancel} data-testid="delete-account-confirm">
      <div
        ref={rootRef}
        className="confirm"
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-account-title"
        onClick={(event) => event.stopPropagation()}
      >
        <strong id="delete-account-title">删除账号「{name}」？</strong>
        <p>这个账号的书架、阅读进度、书签和书单会一起删掉，删了没法恢复。库里的书不受影响。</p>
        <div className="confirm-row">
          <button className="btn" type="button" data-testid="delete-account-cancel" onClick={onCancel}>
            取消
          </button>
          <button
            className="btn btn-danger"
            type="button"
            disabled={deleting}
            data-testid="delete-account-confirm-button"
            onClick={onConfirm}
          >
            {deleting ? "正在删除…" : "确认删除账号"}
          </button>
        </div>
      </div>
    </div>
  );
}
