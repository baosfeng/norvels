import { useEffect, useRef, useState } from "react";
import type { ReadingStats } from "../../api/types";
import { formatDurationShort } from "../../lib/format";
import { buildHeatmap, type HeatmapDay } from "../../lib/seriesGroup";

/** 热力图 5 档底色：0 档用弱边框色，1–4 档由 --accent 与 --surface 混出，深浅主题自动跟随。 */
const LEVEL_BG = [
  "var(--border-soft)",
  "color-mix(in oklab, var(--accent) 22%, var(--surface))",
  "color-mix(in oklab, var(--accent) 42%, var(--surface))",
  "color-mix(in oklab, var(--accent) 66%, var(--surface))",
  "var(--accent)",
];

const CELL = 11;
const GAP = 3;
const WEEKDAY_LABELS = ["一", "三", "五"];
/** 悬停停留多久才浮出读数（契约 C-30 hover：120ms 内移开则不浮出）。 */
const HOVER_DELAY_MS = 120;
/** 点开后的浮层固定显示 5 秒。 */
const PIN_MS = 5000;

interface TipState {
  label: string;
  x: number;
  y: number;
  pinned: boolean;
}

/**
 * 阅读时长卡（驾驶舱）：今日 / 本周 / 累计 / 连续天数 + 近 90 天日历热力图。
 * 格子用按钮自绘（不引依赖），每格自己带 aria 与浮层文案（契约 C-30，新增）；
 * 配色全部取 tokens.css 变量。
 */
export function ReadingHeatmapCard({
  stats,
  loading,
  failed,
  onRetry,
}: {
  stats: ReadingStats | null;
  loading: boolean;
  failed: boolean;
  onRetry: () => void;
}) {
  const model = buildHeatmap(stats?.days ?? []);
  const [tip, setTip] = useState<TipState | null>(null);
  const hoverTimer = useRef<number | null>(null);

  /** 每次浮层读数一定来自被指向的那一格，不共享闭包里的旧值。 */
  function point(label: string, cell: HTMLElement, pinned: boolean) {
    const rect = cell.getBoundingClientRect();
    setTip({ label, x: rect.left, y: rect.top, pinned });
  }

  function clearHoverTimer() {
    if (hoverTimer.current === null) return;
    window.clearTimeout(hoverTimer.current);
    hoverTimer.current = null;
  }

  // 固定浮层 5 秒自散；换格或 Esc 会先一步收起。
  useEffect(() => {
    if (!tip?.pinned) return;
    const timer = window.setTimeout(() => setTip(null), PIN_MS);
    return () => window.clearTimeout(timer);
  }, [tip]);

  useEffect(() => clearHoverTimer, []);

  const note = loading
    ? "正在读取…"
    : failed
      ? "阅读时长暂时读不到。"
      : model.empty
        ? "近 90 天还没有阅读记录。"
        : `近 ${model.days.length} 天有 ${model.activeDays} 天在读，单日最长 ${formatDurationShort(model.maxSeconds)}。`;

  return (
    <article className="chart-card" data-chart="reading-time" data-testid="chart:reading-time" data-od-id="chart:reading-time">
      <div className="chart-head">
        <h3>阅读时长</h3>
        <p className="chart-note" data-testid="chart:reading-time:note">
          {note}
        </p>
      </div>

      {loading ? (
        <div className="chart-empty" data-testid="chart:reading-time:loading">
          正在读取…
        </div>
      ) : failed ? (
        <div className="chart-empty" data-testid="chart:reading-time:error">
          <span>数据暂不可用</span>
          <button className="load-more" type="button" onClick={onRetry}>
            重试
          </button>
        </div>
      ) : (
        <>
          <div className="dash-recent" data-testid="chart:reading-time:figures">
            <Figure label="今日" testId="reading-time-today" value={stats?.todaySeconds ?? 0} />
            <Figure label="本周" testId="reading-time-week" value={stats?.weekSeconds ?? 0} />
            <Figure label="累计" testId="reading-time-total" value={stats?.totalSeconds ?? 0} />
            <Figure label="连续" testId="reading-time-streak" value={stats?.streak ?? 0} suffix="天" />
          </div>

          {model.empty ? (
            <div className="chart-empty" data-testid="chart:reading-time:empty">
              读几分钟就会在这里留下第一格
            </div>
          ) : (
            <div className="dash-list" style={{ gap: GAP, overflowX: "auto" }} data-testid="chart:reading-time:grid">
              <div style={{ display: "flex", gap: GAP, alignItems: "flex-start" }}>
                <div style={{ display: "flex", flexDirection: "column", gap: GAP }} aria-hidden="true">
                  {Array.from({ length: 7 }, (_, row) => (
                    <span key={row} className="meta" style={{ height: CELL, fontSize: 9, lineHeight: `${CELL}px` }}>
                      {row % 2 === 0 ? WEEKDAY_LABELS[row / 2] ?? "" : ""}
                    </span>
                  ))}
                </div>
                {model.columns.map((column, index) => (
                  <div key={index} style={{ display: "flex", flexDirection: "column", gap: GAP }}>
                    {column.map((day, row) =>
                      day ? (
                        <HeatCell
                          key={day.date}
                          day={day}
                          active={tip?.label === day.label}
                          onEnter={(cell) => {
                            clearHoverTimer();
                            hoverTimer.current = window.setTimeout(() => point(day.label, cell, false), HOVER_DELAY_MS);
                          }}
                          onLeave={() => {
                            clearHoverTimer();
                            setTip((prev) => (prev?.pinned ? prev : null));
                          }}
                          onFocus={(cell) => point(day.label, cell, false)}
                          onBlur={() => setTip((prev) => (prev?.pinned ? prev : null))}
                          onPin={(cell) => point(day.label, cell, true)}
                          onEscape={() => setTip(null)}
                        />
                      ) : (
                        <span key={`blank-${index}-${row}`} aria-hidden="true" style={{ width: CELL, height: CELL }} />
                      ),
                    )}
                  </div>
                ))}
              </div>
              <div className="meta" style={{ display: "flex", gap: 6, alignItems: "center" }} data-testid="chart:reading-time:legend">
                <span>少</span>
                {LEVEL_BG.map((bg, level) => (
                  <span key={level} aria-hidden="true" style={{ width: CELL, height: CELL, background: bg, borderRadius: 2 }} />
                ))}
                <span>多</span>
              </div>
            </div>
          )}
        </>
      )}

      {tip ? (
        <div
          className={`heat-tip${tip.pinned ? " is-pinned" : ""}`}
          role="status"
          aria-live="polite"
          data-testid="heat-tip"
          style={{ left: tip.x, top: Math.max(0, tip.y - 30) }}
        >
          {tip.label}
        </div>
      ) : null}
    </article>
  );
}

