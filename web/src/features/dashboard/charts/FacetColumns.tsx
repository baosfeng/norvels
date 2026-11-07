import type { ReactNode } from "react";
import { bandScale, barHeight, barWidth, estimateTextWidth, ZERO_BAR_PX } from "../../../lib/chartScale";
import type { ChartTipBindings } from "../useChartTip";
import type { ColumnSeries } from "./ColumnChart";
import { chartLayout, type ChartLayout, XLabels, YAxis } from "./svgLayout";
import { useChartWidth } from "./useChartWidth";

/**
 * 分面竖条图（纯 SVG）：一列 x 轴、上下两个 mini 面，各自独立刻度与单位。
 *
 * 对应实测缺陷：曝光 / 点击 / 阅读三色柱并排时「三色各自按峰值归一」（116 / 30 / 130px），
 * 量纲不同的三组数被画成一样高，读者没法比较。分面之后每面共用同一条 x 轴与同一个左边距，
 * 一面只放同一量纲的系列，高度就可以跨天、跨面直接比。
 *
 * 每面的图例就写在该面标题行里（色块 + 名字 + 单位）：不再另占一行整卡图例，
 * 省下的 13px 直接给两个面的绘图区（上下各约 5px 高）。
 */

export interface FacetSpec {
  key: string;
  /** 单位（如「次」/「分钟」），跟在色块图例后面 */
  unit: string;
  series: ColumnSeries[];
  formatValue?: (value: number) => string;
}

export interface FacetColumnsProps {
  labels: string[];
  facets: FacetSpec[];
  /** 每列的读数文案（浮层共用一份） */
  titles: string[];
  height?: number;
  tipBind: (label: string) => ChartTipBindings;
  testId?: string;
  ariaLabel: string;
}

const LEGEND_FONT = 9;
const LEGEND_H = 12;
const LABELS_H = 14;
const FACE_GAP = 8;
const DEFAULT_FORMAT = (value: number) => String(value);

export function FacetColumns({
  labels,
  facets,
  titles,
  height = 132,
  tipBind,
  testId,
  ariaLabel,
}: FacetColumnsProps) {
  const [ref, width] = useChartWidth();
  const count = Math.max(1, facets.length);
  const faceH = (height - LABELS_H - FACE_GAP * (count - 1)) / count;
  const faceTop = (index: number) => index * (faceH + FACE_GAP);

  const base = facets.map((facet, index) =>
    chartLayout({
      width,
      height,
      values: facet.series.flatMap((item) => item.values),
      formatValue: facet.formatValue ?? DEFAULT_FORMAT,
      hasXLabels: false,
      captionH: 0,
      plotTop: faceTop(index) + LEGEND_H,
      plotBottom: faceTop(index) + faceH,
    }),
  );
  /** 上下两面共用左边距：刻度文本宽度不同时也要保证柱子落在同一列上。 */
  const plotLeft = base.reduce((acc, layout) => Math.max(acc, layout.plotLeft), 0);
  const layouts: ChartLayout[] = facets.map((facet, index) =>
    chartLayout({
      width,
      height,
      values: facet.series.flatMap((item) => item.values),
      formatValue: facet.formatValue ?? DEFAULT_FORMAT,
      hasXLabels: false,
      captionH: 0,
      plotTop: faceTop(index) + LEGEND_H,
      plotBottom: faceTop(index) + faceH,
      plotLeft,
    }),
  );
  const plotW = layouts[0]?.plotW ?? 0;
  const band = bandScale(labels.length, plotW, 0.4);

  return (
    <div className="chart-fit" ref={ref} data-testid={testId}>
      <svg className="c-svg" width={width} height={height} role="img" aria-label={ariaLabel}>
        {facets.map((facet, index) => {
          const layout = layouts[index];
          const formatValue = facet.formatValue ?? DEFAULT_FORMAT;
          const seriesCount = Math.max(1, facet.series.length);
          const barW = barWidth(band.bandwidth / seriesCount, 1, seriesCount > 1 ? 3 : 5, 22);
          const groupW = barW * seriesCount + Math.max(0, seriesCount - 1);
          /** 面标题行：色块 + 名字 + 单位，按顺序排一行。 */
          let cursor = 2;
          const legendNodes: { key: string; node: ReactNode }[] = [];
          for (const item of facet.series) {
            const labelX = cursor + 10;
            legendNodes.push({
              key: `dot-${item.key}`,
              node: <rect x={cursor} y={faceTop(index) + 1.5} width={7} height={7} rx={1.5} fill={item.color} />,
            });
            legendNodes.push({
              key: `name-${item.key}`,
              node: (
                <text className="c-caption" x={labelX} y={faceTop(index) + 9}>
                  {item.label}
                </text>
              ),
            });
            cursor = labelX + estimateTextWidth(item.label, LEGEND_FONT) + 8;
          }
          return (
            <g key={facet.key} data-facet={facet.key}>
              {legendNodes.map((entry) => (
                <g key={`${facet.key}-${entry.key}`}>{entry.node}</g>
              ))}
              <text className="c-caption is-unit" x={cursor} y={faceTop(index) + 9}>
                · {facet.unit}
              </text>
              <YAxis layout={layout} formatValue={formatValue} />
              {labels.map((_, dayIndex) =>
                facet.series.map((item, seriesIndex) => {
                  const value = item.values[dayIndex] ?? 0;
                  const barH = barHeight(value, layout.scale);
                  const x =
                    layout.plotLeft + band.start(dayIndex) + Math.max(0, (band.bandwidth - groupW) / 2) +
                    seriesIndex * (barW + 1);
                  return (
                    <rect
                      key={`${item.key}-${dayIndex}`}
                      className={`col-bar${value > 0 ? "" : " is-zero"}`}
                      data-zero-baseline={value > 0 ? undefined : "1"}
                      x={x}
                      y={layout.plotBottom - barH}
                      width={barW}
                      height={Math.max(ZERO_BAR_PX, barH)}
                      rx={1.5}
                      fill={item.color}
                    />
                  );
                }),
              )}
            </g>
          );
        })}
        {labels.map((label, index) => (
          <rect
            key={`hit-${index}`}
            className="c-hit"
            x={plotLeft + index * band.step}
            y={0}
            width={Math.max(1, band.step)}
            height={height - LABELS_H}
            {...tipBind(titles[index] ?? label)}
          />
        ))}
        <XLabels layout={layouts[layouts.length - 1]} labels={labels} />
      </svg>
    </div>
  );
}
