import { useReaderScrollPercent } from "./useReaderScrollPercent";

/**
 * 阅读态常驻进度：正文下沿一条 3px 细条表示「本章读到哪」，滚动时右下角浮出「本章 N%」。
 *
 * 只表达本章、只读：不吃点击（点击仍归三区手势），菜单唤出时整组由 .is-controls 淡出，
 * 避免与底栏的章节滑条重复；章末那行的「全书进度」是全书口径，两者语义不重叠。
 */
export function ReaderScrollProgress({ resetKey }: { resetKey: number }) {
  const { percent, scrolling } = useReaderScrollPercent(resetKey);
  const shown = Math.round(percent * 100);
  return (
    <>
      <div
        className="reader-scroll-track"
        data-testid="reader-progress-track"
        role="progressbar"
        aria-label="本章阅读进度"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={shown}
      >
        <div
          className="reader-scroll-fill"
          data-testid="reader-progress-bar"
          style={{ width: `${(percent * 100).toFixed(2)}%` }}
        />
      </div>
      <p className={`reader-scroll-chip${scrolling ? " is-shown" : ""}`} data-testid="reader-progress-chip" aria-hidden="true">
        本章 {shown}%
      </p>
    </>
  );
}