function Figure({ label, testId, value, suffix }: { label: string; testId: string; value: number; suffix?: string }) {
  return (
    <div>
      <span className="meta">{label}</span>
      <b className="num" data-testid={testId}>
        {suffix ? `${value} ${suffix}` : formatDurationShort(value)}
      </b>
    </div>
  );
}

/** 一格一天：可点、可聚焦、可单独读屏（契约 C-30 每格独立 aria）。 */
function HeatCell({
  day,
  active,
  onEnter,
  onLeave,
  onFocus,
  onBlur,
  onPin,
  onEscape,
}: {
  day: HeatmapDay;
  active: boolean;
  onEnter: (cell: HTMLElement) => void;
  onLeave: () => void;
  onFocus: (cell: HTMLElement) => void;
  onBlur: () => void;
  onPin: (cell: HTMLElement) => void;
  onEscape: () => void;
}) {
  return (
    <button
      type="button"
      className={`heat-cell${active ? " is-active" : ""}`}
      data-testid={`heat-cell-${day.date}`}
      data-level={day.level}
      data-od-id="C-30"
      aria-label={day.label}
      onMouseEnter={(event) => onEnter(event.currentTarget)}
      onMouseLeave={onLeave}
      onFocus={(event) => onFocus(event.currentTarget)}
      onBlur={onBlur}
      onClick={(event) => onPin(event.currentTarget)}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          onEscape();
        }
      }}
      style={{ width: CELL, height: CELL, borderRadius: 2, background: LEVEL_BG[day.level] }}
    />
  );
}
