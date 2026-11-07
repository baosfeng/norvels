import { bandScale } from "../../../lib/chartScale";
import type { ChartTipBindings } from "../useChartTip";
import { CaptionRow, chartLayout, type LegendItem, XLabels, YAxis } from "./svgLayout";
import { useChartWidth } from "./useChartWidth";

/**
 * 折线图（纯 SVG）：近 14 天阅读时长 / 导入累计曲线。
 *
 * 三条口径：
 *   - 0 基线必画：那天没读就是贴着基线的一个点，不是「没数据」；
 *   - 可带面积：面积只做阅读时长的底衬（浅到不影响线本身的可读性）；
 *   - 点 + 整列读数：14 个点每一列都是读数目标，浮层给当天真实值。
 */

export interface LineSeries {
  key: string;
  label: string;
  color: string;
  values: number[];
}

export interface LineChartProps {
  labels: string[];
  series: LineSeries[];
  titles: string[];
  height?: number;
  caption?: string;
  legend?: LegendItem[];
  /** 线下的浅色面积 */
  area?: boolean;
  /** 画数据点（点数 ≤ 20 时默认画） */
  showDots?: boolean;
  formatValue?: (value: number) => string;
  tickCount?: number;
  tipBind: (label: string) => ChartTipBindings;
  testId?: string;
  ariaLabel: string;
}

const DEFAULT_FORMAT = (value: number) => String(value);
const DOT_R = 2.4;

export function LineChart({
  labels,
  series,
  titles,
  height = 132,
  caption,
  legend,
  area = false,
  showDots,
  formatValue = DEFAULT_FORMAT,
  tickCount,
  tipBind,
  testId,
  ariaLabel,
}: LineChartProps) {
  const [ref, width] = useChartWidth();
  const values = series.flatMap((item) => item.values);
  const layout = chartLayout({
    width,
    height,
    values,
    formatValue,
    captionH: caption || legend?.length ? 13 : 0,
    tickCount,
  });
  const band = bandScale(labels.length, layout.plotW, 0);
  const dots = showDots ?? labels.length <= 20;
  const xOf = (index: number) => layout.plotLeft + band.center(index);
  const yOf = (value: number) => layout.yOf(Number.isFinite(value) && value > 0 ? value : 0);

  return (
    <div className="chart-fit" ref={ref} data-testid={testId}>
      <svg className="c-svg" width={width} height={height} role="img" aria-label={ariaLabel}>
        <CaptionRow caption={caption} legend={legend} />
        <YAxis layout={layout} formatValue={formatValue} />
        {series.map((item) => {
          const points = labels.map((_, index) => `${xOf(index)},${yOf(item.values[index] ?? 0)}`);
          const areaPath = `M ${xOf(0)} ${layout.plotBottom} L ${points.join(" L ")} L ${xOf(labels.length - 1)} ${layout.plotBottom} Z`;
          return (
            <g key={item.key}>
              {area && labels.length > 1 ? (
                <path className="c-area" d={areaPath} fill={item.color} opacity={0.14} />
              ) : null}
              <polyline
                className="c-line"
                points={points.join(" ")}
                fill="none"
                stroke={item.color}
                strokeWidth={1.8}
                strokeLinecap="round"
                strokeLinejoin="round"
              />
              {dots
                ? labels.map((_, index) => (
                    <circle
                      key={`dot-${item.key}-${index}`}
                      className="c-dot"
                      cx={xOf(index)}
                      cy={yOf(item.values[index] ?? 0)}
                      r={DOT_R}
                      fill={item.color}
                    />
                  ))
                : null}
            </g>
          );
        })}
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
