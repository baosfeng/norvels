import { useState } from "react";
import { Link } from "react-router-dom";
import { useReadingHistory, useReadingProgressAll } from "../../api/hooks";
import type { ReadingHistoryResult, ReadingHistoryRow } from "../../api/types";
import { ErrorPanel, EmptyPanel } from "../../components/StatePanel";
import { formatCount, formatDate, formatDuration } from "../../lib/format";
import { useIsH5 } from "../../lib/tier";
import { showToast } from "../../lib/toastStore";
import { HistoryCard } from "./HistoryCard";
import { HistorySkeleton } from "./HistorySkeleton";
import { groupHistory, type HistoryGroup } from "./historyGroups";
import { useHistoryDeletion } from "./useHistoryDeletion";

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
 *
 * 删除只在 H5 卡片上做入口（PC 表格的删除留给后续，避免在这一版给表格塞行内确认）。
 * 页头「清空」两档都有：它是整页动作，与承载形态无关。
 */
export function HistoryPage() {
  const isH5 = useIsH5();
  const [page, setPage] = useState(1);
  const feed = useReadingHistory({ page, size: PAGE_SIZE });
  const progress = useReadingProgressAll();
  const live: ReadingHistoryResult | null = feed.data;
  const pages = Math.max(1, Math.ceil((live?.total ?? 0) / PAGE_SIZE));

  const deletion = useHistoryDeletion({
    data: live,
    reload: feed.reload,
    onPageEmptied: () => {
      if (page <= 1) return false;
      setPage((current) => Math.max(1, current - 1));
      return true;
    },
    notify: showToast,
  });

  // 乐观删除期间展示 deletion.view；服务端新数据一到派生结果自动让位（见 useHistoryDeletion）
  const data = deletion.view ?? live;
  const rows = data?.items ?? [];
  const total = data?.total ?? 0;

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
          <p className="eyebrow">阅读 · {feed.loading ? "读取中" : formatCount(total) + " 本"}</p>
          <h1>阅读历史</h1>
          <p className="sub">按书聚合的最后阅读位置与累计时长，最近读过的排在前面</p>
        </div>
        {rows.length > 0 && live ? (
          <ClearAll
            open={deletion.pending === "all"}
            busy={deletion.busy !== null}
            onAsk={() => deletion.ask("all")}
            onCancel={deletion.cancel}
            onConfirm={() => void deletion.confirm()}
          />
        ) : null}
      </div>

      <HistoryBody
        isH5={isH5}
        feed={feed}
        rows={rows}
        percentByBook={percentMap(progress.data?.items ?? [])}
        deletion={deletion}
        pager={{ page, pages, go }}
      />
    </section>
  );
}

/** 四种状态（加载 / 出错 / 空 / 有数据）+ 分页：与页头分开，免得一个函数里挤两件事。 */
function HistoryBody(props: {
  isH5: boolean;
  feed: ReturnType<typeof useReadingHistory>;
  rows: ReadingHistoryRow[];
  percentByBook: Map<number, number>;
  deletion: ReturnType<typeof useHistoryDeletion>;
  pager: { page: number; pages: number; go: (next: number) => void };
}) {
  const { isH5, feed, rows, percentByBook, deletion, pager } = props;

  if (feed.loading) return <HistorySkeleton count={isH5 ? 4 : 5} />;
  if (feed.error) {
    return (
      <div data-od-id="C-68">
        <ErrorPanel error={feed.error} title="阅读历史没能加载" onRetry={feed.reload} testId="history-error" />
      </div>
    );
  }
  if (rows.length === 0) {
    return (
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
    );
  }

  return (
    <>
      {isH5 ? (
        <HistoryCards groups={groupHistory(rows)} percentByBook={percentByBook} deletion={deletion} />
      ) : (
        <HistoryTable groups={groupHistory(rows)} deletion={deletion} />
      )}
      <HistoryPager page={pager.page} pages={pager.pages} go={pager.go} />
    </>
  );
}

function HistoryPager({ page, pages, go }: { page: number; pages: number; go: (next: number) => void }) {
  if (pages <= 1) return null;
  return (
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
  );
}

