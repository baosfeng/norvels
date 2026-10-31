import { useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { Link } from "react-router-dom";
import { useHealthIssues } from "../../api/hooks";
import type { HealthIssueItem, HealthIssueKind } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount, formatWords } from "../../lib/format";
import { HEALTH_ISSUE_META } from "./healthIssues";

/** 每页条数：与其它列表接口同一档（抽屉里一屏能滚完，翻页也不至于太碎）。 */
export const HEALTH_ISSUE_PAGE_SIZE = 50;

/**
 * 抽屉内可聚焦的元素（按 DOM 顺序）：焦点陷阱用它算「第一个/最后一个」。
 *
 * 三态面板是条件渲染的（没渲染就不在 DOM 里），所以这里只过滤显式隐藏的元素；
 * 不用 getClientRects 判可见——jsdom 没有布局，恒为空数组，会把可聚焦列表清空。
 */
function focusableNodes(root: HTMLElement): HTMLElement[] {
  const nodes = root.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])');
  return [...nodes].filter((node) => !node.hidden && node.getAttribute("aria-hidden") !== "true");
}

/**
 * 健康卡下钻抽屉：列出某一类问题的书 / 章节，标题链到单书管理页。
 *
 * 四个取舍：
 *   - 明细按页取（不是一次拉全部）：缺简介的真实库有几百本，前端不分页会把抽屉拖慢；
 *   - 条目标题指向 /admin/books/:id：用户看到问题后下一步必然是去改那一本；
 *   - modal 语义：aria-modal + 焦点陷阱（Tab / Shift+Tab 在抽屉内循环），
 *     关闭后焦点回到打开它的那个入口按钮（见 HealthCard），键盘用户不会被丢到遮罩下方；
 *   - 打开即聚焦「关闭」，Esc 与点遮罩都能关。
 */
export function HealthDrawer({ kind, onClose }: { kind: HealthIssueKind; onClose: () => void }) {
  const meta = HEALTH_ISSUE_META[kind];
  const [page, setPage] = useState(1);
  const { data, loading, error, reload } = useHealthIssues(kind, page, HEALTH_ISSUE_PAGE_SIZE);
  const closeRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    closeRef.current?.focus();
  }, []);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / HEALTH_ISSUE_PAGE_SIZE));
  const items = data?.items ?? [];

  /** 焦点陷阱：Tab 走到边界就绕回另一端（没有它，最后一个条目 Tab 下去会跑到遮罩下方）。 */
  function trapTab(event: ReactKeyboardEvent<HTMLElement>) {
    if (event.key !== "Tab") return;
    const nodes = focusableNodes(event.currentTarget);
    if (nodes.length === 0) return;
    const first = nodes[0];
    const last = nodes[nodes.length - 1];
    const active = document.activeElement;
    const outside = !event.currentTarget.contains(active);
    if (event.shiftKey ? active === first || outside : active === last || outside) {
      event.preventDefault();
      (event.shiftKey ? last : first).focus();
    }
  }

  return (
    <>
      <div className="overlay is-open" role="presentation" data-testid="health-drawer-overlay" onClick={onClose} />
      <aside
        className="drawer is-open health-drawer"
        role="dialog"
        aria-modal="true"
        aria-labelledby="health-drawer-title"
        data-testid="health-drawer"
        data-kind={kind}
        onKeyDown={trapTab}
      >
        <div className="health-drawer-head">
          <div>
            <p className="eyebrow">库健康状况</p>
            <h2 id="health-drawer-title" data-testid="health-drawer-title">
              {meta.title}
            </h2>
            <p className="meta" data-testid="health-drawer-count">
              {data ? `共 ${formatCount(total)} ${meta.unit}` : "正在统计…"}
            </p>
          </div>
          <button className="btn btn-sm" type="button" ref={closeRef} data-testid="health-drawer-close" onClick={onClose}>
            关闭
          </button>
        </div>

        <p className="meta health-drawer-rule">{meta.rule}</p>

        <div className="health-drawer-body">
          {loading && !data ? (
            <LoadingPanel title="正在读取明细" description="稍等一下。" testId="health-drawer-loading" />
          ) : error ? (
            <ErrorPanel error={error} title="明细没能读取" onRetry={reload} testId="health-drawer-error" />
          ) : items.length === 0 ? (
            <EmptyPanel title={meta.empty} description={meta.emptyHint} testId="health-drawer-empty" />
          ) : (
            <ul className="health-issue-list" data-testid="health-issue-list">
              {items.map((item) => (
                <IssueRow key={`${item.bookId}-${item.chapterId ?? 0}`} item={item} chapterLevel={meta.chapterLevel} />
              ))}
            </ul>
          )}
        </div>

        {data && total > HEALTH_ISSUE_PAGE_SIZE ? (
          <div className="health-drawer-foot">
            <button
              className="btn btn-sm"
              type="button"
              disabled={page <= 1}
              data-testid="health-drawer-prev"
              onClick={() => setPage((prev) => Math.max(1, prev - 1))}
            >
              上一页
            </button>
            <span className="meta" data-testid="health-drawer-page">
              第 {page} / {pages} 页
            </span>
            <button
              className="btn btn-sm"
              type="button"
              disabled={page >= pages}
              data-testid="health-drawer-next"
              onClick={() => setPage((prev) => Math.min(pages, prev + 1))}
            >
              下一页
            </button>
          </div>
        ) : null}
      </aside>
    </>
  );
}

/** 一条明细：标题是可点的入口，副行给「这本书 / 这一章是什么情况」。 */
function IssueRow({ item, chapterLevel }: { item: HealthIssueItem; chapterLevel: boolean }) {
  // 章节级给精确字数（与目录/正文编辑器同一口径）；书级总量用「万」量级，和书籍管理页一致。
  const subtitle = chapterLevel
    ? `${item.title} · ${formatWords(item.wordCount)}`
    : `${item.author || "缺作者"} · ${item.chapterCount} 章 · ${formatCount(item.wordCount)} 字`;
  return (
    <li className="health-issue-item">
      <Link
        className="health-issue-row"
        to={`/admin/books/${item.bookId}`}
        data-testid={`health-issue-${item.bookId}${item.chapterId ? `-${item.chapterId}` : ""}`}
      >
        <span className="health-issue-title">{chapterLevel ? item.chapterTitle : item.title}</span>
        <span className="meta">{subtitle}</span>
        <span className="health-issue-go" aria-hidden="true">
          去管理 →
        </span>
      </Link>
    </li>
  );
}