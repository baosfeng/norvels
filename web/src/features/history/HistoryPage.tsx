import { useState } from "react";
import { Link } from "react-router-dom";
import { useReadingHistory } from "../../api/hooks";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount, formatDate, formatDuration } from "../../lib/format";

const PAGE_SIZE = 20;

/**
 * 阅读历史（P3 发现域，PC 专属）：按书聚合的最后章节 / 最后阅读时间 / 累计时长 / 阅读天数。
 * 一行点进去直接续读那一章（后端给的就是最后读到的章），不额外查进度接口。
 */
export function HistoryPage() {
  const [page, setPage] = useState(1);
  const feed = useReadingHistory({ page, size: PAGE_SIZE });
  const rows = feed.data?.items ?? [];
  const total = feed.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  function go(next: number) {
    setPage(Math.min(Math.max(1, next), pages));
    try {
      window.scrollTo({ top: 0 });
    } catch {
      /* 测试环境没有 scrollTo */
    }
  }

  return (
    <section data-testid="view:history" data-od-id="history">
      <div className="page-head">
        <div>
          <p className="eyebrow">阅读 · {feed.loading ? "读取中" : `${formatCount(total)} 本`}</p>
          <h1>阅读历史</h1>
          <p className="sub">按书聚合的最后阅读位置与累计时长，最近读过的排在前面</p>
        </div>
      </div>

      {feed.loading ? (
        <LoadingPanel title="正在整理阅读记录" description="正在汇总每本书的最后位置与累计时长。" />
      ) : feed.error ? (
        <div data-od-id="C-68">
          <ErrorPanel error={feed.error} title="阅读历史没能加载" onRetry={feed.reload} testId="history-error" />
        </div>
      ) : rows.length === 0 ? (
        <div data-od-id="C-67">
          <EmptyPanel
            title="还没有阅读记录"
            description="打开一本书读上几分钟，这里就会记下最后读到的章节和累计时长。"
            testId="history-empty"
            actions={
              <Link className="btn btn-primary" to="/shelf">
                去书架挑一本
              </Link>
            }
          />
        </div>
      ) : (
        <>
          <div className="table-wrap">
            <table className="table" data-testid="history-live">
              <thead>
                <tr>
                  <th scope="col">书</th>
                  <th scope="col">最后章节</th>
                  <th scope="col">最后阅读</th>
                  <th scope="col">累计时长</th>
                  <th scope="col">阅读天数</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.bookId} data-testid={`history-row-${row.bookId}`} data-od-id="C-66">
                    <td>
                      <Link className="book-title" to={`/read/${row.bookId}/${row.chapterId}`}>
                        {row.bookTitle}
                      </Link>
                    </td>
                    <td className="meta">{row.chapterTitle || "—"}</td>
                    <td className="num">{formatDate(row.lastReadAt)}</td>
                    <td className="num">{formatDuration(row.totalSeconds)}</td>
                    <td className="num">{row.days > 0 ? `${row.days} 天` : "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {pages > 1 ? (
            <nav className="pagination" aria-label="分页" data-testid="history-pagination">
              <button type="button" aria-label="上一页" data-od-id="C-69" disabled={page <= 1} onClick={() => go(page - 1)}>
                ←
              </button>
              <span className="is-current num">{page}</span>
              <span className="meta">/ {pages}</span>
              <button type="button" aria-label="下一页" data-od-id="C-70" disabled={page >= pages} onClick={() => go(page + 1)}>
                →
              </button>
            </nav>
          ) : null}
        </>
      )}
    </section>
  );
}
