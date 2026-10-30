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
 * 整张卡是「续读这一章」的链接（目标与 PC 表格里的书名一致），点击区远大于 44×44。
 * 删除入口浮在卡片右上角，是**链接之外的兄弟节点**——嵌在 <a> 里的按钮点下去会先触发跳转，
 * 于是"想删"变成"进了书"。确认态用行内替换而不是压住链接：留着 44px 的取消位，误触可退。
 */
export interface HistoryCardProps {
  row: ReadingHistoryRow;
  percent?: number;
  /** 删除入口（H5 才有）；未传则不渲染。 */
  onAskDelete?: () => void;
  /** 已展开二次确认。 */
  confirming?: boolean;
  /** 有删除在跑（含别的行）：入口与确认按钮一起禁用。 */
  busy?: boolean;
  onConfirmDelete?: () => void;
  onCancelDelete?: () => void;
}

export function HistoryCard({
  row,
  percent,
  onAskDelete,
  confirming = false,
  busy = false,
  onConfirmDelete,
  onCancelDelete,
}: HistoryCardProps) {
  const pct = typeof percent === "number" && percent > 0 ? Math.round(percent * 100) : null;
  const deletable = typeof onAskDelete === "function";
  return (
    <article
      className={"history-card" + (confirming ? " is-confirming" : "")}
      data-testid={"history-row-" + row.bookId}
      data-od-id="C-66"
    >
      <Link className="history-link" to={"/read/" + row.bookId + "/" + row.chapterId}>
        <Cover title={row.bookTitle} bookId={row.bookId} className="cover-xs" showLabel={false} />
        <div className="history-body">
          <h3 className="book-title">{row.bookTitle}</h3>
          <p className="history-chapter">
            {row.chapterTitle ? "读到 " + row.chapterTitle : "还没读到具体章节"}
          </p>
          {pct === null ? null : (
            <p className="history-progress">
              <span className="history-bar" aria-hidden="true">
                <i style={{ width: pct + "%" }} />
              </span>
              <span className="num" aria-label={"已读 " + pct + "%"}>
                {pct}%
              </span>
            </p>
          )}
          <p className="history-meta">
            <span>
              累计 <span className="num">{formatDuration(row.totalSeconds)}</span>
            </span>
            <span>
              读了 <span className="num">{row.days > 0 ? row.days + " 天" : "不到 1 天"}</span>
            </span>
            <span>
              最后 <span className="num">{formatDate(row.lastReadAt)}</span>
            </span>
          </p>
        </div>
      </Link>

      {deletable && !confirming ? (
        <button
          className="history-delete"
          type="button"
          aria-label={"删除《" + row.bookTitle + "》的阅读记录"}
          data-testid={"history-delete-" + row.bookId}
          disabled={busy}
          onClick={onAskDelete}
        >
          <span aria-hidden="true">×</span>
        </button>
      ) : null}

      {deletable && confirming ? (
        <div className="history-confirm" role="group" aria-label="确认删除这条阅读记录">
          <p className="history-confirm-text">删掉这条阅读记录？</p>
          <p className="history-confirm-note">进度和书签会留着</p>
          <div className="history-confirm-actions">
            <button
              className="btn btn-primary history-confirm-yes"
              type="button"
              data-testid={"history-confirm-" + row.bookId}
              disabled={busy}
              onClick={onConfirmDelete}
            >
              {busy ? "删除中…" : "确认删除"}
            </button>
            <button
              className="btn history-confirm-no"
              type="button"
              data-testid={"history-cancel-" + row.bookId}
              disabled={busy}
              onClick={onCancelDelete}
            >
              取消
            </button>
          </div>
        </div>
      ) : null}
    </article>
  );
}
