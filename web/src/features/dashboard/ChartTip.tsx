import type { ChartTipState } from "./useChartTip";

/** 浮层本体：role=status + aria-live，读数变化时读屏也会跟着念一遍。 */
export function ChartTipLayer({ tip, testId = "chart-tip" }: { tip: ChartTipState | null; testId?: string }) {
  if (!tip) return null;
  return (
    <div
      className={`chart-tip${tip.below ? " is-below" : ""}${tip.pinned ? " is-pinned" : ""}`}
      style={{ left: tip.x, top: tip.y }}
      role="status"
      aria-live="polite"
      data-testid={testId}
    >
      {tip.label}
    </div>
  );
}
