/**
 * 自动滚动的悬浮控制条：只在开启自动滚动时出现。
 * 面板 / 抽屉打开、窗口失焦、页面隐藏时滚动会自动暂停，这里显示「继续」而不是「暂停」。
 *
 * 分页模式下同一份设置驱动的不是滚动而是翻页：这里只换称谓（「自动翻页中」），
 * 状态机、按钮与像素速度档全都共用——读者在两处看到的是同一个开关。
 */
export function ReaderAutoScrollBar({
  running,
  paused,
  speed,
  paged = false,
  onTogglePause,
  onExit,
}: {
  running: boolean;
  paused: boolean;
  speed: number;
  /** 分页模式：文案叫「自动翻页」。 */
  paged?: boolean;
  onTogglePause: () => void;
  onExit: () => void;
}) {
  const speedText = speed % 1 === 0 ? speed.toFixed(0) : speed.toFixed(1);
  const noun = paged ? "自动翻页" : "自动滚动";
  const state = running ? `${noun}中` : paused ? "已暂停" : "待开始";
  return (
    <div className="reader-autoscroll" role="group" aria-label={noun} data-testid="reader-autoscroll" aria-live="off">
      <span className="reader-autoscroll-state" data-testid="reader-autoscroll-state">
        {state}
      </span>
      <span className="meta num">
        {speedText} 档{paused ? " · 已暂停" : ""}
      </span>
      <button className="btn" type="button" onClick={onTogglePause} data-testid="reader-autoscroll-toggle">
        {running ? "暂停" : "继续"}
      </button>
      <button className="btn btn-ghost" type="button" onClick={onExit} aria-label={`退出${noun}`}>
        退出
      </button>
    </div>
  );
}
