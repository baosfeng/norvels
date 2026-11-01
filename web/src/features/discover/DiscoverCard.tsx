import { useRef } from "react";
import { Link } from "react-router-dom";
import type { RecommendItem } from "../../api/types";
import { Cover } from "../../components/Cover";
import { track, useImpression } from "../../lib/telemetry";

/**
 * 推荐卡：复用书架的 .book-card / .book-link / .book-card-body 结构，
 * 只在正文列多一行「推荐理由」——理由由后端给（「因为你常看武侠」），前端不拼装。
 *
 * 卡片上有三个交互：
 * - 点卡进详情（跳转前记一次 click）；
 * - 曝光：可见 ≥50% 且 ≥500ms 记一次 impression（每张卡一次）；
 * - 「不感兴趣」：按钮是链接的**兄弟**而不是子节点（按钮套链接是无效嵌套，点了还会连带跳转），
 *   悬浮 / 键盘聚焦时出现，触屏常驻（见 ix-recommend.css 的 hover 媒体查询）。
 *
 * impression 与 click 的 extra 都带本条推荐理由：后端存进 events.extra，
 * 之后才能回答「哪条理由带来的点击最有效」。
 */
export function DiscoverCard({
  item,
  isH5,
  onDislike,
}: {
  item: RecommendItem;
  isH5: boolean;
  /** 点「不感兴趣」：父级负责记 dislike、把这张卡从列表里摘掉并给提示 */
  onDislike: (item: RecommendItem) => void;
}) {
  const cardRef = useRef<HTMLElement | null>(null);
  const signal = {
    bookId: item.bookId,
    categoryL1: item.categoryL1,
    categoryL2: item.categoryL2,
    extra: item.reason,
  };
  useImpression(cardRef, signal);

  return (
    <article
      ref={cardRef}
      className="book-card discover-card"
      data-book-id={item.bookId}
      data-testid={`rec-${item.bookId}`}
    >
      <Link
        className="book-link"
        to={`/book/${item.bookId}`}
        state={{ from: "discover", reason: item.reason }}
        onClick={() => track("click", signal)}
      >
        <Cover title={item.title} bookId={item.bookId} hasCover={item.hasCover} showLabel={!isH5} />
        <div className="book-card-body">
          <h3 className="book-title">{item.title}</h3>
          <p className="book-author">{item.authorName || "佚名"}</p>
          {item.categoryL1 || item.categoryL2 ? (
            <p className="book-meta discover-cat">
              {item.categoryL1 ? <span>{item.categoryL1}</span> : null}
              {item.categoryL2 ? <span>{item.categoryL2}</span> : null}
            </p>
          ) : null}
          <p className="discover-reason">{item.reason}</p>
        </div>
      </Link>
      <button
        className="discover-dislike"
        type="button"
        data-testid={`dislike-${item.bookId}`}
        aria-label={`不再推荐《${item.title}》`}
        onClick={() => onDislike(item)}
      >
        不感兴趣
      </button>
    </article>
  );
}
