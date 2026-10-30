import { useIsH5 } from "../../lib/tier";

/**
 * 搜索结果的骨架屏：结构与 SearchResultList 的条目一致（小封面 + 两行字），
 * 加载时占位不跳动，也不出现转圈。
 *
 * 不复用 StatePanel 的 CardSkeleton：那是书目网格的 4 列卡，形状与搜索行不同，
 * 换上会让结果区在数据到达时整块重排。
 */
export function SearchResultSkeleton({ count = 3 }: { count?: number }) {
  const isH5 = useIsH5();
  return (
    <div className="search-books" aria-hidden="true" data-testid="search-skeleton">
      {Array.from({ length: count }, (_, i) => (
        <div className="search-book is-skeleton" key={i}>
          <div className="sk sk-cover search-sk-cover" />
          <div className="sk-lines search-sk-lines">
            <div className="sk sk-line w60" />
            <div className="sk sk-line w40" />
            {isH5 ? null : <div className="sk sk-line w80" />}
          </div>
        </div>
      ))}
    </div>
  );
}
