import { Link } from "react-router-dom";
import { useSimilarRecommendations } from "../../api/hooks";
import type { RecommendItem } from "../../api/types";
import { Cover } from "../../components/Cover";
import { ErrorPanel, RowSkeleton } from "../../components/StatePanel";
import { track } from "../../lib/telemetry";
import { sceneCopy } from "../recommend/sceneCopy";

/** 一次给几张：横向一排 6 张，多于一屏宽就在块内横向滚动，不把页面撑宽。 */
export const SIMILAR_LIMIT = 6;

/**
 * 详情页「你可能也喜欢」：GET /api/recommend?similar=<bookId>，横向 6 张。
 *
 * 三态：加载给两行骨架（形状与真卡一致，回来时不跳）；失败给可重试的错误态；
 * **空的时候整块不渲染**——详情页底部挂一个「暂无相似书目」的空框，除了占地方没有任何信息。
 *
 * 点击埋点 click（extra = 这条推荐的理由）：它证明「从详情页跳到另一本书」这条路是否有效，
 * 与发现页的点击分开统计。
 */
export function SimilarBooks({ bookId }: { bookId: number }) {
  const rec = useSimilarRecommendations(bookId, SIMILAR_LIMIT);
  const items = rec.data?.items ?? [];

  if (rec.loading) {
    return (
      <section className="similar-block" data-testid="book:similar-loading">
        <h2 className="similar-title">{sceneCopy("book").title}</h2>
        <RowSkeleton count={2} testId="book:similar-skeleton" />
      </section>
    );
  }
  if (rec.error) {
    return (
      <section className="similar-block" data-testid="book:similar-error-block">
        <h2 className="similar-title">{sceneCopy("book").title}</h2>
        <ErrorPanel error={rec.error} title="相似推荐没能加载" onRetry={rec.reload} testId="book:similar-error" />
      </section>
    );
  }
  // 没有相似书目：整块不渲染（不是渲染一个空框）
  if (items.length === 0) return null;

  return (
    <section className="similar-block" data-testid="book:similar">
      <div className="similar-head">
        <h2 className="similar-title">{sceneCopy("book").title}</h2>
        <span className="meta" data-testid="book:similar-note">
          {sceneCopy("book").note}
        </span>
      </div>
      <div className="similar-row">
        {items.map((item) => (
          <SimilarCard key={item.bookId} item={item} />
        ))}
      </div>
    </section>
  );
}

function SimilarCard({ item }: { item: RecommendItem }) {
  function onClick() {
    // 只报「点了哪本 + 哪条理由」：来源书（fromBookId）没有对应字段——
    // value 在后端是「停留秒数 / 星级」的量纲，塞书 id 会把聚合污染掉；
    // extra 已经放理由了。要单独统计这条路径，得后端加一个 source 字段。
    track("click", {
      bookId: item.bookId,
      authorId: item.authorId,
      categoryL1: item.categoryL1,
      categoryL2: item.categoryL2,
      extra: item.reason,
    });
  }

  return (
    <article className="book-card similar-card" data-testid={`similar-${item.bookId}`}>
      <Link className="book-link" to={`/book/${item.bookId}`} onClick={onClick}>
        <Cover title={item.title} bookId={item.bookId} hasCover={item.hasCover} showLabel={false} />
        <div className="book-card-body">
          <h3 className="book-title">{item.title}</h3>
          <p className="book-author">{item.authorName || "佚名"}</p>
          <p className="discover-reason">{item.reason}</p>
        </div>
      </Link>
    </article>
  );
}
