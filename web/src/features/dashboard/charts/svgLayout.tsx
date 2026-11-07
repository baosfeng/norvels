import type { ReactNode } from "react";
import {
  bandScale,
  domainTopOf,
  estimateTextWidth,
  labelStride,
  labelVisible,
  linearScale,
  tickStep,
  ticksWithin,
  type LinearScale,
} from "../../../lib/chartScale";

/**
 * 一套 SVG 图的公共坐标：绘图区、y 刻度、刻度文本宽度决定左边距。
 *
 * 所有图共用它，好处是「刻度条数 / 0 基线 / 左边距算法 / 标签抽稀」只在一处实现：
 * 改一次，柱状图与折线图的刻度就永远对得上。
 * 分面图（事件趋势上下两面）用 plotTop / plotBottom / plotLeft 覆盖同一套算法，
 * 于是上下两面的 0 基线、刻度文本与 x 轴标签严格共用一条竖线。
 */

/** 顶部说明行（标题 / 图例）的高度；不画说明行时不留这条空。 */
export const CAPTION_H = 13;

/** x 轴标签行高度（标签基线在 plotBottom + 10）。 */
export const XLABEL_H = 14;

export interface LegendItem {
  key: string;
  label: string;
  color: string;
}

export interface ChartLayout {
  readonly width: number;
  readonly height: number;
  /** 绘图区左边界（让出 y 刻度文本） */
  readonly plotLeft: number;
  readonly plotRight: number;
  readonly plotTop: number;
  /** 0 基线所在的 y */
  readonly plotBottom: number;
  readonly plotW: number;
  readonly plotH: number;
  /** y 刻度值（升序，含 0） */
  readonly ticks: number[];
  /** 域的上界（= 最大值 × (1 + pad)，不一定是刻度值） */
  readonly domainTop: number;
  /** 数值 → y 像素 */
  readonly yOf: (value: number) => number;
  /** 完整刻度对象（算柱高、零点要用） */
  readonly scale: LinearScale;
  readonly axisFont: number;
}

export interface LayoutOptions {
  width: number;
  height: number;
  /** 参与定刻度的所有数值 */
  values: number[];
  formatValue: (value: number) => string;
  /** 顶部说明行（标题 / 图例） */
  captionH?: number;
  /** x 轴标签行：没有标签就不留底部空间 */
  hasXLabels?: boolean;
  /** 期望的刻度格数（0 以上）；省略时按绘图区高度给 */
  tickCount?: number;
  /** 顶部留白比例：最高值占绘图区 (1/(1+pad)) */
  padRatio?: number;
  /** 显式覆盖绘图区（分面图把同一套算法套到某一面上） */
  plotLeft?: number;
  plotTop?: number;
  plotBottom?: number;
  plotRight?: number;
}

export function chartLayout(options: LayoutOptions): ChartLayout {
  const { width, height, values, formatValue } = options;
  const captionH = options.captionH ?? 0;
  const defaultTop = 4 + captionH;
  const defaultBottom = options.hasXLabels === false ? 4 : XLABEL_H;
  const plotTop = options.plotTop ?? defaultTop;
  const plotBottom = options.plotBottom ?? Math.max(plotTop + 8, height - defaultBottom);
  const axisFont = plotBottom - plotTop >= 100 ? 9 : 8;
  const max = values.reduce((acc, value) => (Number.isFinite(value) && value > acc ? value : acc), 0);
  const domainTop = domainTopOf(max, options.padRatio ?? 0.08);
  const plotH = plotBottom - plotTop;
  // 刻度条数按绘图区高度定：101px 的图给 5 条（838 本是 0/200/400/600/800），
  // 只给 3 条时 1/2/5 阶梯会跳到 500，图上就只剩「0 和 500」两条线，读不出高度。
  const tickCount = options.tickCount ?? (plotH >= 90 ? 5 : plotH >= 50 ? 3 : 2);
  const ticks = ticksWithin(tickStep(domainTop, tickCount), domainTop);
  const scale = linearScale(0, domainTop, plotBottom, plotTop);
  const tickWidth = ticks.reduce(
    (acc, tick) => Math.max(acc, estimateTextWidth(formatValue(tick), axisFont)),
    0,
  );
  const measuredLeft = Math.min(Math.round(tickWidth + 8), Math.max(24, width * 0.42));
  const plotLeft = options.plotLeft ?? measuredLeft;
  const plotRight = options.plotRight ?? Math.max(plotLeft + 24, width - 2);
  return {
    width,
    height,
    plotLeft,
    plotRight,
    plotTop,
    plotBottom,
    plotW: plotRight - plotLeft,
    plotH,
    ticks,
    domainTop,
    yOf: scale.scale,
    scale,
    axisFont,
  };
}

