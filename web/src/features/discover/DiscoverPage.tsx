import { useState } from "react";
import { Link } from "react-router-dom";
import { useRecommendations } from "../../api/hooks";
import type { RecommendItem, RecommendTab } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";
import { track } from "../../lib/telemetry";
import { useIsH5 } from "../../lib/tier";
import { showToast } from "../../lib/toastStore";
import { DiscoverCard } from "./DiscoverCard";

/** 一批取多少本：契约 1–50；20 是「一屏扫得完、换一批又有新鲜感」的折中。 */
export const RECOMMEND_LIMIT = 20;

/** 三个来源：id 进请求参数（tab=），label 是页面上那排 tab 的文案。 */
const TABS: { id: RecommendTab; label: string }[] = [
  { id: "for_you", label: "为你推荐" },
  { id: "resume", label: "继续阅读" },
  { id: "random", label: "随机发现" },
];

/** 空态文案按 tab 区分：三个来源「为什么是空的」并不一样，共用一句话等于什么都没说。 */
const EMPTY_COPY: Record<RecommendTab, { title: string; description: string }> = {
  for_you: {
    title: "还没有可以推荐的书",
    description: "书库有内容之后推荐才会出现：先导入一些书，或者去书架翻几本、点开读一段，推荐会越来越准。",
  },
  resume: {
    title: "还没有读到一半的书",
    description: "打开一本书读几页，下次回到这里就能接着看没读完的那几本。",
  },
  random: {
    title: "书库还没有可以翻的书",
    description: "导入一些书之后，这里会随机递给你一本，不用挑。",
  },
};

/**
 * 发现页（/discover，两档都开）：三路推荐流 + 换一批 + 不感兴趣。
 *
 * 同一套 DOM 两档共用，形态由 CSS 换（PC 多列、H5 两列海报格）——与书架、系列页同一做法。
 * 「换一批」调 reload 而不是换查询键：列表在重取期间保持原样，不先清空闪骨架；
 * 服务端每次都会重新随机（设计 §3 的扰动 + ε 探索），所以换一批确实换内容。
 *
 * 「不感兴趣」只改本地列表 + 记一条 dislike：不做整页重取（一次点击发一次全量请求，
 * 连点几本就会排队等），后端收到 dislike 后自然不会再把它排进来。
 */
export function DiscoverPage() {
  const isH5 = useIsH5();
  const [tab, setTab] = useState<RecommendTab>("for_you");
  const rec = useRecommendations(RECOMMEND_LIMIT, tab);
  const [refreshing, setRefreshing] = useState(false);
  /** 本批里被点过「不感兴趣」的书：只影响当前这一批的显示，换批 / 换 tab 就清空 */
  const [dismissed, setDismissed] = useState<ReadonlySet<number>>(() => new Set());
  // 已经渲染过的那一批：数据换了（成功或失败回 null）就结算「换一批中」并清掉本地移除。
  // 派生 state 在渲染期结算，与 useResource 同一写法，不进 effect（避免级联渲染）。
  const [seen, setSeen] = useState(rec.data);
  if (rec.data !== seen) {
    setSeen(rec.data);
    setDismissed(new Set());
    if (refreshing) setRefreshing(false);
  }

  const items = (rec.data?.items ?? []).filter((item) => !dismissed.has(item.bookId));
  const emptyCopy = EMPTY_COPY[tab];

  function refresh() {
    if (rec.loading || refreshing) return;
    setRefreshing(true);
    rec.reload();
  }

  function dislike(book: RecommendItem) {
    track("dislike", { bookId: book.bookId, extra: book.reason });
    setDismissed((prev) => {
      const next = new Set(prev);
      next.add(book.bookId);
      return next;
    });
    showToast("不会再推荐这本");
  }

  return (
    <section className="view-discover" data-testid="view:discover" data-od-id="discover">
      <div className="page-head discover-head">
        <div>
          <p className="eyebrow">
            发现 · {rec.loading ? "读取中" : `${formatCount(items.length)} 本`}
          </p>
          <h1>发现</h1>
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

      {/* 来源切换：沿用搜索页那排轻量 tab 的版式（.search-tabs），按钮语义用 aria-pressed
          ——它是「换数据源」的开关组，不是内容分页，所以不做完整 tablist 的 roving tabindex。 */}
      <div className="search-tabs discover-tabs" role="group" aria-label="推荐来源">
        {TABS.map((entry) => (
          <button
            key={entry.id}
            type="button"
            data-testid={`discover-tab-${entry.id}`}
            aria-pressed={tab === entry.id}
            className={tab === entry.id ? "is-active" : undefined}
            onClick={() => setTab(entry.id)}
          >
            {entry.label}
          </button>
        ))}
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
          title={emptyCopy.title}
          description={emptyCopy.description}
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
            <DiscoverCard key={item.bookId} item={item} isH5={isH5} onDislike={dislike} />
          ))}
        </div>
      )}
    </section>
  );
}
