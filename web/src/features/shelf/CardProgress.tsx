import type { ReadingProgress } from "../../api/types";
import { chapterPosition, readPercent } from "./progressText";

/**
 * 书卡上的阅读进度（H5 网格卡）：一条细进度条 + 已读百分比，下面一行「第 N 章 / 共 M 章」。
 *
 * 为什么百分比单独占进度条那行：390 屏三列时单卡只有 ~111px，把「· 12%」接在章节后面
 * 会被挤成省略号——百分比跟着进度条走才不会被截掉；章节口径这一行永远是完整的一句话。
 * 没读过的书没有进度条，只留「共 M 章」（章数原本在元信息行，网格卡不再有那一行）。
 */
export function CardProgress({
  bookId,
  chapterCount,
  progress,
}: {
  bookId: number;
  chapterCount: number;
  /** 没有进度记录（没读过 / 进度接口失败）时只显示章数 */
  progress?: ReadingProgress;
}) {
  const position = chapterPosition(progress?.chapterIdx ?? null, chapterCount);
  if (!position) return null;
  const percent = readPercent(progress);
  return (
    <div
      className="book-progress"
      data-testid={`book-progress-${bookId}`}
      data-read={percent === null ? "0" : "1"}
    >
      {percent === null ? null : (
        <span className="book-progress-head">
          <span className="book-progress-bar" aria-hidden="true">
            <i style={{ width: `${percent}%` }} />
          </span>
          <span className="book-progress-pct num" aria-label={`已读 ${percent}%`}>
            {percent}%
          </span>
        </span>
      )}
      <span className="book-progress-text">{position}</span>
    </div>
  );
}
