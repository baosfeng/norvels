import { useEffect, useRef, useState } from "react";
import { useReaderScrollPercent } from "./useReaderScrollPercent";

/** 分页模式下的浮标口径：页码文案与页内比例由分页游标提供。 */
export interface PagedProgress {
  label: string;
  percent: number;
}

/** 翻页后浮标停留的时长（ms）：与滚动时的手感一致（1.2 秒后淡出）。 */
const PAGED_CHIP_MS = 1200;

/**
 * 阅读态常驻进度：屏幕最底一条 3px 细条表示「本章读到哪」，翻页 / 滚动时右下角浮出浮标。
 *
 * 分页模式下同一处表达「第 p / P 页」：读者翻页时关心的是页码不是百分比，
 * 细条也按页序走（末页到底）。两种情况共用同一套样式与淡出规则——
 * 分页没有 scroll 事件可听，所以由页码变化触发同一条 1.2 秒计时。
 * 只表达本章、只读：不吃点击（点击仍归三区手势），菜单唤出时整组由 .is-controls 淡出。
 */
export function ReaderScrollProgress({
  resetKey,
  chipSuppressed = false,
  paged = null,
}: {
  resetKey: number;
  chipSuppressed?: boolean;
  /** 分页模式：给了就用它当文案与比例，不再读滚动。 */
  paged?: PagedProgress | null;
}) {
  // 分页模式下才关掉滚动读数（paged 给了＝分页）
  const { percent, scrolling } = useReaderScrollPercent(resetKey, paged === null);
  const pagedVisible = usePagedChipVisible(paged);
  const ratio = paged ? paged.percent : percent;
  const shown = Math.round(ratio * 100);
  const chipVisible = paged ? pagedVisible : scrolling;
  const label = paged ? paged.label : `本章 ${shown}%`;
  return (
    <>
      <div
        className="reader-scroll-track"
        data-testid="reader-progress-track"
        role="progressbar"
        aria-label={paged ? "本章阅读页数" : "本章阅读进度"}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={shown}
      >
        <div
          className="reader-scroll-fill"
          data-testid="reader-progress-bar"
          style={{ width: `${(ratio * 100).toFixed(2)}%` }}
        />
      </div>
      <p
        className={`reader-scroll-chip${chipVisible && !chipSuppressed ? " is-shown" : ""}`}
        data-testid="reader-progress-chip"
        aria-hidden="true"
      >
        {label}
      </p>
    </>
  );
}

/**
 * 分页模式的浮标显隐：页码一变就亮 1.2 秒。
 * 分页不产生 scroll 事件，只能拿「页码变了」当「读者动了一下」——
 * 停在某一页读几分钟时它自然淡出，不常驻挡正文。
 */
function usePagedChipVisible(paged: PagedProgress | null): boolean {
  const [visible, setVisible] = useState(false);
  const last = useRef<string>("");
  const label = paged?.label ?? "";
  useEffect(() => {
    if (!paged) {
      last.current = "";
      return;
    }
    if (last.current === label) return;
    last.current = label;
    setVisible(true);
    const timer = window.setTimeout(() => setVisible(false), PAGED_CHIP_MS);
    return () => window.clearTimeout(timer);
  }, [paged, label]);
  return visible;
}
