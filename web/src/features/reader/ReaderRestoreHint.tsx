import { useEffect, useRef } from "react";

/** 提示条自动消失的时间：够读者看到，又不长期挡在正文上。 */
export const RESTORE_HINT_MS = 3000;

/** 收起的原因：超时自动收（不抢焦点）/ 读者点「知道了」（焦点交还正文）。 */
export type RestoreHintDismiss = "timeout" | "user";

/**
 * 进度恢复提示：进入章节时若存在上次读到的段落，浮一个只包住文字的胶囊——
 * H5 落在正文底部留白带里（不压正文、不横贯屏幕），PC 仍贴底浮在正文之上。
 *
 * 3 秒自动消失——恢复本身是「已经发生在你身上」的事，不需要读者先关掉浮条才能读；
 * 「知道了」保留给想立刻收起的读者。
 *
 * 计时只认 visible，回调走 ref：阅读器每帧都在重渲染（滚动进度 / 可见段 / 进度保存），
 * 把不稳定的 onDismiss 放进依赖会反复重建定时器——实测提示于是永远等不到 3 秒，
 * 只能点「知道了」才消失。
 */
export function ReaderRestoreHint({
  visible,
  onDismiss,
}: {
  visible: boolean;
  onDismiss: (reason: RestoreHintDismiss) => void;
}) {
  const dismiss = useRef(onDismiss);
  useEffect(() => {
    dismiss.current = onDismiss;
  }, [onDismiss]);

  useEffect(() => {
    if (!visible) return;
    const timer = window.setTimeout(() => dismiss.current("timeout"), RESTORE_HINT_MS);
    return () => window.clearTimeout(timer);
  }, [visible]);

  if (!visible) return null;
  return (
    <div className="restore-hint" role="status" data-testid="reader-restore-hint">
      <span>已回到上次读到的地方</span>
      <button
        className="btn btn-ghost"
        type="button"
        data-testid="reader-restore-dismiss"
        onClick={() => dismiss.current("user")}
      >
        知道了
      </button>
    </div>
  );
}
