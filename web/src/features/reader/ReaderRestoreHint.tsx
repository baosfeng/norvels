import { useEffect } from "react";

/** 提示条自动消失的时间：够读者看到，又不长期挡在正文上。 */
export const RESTORE_HINT_MS = 3000;

/**
 * 进度恢复提示（C-16）：进入章节时若存在上次读到的段落，浮一个只包住文字的胶囊——
 * H5 落在正文底部留白带里（不压正文、不横贯屏幕），PC 仍贴底浮在正文之上。
 *
 * 3 秒自动消失——恢复本身是「已经发生在你身上」的事，不需要读者先关掉浮条才能读；
 * 「知道了」保留给想立刻收起的读者。焦点不抢：点「知道了」后交还正文。
 */
export function ReaderRestoreHint({
  visible,
  onDismiss,
}: {
  visible: boolean;
  onDismiss: () => void;
}) {
  useEffect(() => {
    if (!visible) return;
    const timer = window.setTimeout(onDismiss, RESTORE_HINT_MS);
    return () => window.clearTimeout(timer);
  }, [visible, onDismiss]);

  if (!visible) return null;
  return (
    <div className="restore-hint" role="status" data-testid="reader-restore-hint">
      <span>已回到上次读到的地方</span>
      <button className="btn btn-ghost" type="button" data-testid="reader-restore-dismiss" data-od-id="C-16" onClick={onDismiss}>
        知道了
      </button>
    </div>
  );
}
