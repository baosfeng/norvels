import { useEffect, useRef } from "react";

/**
 * 进度恢复提示条（C-16）：进入章节时若存在上次读到的段落，提示条浮在正文顶部。
 * 「知道了」点击后 150ms 淡出；滚动超过一屏也自动消失。焦点交还正文，不抢走。
 */
export function ReaderRestoreHint({
  visible,
  onDismiss,
}: {
  visible: boolean;
  onDismiss: () => void;
}) {
  const base = useRef<number | null>(null);

  useEffect(() => {
    if (!visible) {
      base.current = null;
      return;
    }
    base.current = window.scrollY;
    const onScroll = () => {
      if (base.current === null) return;
      if (Math.abs(window.scrollY - base.current) > window.innerHeight) onDismiss();
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
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
