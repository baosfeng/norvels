import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useCapabilities, useRecommendations } from "../../api/hooks";
import type { RecommendItem, RecommendTab } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";
import { track } from "../../lib/telemetry";
import { useIsH5 } from "../../lib/tier";
import { showToast } from "../../lib/toastStore";
import { DiscoverCard, type DislikeScope } from "./DiscoverCard";

/** 一批取多少本：契约 1–50；20 是「一屏扫得完、换一批又有新鲜感」的折中。 */
export const RECOMMEND_LIMIT = 20;

/**
 * 三个来源的兜底名册：正常路径从 capabilities.recommend.tabs 取（服务端是唯一事实源，
 * 多端不会各拼一套文案）；这里只在能力清单还没到 / 旧服务端不下发时顶上。
 */
const FALLBACK_TABS: { id: RecommendTab; label: string }[] = [
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
 * 发现页（/discover，两档都开）：三路推荐流 + 换一批 + 不感兴趣（书 / 作者两级）。
 *
 * 同一套 DOM 两档共用，形态由 CSS 换（PC 多列、H5 两列海报格）——与书架、系列页同一做法。
 * 「换一批」调 reload 而不是换查询键：列表在重取期间保持原样，不先清空闪骨架；
 * 服务端每次都会重新随机（设计 §3 的扰动 + ε 探索），所以换一批确实换内容。
 *
 * 「不感兴趣」只改本地列表 + 记一条 dislike：不做整页重取（一次点击发一次全量请求，
 * 连点几本就会排队等），后端收到 dislike 后自然不会再把它排进来。
 * 选「这个作者」时本地还会把同一作者的其它卡一起摘掉——用户的意思是「别给我推这个人」，
 * 只摘当前这一本，下一批又会把同作者的书递上来。
 */
export function DiscoverPage() {
  const isH5 = useIsH5();
  const caps = useCapabilities();
  const tabs: { id: RecommendTab; label: string }[] = useMemo(() => {
    const fromServer = caps.data?.recommend?.tabs;
    if (!fromServer || fromServer.length === 0) return FALLBACK_TABS;
    return fromServer.map((t) => ({ id: t.id as RecommendTab, label: t.label }));
  }, [caps.data]);
  const [tab, setTab] = useState<RecommendTab>("for_you");
  const rec = useRecommendations(RECOMMEND_LIMIT, tab);
  const [refreshing, setRefreshing] = useState(false);
  /** 本批里被点过「不感兴趣」的书 / 作者：只影响当前这一批的显示，换批或换 tab 就清空 */
  const [dismissed, setDismissed] = useState<ReadonlySet<number>>(() => new Set());
  const [dismissedAuthors, setDismissedAuthors] = useState<ReadonlySet<number>>(() => new Set());
  /** 展开着菜单的那张卡：菜单状态放这里，保证同一时刻只有一个菜单 */
  const [menuFor, setMenuFor] = useState<number | null>(null);
  // 已经渲染过的那一批：数据换了（成功或失败回 null）就结算「换一批中」并清掉本地移除。
  // 派生 state 在渲染期结算，与 useResource 同一写法，不进 effect（避免级联渲染）。
  const [seen, setSeen] = useState(rec.data);
  if (rec.data !== seen) {
    setSeen(rec.data);
    setDismissed(new Set());
    setDismissedAuthors(new Set());
    setMenuFor(null);
    if (refreshing) setRefreshing(false);
  }

  const items = (rec.data?.items ?? []).filter(
    (item) =>
      !dismissed.has(item.bookId) &&
      !(item.authorId !== undefined && dismissedAuthors.has(item.authorId)),
  );
  const emptyCopy = EMPTY_COPY[tab];

  /**
   * 菜单开着时：点菜单和触发按钮以外的地方收起，Esc 也收起。
   * 监听挂在 document 上（而不是每张卡各自监听）：一张卡一个 document 监听会让
   * 打开第二张卡的菜单时第一张的监听先跑，状态互相打断。
   */
  useEffect(() => {
    if (menuFor === null) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Element | null;
      if (target?.closest(".dislike-menu") || target?.closest(".discover-dislike")) return;
      setMenuFor(null);
    };
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") setMenuFor(null);
    };
    document.addEventListener("pointerdown", onPointerDown, true);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown, true);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [menuFor]);

  function refresh() {
    if (rec.loading || refreshing) return;
    setRefreshing(true);
    rec.reload();
  }

  function dislike(book: RecommendItem, scope: DislikeScope) {
    setMenuFor(null);
    if (scope === "author" && book.authorId) {
      const authorId = book.authorId;
      track("dislike", { bookId: book.bookId, authorId, extra: book.reasonCode || book.reason });
      setDismissedAuthors((prev) => new Set(prev).add(authorId));
      showToast("不会再推荐这个作者");
      return;
    }
    track("dislike", { bookId: book.bookId, extra: book.reasonCode || book.reason });
    setDismissed((prev) => new Set(prev).add(book.bookId));
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
        {tabs.map((entry) => (
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
            <DiscoverCard
              key={item.bookId}
              item={item}
              isH5={isH5}
              menuOpen={menuFor === item.bookId}
              onToggleMenu={(open) => setMenuFor(open ? item.bookId : null)}
              onDislike={dislike}
            />
          ))}
        </div>
      )}
    </section>
  );
}
