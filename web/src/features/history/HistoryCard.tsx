import { Link } from "react-router-dom";
import type { ReadingHistoryRow } from "../../api/types";
import { Cover } from "../../components/Cover";
import { formatDate, formatDuration } from "../../lib/format";

/**
 * H5 的一张历史卡：封面 + 书名 + 「读到 第 N 章 …」+ 进度百分比 + 三项带标签的时间。
 *
 * 三项数字必须带标签：之前是「10 分钟 1 天 1 分钟前」三连，量纲不同却排成一排，
 * 读起来像同一种东西的三个取值。现在写清「累计 10 分钟 · 读了 1 天 · 1 分钟前」。
 *
 * 整张卡就是「续读这一章」的链接（目标与 PC 表格里的书名一致），点击区远大于 44×44。
 */
export function HistoryCard({ row, percent }: { row: ReadingHistoryRow; percent?: number }) {
  const pct = typeof percent === "number" && percent > 0 ? Math.round(percent * 100) : null;
  return (
    <article className="history-card" data-testid={`history-row-${row.bookId}`} data-od-id="C-66">
      <Link className="history-link" to={`/read/${row.bookId}/${row.chapterId}`}>
        <Cover title={row.bookTitle} bookId={row.bookId} className="cover-xs" showLabel={false} />
        <div className="history-body">
          <h3 className="book-title">{row.bookTitle}</h3>
          <p className="history-chapter">
            {row.chapterTitle ? `读到 ${row.chapterTitle}` : "还没读到具体章节"}
          </p>
          {pct === null ? null : (
            <p className="history-progress">
              <span className="history-bar" aria-hidden="true">
                <i style={{ width: `${pct}%` }} />
              </span>
              <span className="num" aria-label={`已读 ${pct}%`}>
                {pct}%
              </span>
            </p>
          )}
          <p className="history-meta">
            <span>
              累计 <span className="num">{formatDuration(row.totalSeconds)}</span>
            </span>
            <span>
              读了 <span className="num">{row.days > 0 ? `${row.days} 天` : "不到 1 天"}</span>
            </span>
            <span>
              最后 <span className="num">{formatDate(row.lastReadAt)}</span>
            </span>
          </p>
        </div>
      </Link>
    </article>
  );
}
