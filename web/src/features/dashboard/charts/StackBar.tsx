import { useId } from "react";
import { stackSlices } from "../../../lib/chartScale";
import type { ChartTipBindings } from "../useChartTip";
import { CaptionRow, type LegendItem } from "./svgLayout";
import { useChartWidth } from "./useChartWidth";

/**
 * 堆叠条（纯 SVG）：库体积构成。
 *
 * 两条口径：
 *   - 段宽 = 真实占比，不做最小宽度归一（0.1% 的「其他」就画 0.1%，图例写「<1%」解释）；
 *   - 铺满图体：条带占满 132px 的图体（原先 26px 条沉底、上方空 75px），
 *     段内直接写名字与占比，读占比不必再跳回图例。
 */

export interface StackPart {
  key: string;
  label: string;
  value: number;
  /** 段内/图例里的体积文案（已格式化，如 1.7 GB） */
  display: string;
  /** 段内/图例里的占比文案（<1% 也在这里定） */
  pctText: string;
  color: string;
  /** 段内文字颜色（深色段配浅字） */
  labelColor?: string;
  title: string;
}

export interface StackBarProps {
  parts: StackPart[];
  height?: number;
  caption?: string;
  legend?: LegendItem[];
  tipBind: (label: string) => ChartTipBindings;
  testId?: string;
  ariaLabel: string;
}

const RADIUS = 6;
const NAME_FONT = 11;
const PCT_FONT = 15;
const SIZE_FONT = 10;

export function StackBar({ parts, height = 132, caption, legend, tipBind, testId, ariaLabel }: StackBarProps) {
  const [ref, width] = useChartWidth();
  const clipId = useId();
  const barTop = caption || legend?.length ? 14 : 0;
  const barH = Math.max(24, height - barTop);
  const slices = stackSlices(
    parts.map((part) => part.value),
    width,
  );

  return (
    <div className="chart-fit" ref={ref} data-testid={testId}>
      <svg className="c-svg" width={width} height={height} role="img" aria-label={ariaLabel}>
        <CaptionRow caption={caption} legend={legend} />
        <defs>
          <clipPath id={clipId}>
            <rect x={0} y={barTop} width={width} height={barH} rx={RADIUS} />
          </clipPath>
        </defs>
        <g clipPath={`url(#${clipId})`}>
          {parts.map((part, index) => {
            const slice = slices[index];
            const sliceW = Math.max(0, slice.end - slice.start);
            if (sliceW <= 0) return null;
            const textColor = part.labelColor ?? "var(--fg)";
            const showName = sliceW >= 76;
            const showSize = sliceW >= 126;
            const centerX = slice.start + sliceW / 2;
            const midY = barTop + barH / 2;
            return (
              <g key={part.key}>
                <rect
                  className="stack-seg"
                  x={slice.start}
                  y={barTop}
                  width={sliceW}
                  height={barH}
                  fill={part.color}
                  {...tipBind(part.title)}
                />
                {showName ? (
                  <>
                    <text className="c-slice-name" x={centerX} y={midY - 6} fontSize={NAME_FONT} fill={textColor} textAnchor="middle">
                      {part.label}
                    </text>
                    <text className="c-slice-pct" x={centerX} y={midY + 14} fontSize={PCT_FONT} fill={textColor} textAnchor="middle">
                      {part.pctText}
                    </text>
                    {showSize ? (
                      <text className="c-slice-size" x={centerX} y={midY + 28} fontSize={SIZE_FONT} fill={textColor} textAnchor="middle">
                        {part.display}
                      </text>
                    ) : null}
                  </>
                ) : null}
              </g>
            );
          })}
        </g>
      </svg>
    </div>
  );
}