/** y 轴：刻度网格线 + 0 基线 + 刻度文本。 */
export function YAxis({ layout, formatValue }: { layout: ChartLayout; formatValue: (value: number) => string }) {
  return (
    <>
      {layout.ticks.map((tick) =>
        tick > 0 ? (
          <line
            key={`grid-${tick}`}
            className="c-grid"
            x1={layout.plotLeft}
            x2={layout.plotRight}
            y1={layout.yOf(tick)}
            y2={layout.yOf(tick)}
          />
        ) : null,
      )}
      <line
        className="c-axis"
        x1={layout.plotLeft}
        x2={layout.plotRight}
        y1={layout.plotBottom}
        y2={layout.plotBottom}
      />
      {layout.ticks.map((tick) => (
        <text
          key={`tick-${tick}`}
          className="c-tick"
          x={layout.plotLeft - 4}
          y={layout.yOf(tick) + 3}
          fontSize={layout.axisFont}
          textAnchor="end"
        >
          {formatValue(tick)}
        </text>
      ))}
    </>
  );
}

/** x 轴标签：按最小间距抽稀，首尾必标。 */
export function XLabels({
  layout,
  labels,
  y,
}: {
  layout: ChartLayout;
  labels: string[];
  y?: number;
}) {
  const count = labels.length;
  const band = bandScale(count, layout.plotW, 0);
  const stride = labelStride(count, band.step);
  const visible = labelVisible(count, stride);
  const baseline = (y ?? layout.plotBottom) + 10;
  return (
    <>
      {labels.map((label, index) =>
        visible[index] ? (
          <text
            key={`x-${index}-${label}`}
            className="c-xlabel"
            x={layout.plotLeft + band.center(index)}
            y={baseline}
            textAnchor="middle"
          >
            {label}
          </text>
        ) : null,
      )}
    </>
  );
}

/**
 * 顶部说明行：左侧一句标题（如「曝光 / 点击 · 次」），右侧可选色块图例。
 *
 * 一行放不下时按顺序截断后面的图例项（宁少一项，不叠字）；说明与图例都是同一份数据的注脚。
 */
export function CaptionRow({
  caption,
  legend,
  x = 2,
}: {
  caption?: string;
  legend?: LegendItem[];
  x?: number;
}) {
  if (!caption && (!legend || legend.length === 0)) return null;
  let cursor = x;
  const nodes: ReactNode[] = [];
  if (caption) {
    nodes.push(
      <text key="caption" className="c-caption" x={cursor} y={9}>
        {caption}
      </text>,
    );
    cursor += estimateTextWidth(caption, 9) + 12;
  }
  for (const item of legend ?? []) {
    const width = 10 + estimateTextWidth(item.label, 9) + 10;
    nodes.push(
      <rect key={`dot-${item.key}`} x={cursor} y={2.5} width={7} height={7} rx={1.5} fill={item.color} />,
    );
    nodes.push(
      <text key={`label-${item.key}`} className="c-caption" x={cursor + 10} y={9}>
        {item.label}
      </text>,
    );
    cursor += width;
  }
  return <>{nodes}</>;
}
