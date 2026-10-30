import { useState } from "react";
import { Link } from "react-router-dom";
import { useReadingHistory, useReadingProgressAll } from "../../api/hooks";
import type { ReadingHistoryRow } from "../../api/types";
import { ErrorPanel, EmptyPanel } from "../../components/StatePanel";
import { formatCount, formatDate, formatDuration } from "../../lib/format";
import { useIsH5 } from "../../lib/tier";
import { HistoryCard } from "./HistoryCard";
import { HistorySkeleton } from "./HistorySkeleton";
import { groupHistory, type HistoryGroup } from "./historyGroups";

const PAGE_SIZE = 20;

/**
 * 阅读历史（发现域，**两档都开**）：按书聚合的最后章节 / 最后阅读时间 / 累计时长 / 阅读天数。
 * 一行点进去直接续读那一章（后端给的就是最后读到的章）。
 *
 * 两档只换承载形态：PC 是表格（一屏对多行更好扫），H5 是卡片列表（窄屏表格必然横向挤爆）。
 * 形态由 `useTier()` 条件渲染决定，不是 CSS 隐藏；分页两档都有。
 *
 * 分组（今天 / 昨天 / 更早）两档都做：历史页的主要问题不是「找不到」，而是
 * 「最近读的那几本被埋在一长串里」，按日历日切开后最上面一组就是答案。
 * 进度百分比来自阅读进度接口（按 bookId 对齐），取不到就不显示，不拿 0% 冒充。
 */
export function HistoryPage() {
  const isH5 = useIsH5();
  const [page, setPage] = useState(1);
  const feed = useReadingHistory({ page, size: PAGE_SIZE });
  const progress = useReadingProgressAll();
  const rows = feed.data?.items ?? [];
  const total = feed.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const percentByBook = percentMap(progress.data?.items ?? []);
  const groups = groupHistory(rows);

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
        <HistorySkeleton count={isH5 ? 4 : 5} />
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
          {isH5 ? <HistoryCards groups={groups} percentByBook={percentByBook} /> : <HistoryTable groups={groups} />}

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

/** 进度百分比按书对齐；没有进度记录的书 absence → 不显示。 */
function percentMap(items: { bookId: number; percent: number }[]): Map<number, number> {
  const map = new Map<number, number>();
  for (const item of items) {
    if (Number.isFinite(item.percent) && item.percent > 0) map.set(item.bookId, item.percent);
  }
  return map;
}

function HistoryCards({ groups, percentByBook }: { groups: HistoryGroup[]; percentByBook: Map<number, number> }) {
  return (
    <div className="history-groups" data-testid="history-live">
      {groups.map((group) => (
        <section className="history-group" key={group.bucket} data-testid={`history-group-${group.bucket}`}>
          <h2 className="history-group-head">{group.label}</h2>
          <div className="history-cards">
            {group.rows.map((row) => (
              <HistoryCard key={row.bookId} row={row} percent={percentByBook.get(row.bookId)} />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function HistoryTable({ groups }: { groups: HistoryGroup[] }) {
  return (
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
        {groups.map((group) => (
          <tbody key={group.bucket}>
            <tr className="history-group-row">
              <th scope="rowgroup" colSpan={5}>
                {group.label}
              </th>
            </tr>
            {group.rows.map((row) => (
              <HistoryRow key={row.bookId} row={row} />
            ))}
          </tbody>
        ))}
      </table>
    </div>
  );
}

function HistoryRow({ row }: { row: ReadingHistoryRow }) {
  return (
    <tr data-testid={`history-row-${row.bookId}`} data-od-id="C-66">
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
  );
}
