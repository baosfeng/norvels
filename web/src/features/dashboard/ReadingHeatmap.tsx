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

/**
 * 阅读时长卡（驾驶舱）：今日 / 本周 / 累计 / 连续天数 + 近 90 天日历热力图。
 * 格子用 div 网格自绘（不引依赖，也不用 SVG 的 viewBox 缩放），配色全部取 tokens.css 变量。
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
            <div>
              <span className="meta">今日</span>
              <b className="num" data-testid="reading-time-today">
                {formatDurationShort(stats?.todaySeconds ?? 0)}
              </b>
            </div>
            <div>
              <span className="meta">本周</span>
              <b className="num" data-testid="reading-time-week">
                {formatDurationShort(stats?.weekSeconds ?? 0)}
              </b>
            </div>
            <div>
              <span className="meta">累计</span>
              <b className="num" data-testid="reading-time-total">
                {formatDurationShort(stats?.totalSeconds ?? 0)}
              </b>
            </div>
            <div>
              <span className="meta">连续</span>
              <b className="num" data-testid="reading-time-streak">
                {stats?.streak ?? 0} 天
              </b>
            </div>
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
                    {column.map((day, row) => (
                      <HeatCell key={row} day={day} />
                    ))}
                  </div>
                ))}
              </div>
              <div className="meta" style={{ display: "flex", gap: 6, alignItems: "center" }} data-testid="chart:reading-time:legend">
                <span>少</span>
                {LEVEL_BG.map((bg, level) => (
                  <span
                    key={level}
                    aria-hidden="true"
                    style={{ width: CELL, height: CELL, background: bg, borderRadius: 2 }}
                  />
                ))}
                <span>多</span>
              </div>
            </div>
          )}
        </>
      )}
    </article>
  );
}

function HeatCell({ day }: { day: HeatmapDay | null }) {
  if (!day) {
    return <span aria-hidden="true" style={{ width: CELL, height: CELL }} />;
  }
  return (
    <span
      data-testid={`heat-cell-${day.date}`}
      data-level={day.level}
      title={day.label}
      aria-label={day.label}
      role="img"
      style={{
        width: CELL,
        height: CELL,
        borderRadius: 2,
        background: LEVEL_BG[day.level],
        display: "inline-block",
      }}
    />
  );
}
