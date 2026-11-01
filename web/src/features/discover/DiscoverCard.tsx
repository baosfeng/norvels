import { useRef } from "react";
import { Link } from "react-router-dom";
import type { RecommendItem } from "../../api/types";
import { Cover } from "../../components/Cover";
import { track, useImpression } from "../../lib/telemetry";

/**
 * 推荐卡：复用书架的 .book-card / .book-link / .book-card-body 结构，
 * 只在正文列多一行「推荐理由」——理由由后端给（「因为你常看武侠」），前端不拼装。
 *
 * 埋点两个：曝光（可见 ≥50% 且 ≥500ms，每张卡一次）与点击（跳详情前记，家在卡片链接上）。
 */
export function DiscoverCard({ item, isH5 }: { item: RecommendItem; isH5: boolean }) {
  const cardRef = useRef<HTMLElement | null>(null);
  const signal = { bookId: item.bookId, categoryL1: item.categoryL1, categoryL2: item.categoryL2 };
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
    </article>
  );
}
