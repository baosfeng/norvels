import { bandScale, barHeight, barWidth, ZERO_BAR_PX } from "../../../lib/chartScale";
import type { ChartTipBindings } from "../useChartTip";
import { CaptionRow, chartLayout, type LegendItem, XLabels, YAxis } from "./svgLayout";
import { useChartWidth } from "./useChartWidth";

/**
 * 竖条图（纯 SVG）：时间轴柱、桶分布共用。
 *
 * 三条口径，对应实测缺陷（14 列每列 23px、6 个 0 值日完全不画、无 y 刻度）：
 *   - 柱宽 = 列宽 62%，收在 4–28px：14 天每根约 11px，6 个桶约 26px；
 *   - 0 值画 1px 基线：读者能分清「这天是 0」与「这天缺数据」；
 *   - y 轴 1/2/5 刻度 + 0 基线：高度可读，不必靠 hover 才知道多少。
 * 整列（不止柱子）都是读数目标：0 值那天没有柱子，也要能读出「这天没有」。
 */

export interface ColumnSeries {
  key: string;
  label: string;
  color: string;
  values: number[];
}

export interface ColumnChartProps {
  /** x 轴标签（已格式化，如 10/18） */
  labels: string[];
  series: ColumnSeries[];
  /** 每个列的读数文案（浮层与 aria-label 共用） */
  titles: string[];
  /** 图体高度（px），默认 132（驾驶舱基准） */
  height?: number;
  /** 顶部说明行：一句单位说明 */
  caption?: string;
  legend?: LegendItem[];
  /** 在柱顶标数值（桶分布这类稀疏图用；14 天的时间轴太密就不标） */
  showValues?: boolean;
  /** 高亮第几列（峰值） */
  peakIndex?: number;
  peakColor?: string;
  formatValue?: (value: number) => string;
  tickCount?: number;
  /** 分面图里共用的左边距：上下两面的柱子必须落在同一列上 */
  plotLeft?: number;
  tipBind: (label: string) => ChartTipBindings;
  testId?: string;
  ariaLabel: string;
}

const DEFAULT_FORMAT = (value: number) => String(value);

export function ColumnChart({
  labels,
  series,
  titles,
  height = 132,
  caption,
  legend,
  showValues = false,
  peakIndex,
  peakColor = "var(--accent)",
  formatValue = DEFAULT_FORMAT,
  tickCount,
  plotLeft,
  tipBind,
  testId,
  ariaLabel,
}: ColumnChartProps) {
  const [ref, width] = useChartWidth();
  const values = series.flatMap((item) => item.values);
  const layout = chartLayout({
    width,
    height,
    values,
    formatValue,
    captionH: caption || legend?.length ? 13 : 0,
    tickCount,
    plotLeft,
  });
  const band = bandScale(labels.length, layout.plotW, 0.38);
  const barW = barWidth(band.bandwidth / Math.max(1, series.length), 1, series.length > 1 ? 3 : 4, 28);
  const groupW = barW * series.length + Math.max(0, series.length - 1);
  const groupStart = (index: number) =>
    layout.plotLeft + band.start(index) + Math.max(0, (band.bandwidth - groupW) / 2);

  return (
    <div className="chart-fit" ref={ref} data-testid={testId}>
      <svg className="c-svg" width={width} height={height} role="img" aria-label={ariaLabel}>
        <CaptionRow caption={caption} legend={legend} />
        <YAxis layout={layout} formatValue={formatValue} />
        {labels.map((_, index) =>
          series.map((item, sIndex) => {
            const value = item.values[index] ?? 0;
            const barH = barHeight(value, layout.scale);
            const isPeak = peakIndex === index && series.length === 1;
            return (
              <rect
                key={`bar-${item.key}-${index}`}
                className={`col-bar${value > 0 ? "" : " is-zero"}${isPeak ? " is-peak" : ""}`}
                data-zero-baseline={value > 0 ? undefined : "1"}
                x={groupStart(index) + sIndex * (barW + 1)}
                y={layout.plotBottom - barH}
                width={barW}
                height={Math.max(ZERO_BAR_PX, barH)}
                rx={2}
                fill={isPeak ? peakColor : item.color}
              />
            );
          }),
        )}
        {showValues
          ? labels.map((_, index) => {
              const value = series[0]?.values[index] ?? 0;
              if (!(value > 0)) return null;
              return (
                <text
                  key={`value-${index}`}
                  className="c-value"
                  x={layout.plotLeft + band.center(index)}
                  y={Math.max(layout.plotTop + 10, layout.yOf(value) - 3)}
                  textAnchor="middle"
                >
                  {formatValue(value)}
                </text>
              );
            })
          : null}
        {labels.map((label, index) => (
          <rect
            key={`hit-${index}`}
            className="c-hit"
            x={layout.plotLeft + index * band.step}
            y={layout.plotTop}
            width={Math.max(1, band.step)}
            height={layout.plotH}
            {...tipBind(titles[index] ?? label)}
          />
        ))}
        <XLabels layout={layout} labels={labels} />
      </svg>
    </div>
  );
}
