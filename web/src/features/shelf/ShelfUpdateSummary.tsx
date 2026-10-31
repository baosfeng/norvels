/**
 * 书架顶部「N 本书有更新」汇总条：把追更角标从单卡信息升级成一屏可见的入口。
 *
 * 数字只算当前列表里 newChapters > 0 的书（书架是分页/无限滚动取数，这里不额外发请求）；
 * 点一下把列表筛到这些书，再点回全部。没有一本书有更新时整条不渲染——不喊空口号、不占版面。
 */
export function ShelfUpdateSummary({
  count,
  active,
  onToggle,
}: {
  count: number;
  active: boolean;
  onToggle: () => void;
}) {
  if (count <= 0) return null;
  return (
    <div className="shelf-updates">
      <button
        type="button"
        className={`shelf-updates-btn${active ? " is-active" : ""}`}
        aria-pressed={active}
        data-testid="shelf-updates"
        title={`当前列表里 ${count} 本书有新章`}
        onClick={onToggle}
      >
        <span className="shelf-updates-dot" aria-hidden="true" />
        <span className="num">{count}</span> 本书有更新
        <span className="shelf-updates-sep" aria-hidden="true">
          ·
        </span>
        <span className="shelf-updates-hint" aria-hidden="true">
          {active ? "显示全部" : "只看这些"}
        </span>
      </button>
    </div>
  );
}