/** 页头「清空」：默认只是一个低调的入口，点开就地换成二段式确认，不弹原生窗口。 */
function ClearAll(props: {
  open: boolean;
  busy: boolean;
  onAsk: () => void;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { open, busy, onAsk, onCancel, onConfirm } = props;
  if (!open) {
    return (
      <button
        className="btn history-clear"
        type="button"
        data-testid="history-clear"
        disabled={busy}
        onClick={onAsk}
      >
        清空历史
      </button>
    );
  }
  return (
    <div className="history-clear-confirm" role="group" aria-label="确认清空阅读历史">
      <p className="history-clear-note">清空后进度和书签仍在</p>
      <div className="history-clear-actions">
        <button
          className="btn btn-primary"
          type="button"
          data-testid="history-clear-confirm"
          disabled={busy}
          onClick={onConfirm}
        >
          {busy ? "清空中…" : "确认清空"}
        </button>
        <button className="btn" type="button" data-testid="history-clear-cancel" disabled={busy} onClick={onCancel}>
          取消
        </button>
      </div>
    </div>
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

function HistoryCards({
  groups,
  percentByBook,
  deletion,
}: {
  groups: HistoryGroup[];
  percentByBook: Map<number, number>;
  deletion: ReturnType<typeof useHistoryDeletion>;
}) {
  return (
    <div className="history-groups" data-testid="history-live">
      {groups.map((group) => (
        <section className="history-group" key={group.bucket} data-testid={`history-group-${group.bucket}`}>
          <h2 className="history-group-head">{group.label}</h2>
          <div className="history-cards">
            {group.rows.map((row) => (
              <HistoryCard
                key={row.bookId}
                row={row}
                percent={percentByBook.get(row.bookId)}
                onAskDelete={() => deletion.ask(row.bookId)}
                confirming={deletion.pending === row.bookId}
                busy={deletion.busy !== null}
                onConfirmDelete={() => void deletion.confirm()}
                onCancelDelete={deletion.cancel}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function HistoryTable({
  groups,
  deletion,
}: {
  groups: HistoryGroup[];
  deletion: ReturnType<typeof useHistoryDeletion>;
}) {
  // 删一列而不是只删 H5：同一份数据上的「删掉这条」不该因为窗口变宽就消失。
  // PC 用行内 ✕（表格里放不下两段式按钮），确认沿用页头的 .history-clear-confirm 形态
  // 换成一个就地的小确认行，避免和 H5 卡片分叉两套交互。
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
            <th scope="col">
              <span className="sr-only">操作</span>
            </th>
          </tr>
        </thead>
        {groups.map((group) => (
          <tbody key={group.bucket}>
            <tr className="history-group-row">
              <th scope="rowgroup" colSpan={6}>
                {group.label}
              </th>
            </tr>
            {group.rows.map((row) => (
              <HistoryRow
                key={row.bookId}
                row={row}
                confirming={deletion.pending === row.bookId}
                busy={deletion.busy !== null}
                onAskDelete={() => deletion.ask(row.bookId)}
                onConfirmDelete={() => void deletion.confirm()}
                onCancelDelete={deletion.cancel}
              />
            ))}
          </tbody>
        ))}
      </table>
    </div>
  );
}

function HistoryRow({
  row,
  confirming,
  busy,
  onAskDelete,
  onConfirmDelete,
  onCancelDelete,
}: {
  row: ReadingHistoryRow;
  confirming: boolean;
  busy: boolean;
  onAskDelete: () => void;
  onConfirmDelete: () => void;
  onCancelDelete: () => void;
}) {
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
      <td className="history-row-del">
        {confirming ? (
          <span className="history-row-confirm" role="group" aria-label="确认删除这条阅读记录">
            <button
              className="btn btn-primary"
              type="button"
              data-testid={`history-confirm-${row.bookId}`}
              disabled={busy}
              onClick={onConfirmDelete}
            >
              {busy ? "删除中…" : "确认删除"}
            </button>
            <button className="btn" type="button" data-testid="history-row-cancel" disabled={busy} onClick={onCancelDelete}>
              取消
            </button>
          </span>
        ) : (
          <button
            className="icon-btn history-del-btn"
            type="button"
            aria-label={`删除《${row.bookTitle}》的阅读记录`}
            data-testid={`history-delete-${row.bookId}`}
            disabled={busy}
            onClick={onAskDelete}
          >
            ✕
          </button>
        )}
      </td>
    </tr>
  );
}
