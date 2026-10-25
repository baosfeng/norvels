/**
 * 自动滚动的悬浮控制条：只在开启自动滚动时出现。
 * 面板 / 抽屉打开、窗口失焦、页面隐藏时滚动会自动暂停，这里显示「继续」而不是「暂停」。
 */
export function ReaderAutoScrollBar({
  running,
  paused,
  speed,
  onTogglePause,
  onExit,
}: {
  running: boolean;
  paused: boolean;
  speed: number;
  onTogglePause: () => void;
  onExit: () => void;
}) {
  return (
    <div className="reader-autoscroll" role="group" aria-label="自动滚动" data-testid="reader-autoscroll" aria-live="off">
      <span className="reader-autoscroll-state" data-testid="reader-autoscroll-state">
        {running ? "自动滚动中" : paused ? "已暂停" : "待开始"}
      </span>
      <span className="meta num">{speed % 1 === 0 ? speed.toFixed(0) : speed.toFixed(1)} 档</span>
      <button className="btn" type="button" onClick={onTogglePause} data-testid="reader-autoscroll-toggle">
        {running ? "暂停" : "继续"}
      </button>
      <button className="btn btn-ghost" type="button" onClick={onExit} aria-label="退出自动滚动">
        退出
      </button>
    </div>
  );
}
