import { useState } from "react";
import { Link } from "react-router-dom";
import { useRecommendations } from "../../api/hooks";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";
import { useIsH5 } from "../../lib/tier";
import { DiscoverCard } from "./DiscoverCard";

/** 一批取多少本：契约 1–50；20 是「一屏扫得完、换一批又有新鲜感」的折中。 */
export const RECOMMEND_LIMIT = 20;

/**
 * 发现页（/discover，两档都开）：「为你推荐」卡片流 + 「换一批」。
 *
 * 同一套 DOM 两档共用，形态由 CSS 换（PC 四列、H5 两列海报格）——与书架、系列页同一做法。
 * 「换一批」调 reload 而不是换查询键：列表在重取期间保持原样，不先清空闪骨架；
 * 服务端每次都会重新随机（设计 §3 的扰动 + ε 探索），所以换一批确实换内容。
 */
export function DiscoverPage() {
  const isH5 = useIsH5();
  const rec = useRecommendations(RECOMMEND_LIMIT);
  const [refreshing, setRefreshing] = useState(false);
  // 已经渲染过的那一批：数据换了（成功或失败回 null）就结算「换一批中」。
  // 派生 state 在渲染期结算，与 useResource 同一写法，不进 effect（避免级联渲染）。
  const [seen, setSeen] = useState(rec.data);
  if (rec.data !== seen) {
    setSeen(rec.data);
    if (refreshing) setRefreshing(false);
  }

  const items = rec.data?.items ?? [];

  function refresh() {
    if (rec.loading || refreshing) return;
    setRefreshing(true);
    rec.reload();
  }

  return (
    <section className="view-discover" data-testid="view:discover" data-od-id="discover">
      <div className="page-head discover-head">
        <div>
          <p className="eyebrow">
            发现 · {rec.loading ? "读取中" : `${formatCount(items.length)} 本推荐`}
          </p>
          <h1>为你推荐</h1>
          <p className="sub">按你的点击、阅读与收藏挑出来的；每换一批都会不太一样。</p>
        </div>
        {rec.loading ? null : (
          <div className="discover-head-actions">
            <button
              className="btn btn-primary"
              type="button"
              data-testid="discover-refresh"
              aria-busy={refreshing}
              disabled={refreshing}
              onClick={refresh}
            >
              {refreshing ? "换一批中…" : "换一批"}
            </button>
          </div>
        )}
      </div>

      {rec.loading ? (
        <LoadingPanel
          title="正在挑书"
          description="正在按你最近的口味从库里挑一批，稍等一下。"
          testId="discover-loading"
        />
      ) : rec.error ? (
        <ErrorPanel error={rec.error} title="推荐没能加载" onRetry={rec.reload} testId="discover-error" />
      ) : items.length === 0 ? (
        <EmptyPanel
          title="还没有可以推荐的书"
          description="书库有内容之后推荐才会出现：先导入一些书，或者去书架翻几本、点开读一段，推荐会越来越准。"
          testId="discover-empty"
          actions={
            <Link className="btn btn-primary" to="/shelf">
              去书架
            </Link>
          }
        />
      ) : (
        <div className="discover-grid" data-testid="discover-grid">
          {items.map((item) => (
            <DiscoverCard key={item.bookId} item={item} isH5={isH5} />
          ))}
        </div>
      )}
    </section>
  );
}
