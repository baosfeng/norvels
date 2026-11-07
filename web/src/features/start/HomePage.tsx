import { Link, useLocation, useNavigate } from "react-router-dom";
import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { useBooks, useReadingProgress, useReadingStats } from "../../api/hooks";
import { Cover } from "../../components/Cover";
import { MenuIcon, SearchIcon } from "../../components/Icons";
import { EmptyPanel, ErrorPanel, LoadingPanel, RowSkeleton } from "../../components/StatePanel";
import { formatCount, formatDuration } from "../../lib/format";
import { SEARCH_HISTORY_MIN_LEN, readSearchHistory, rememberSearch } from "../../lib/searchHistory";
import { showToast } from "../../lib/toastStore";
import { useNavDrawer } from "../../layouts/navDrawer";
import { tierFallbackText } from "../../layouts/tierFallback";
import { SceneRecommendBlock } from "../recommend/SceneRecommendBlock";

/**
 * H5 首屏（≤768）：继续阅读 → 最近在读 → 搜索入口 → 书架列表。
 * PC 不会渲染这个视图（TierRoute minTier="h5"），所以这里不写 PC 分支。
 */
export function HomePage() {
  const navigate = useNavigate();
  const location = useLocation();
  const nav = useNavDrawer();
  const progress = useReadingProgress();
  const recent = useBooks({ sort: "updated", page: 1, size: 5 });
  // 「今天读多久」只用已有的 /api/stats/reading，不新增端点；取最少的 7 天即可
  const stats = useReadingStats(7);
  const [q, setQ] = useState("");
  // 惰性初值：历史只在挂载时读一次；本页没有「清空历史」入口，不需要活状态
  const [history] = useState<string[]>(readSearchHistory);
  const recentRowsRef = useRef<HTMLDivElement | null>(null);
  const [awaitRetry, setAwaitRetry] = useState(false);

  // 重试成功后焦点交给最近入库的第一行（离开错误态）
  useEffect(() => {
    if (!awaitRetry || recent.loading || recent.error) return;
    queueMicrotask(() => setAwaitRetry(false));
    recentRowsRef.current?.querySelector<HTMLElement>("a")?.focus();
  }, [awaitRetry, recent.loading, recent.error]);

  // 错档落点：窄屏访问系列 / 阅读历史等 PC 页会被 TierRoute 送回首页，
  // 这里把「为什么回来了」用 4 秒提示说清楚，并把一次性 state 清掉，刷新不再重复播报。
  useEffect(() => {
    const text = tierFallbackText((location.state as { from?: string } | null)?.from);
    if (!text) return;
    showToast(text);
    navigate(location.pathname, { replace: true, state: null });
  }, [location.state, location.pathname, navigate]);

  const progressItems = progress.data?.items ?? [];
  const resume = progressItems[0] ?? null;
  // 首屏「在读」的完整集合：继续读卡片那本 + 下面「最近在读」两到三本
  const reading = progressItems.slice(0, 4);
  const others = reading.slice(1);
  // 首页两栏原来都取 books?sort=updated，同一批书占两次屏（实测前 3 本完全重合）。
  // 最近入库只留「在读区没出现过的书」，一本不剩就整区不渲染——留个空标题只会白占一屏。
  const readingIds = new Set(reading.map((item) => item.bookId));
  const fresh = (recent.data?.items ?? []).filter((book) => !readingIds.has(book.id));
  const recentVisible = recent.loading || Boolean(recent.error) || fresh.length > 0;

  // 今天读多久：不足 1 分钟 / 取数失败 / 取值缺失时整行不出现，不给首屏添一条没有信息量的字
  const todaySeconds = stats.data?.todaySeconds ?? 0;
  const todayLine = !stats.loading && !stats.error && todaySeconds >= 60;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const keyword = q.trim();
    if (!keyword) return;
    // 与搜索页同一份历史：首页提交也要记，否则「从首页搜的」永远进不了历史
    if (keyword.length >= SEARCH_HISTORY_MIN_LEN) rememberSearch(keyword);
    navigate(`/search?q=${encodeURIComponent(keyword)}`);
  }

  function pickHistory(keyword: string) {
    rememberSearch(keyword);
    navigate(`/search?q=${encodeURIComponent(keyword)}`);
  }

  function onSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    // Esc 清空并保持焦点；输入法组合期间的 Enter 不提交
    if (event.key === "Escape") {
      event.preventDefault();
      setQ("");
      return;
    }
    if (event.key === "Enter" && event.nativeEvent.isComposing) event.preventDefault();
  }

  function retryRecent() {
    setAwaitRetry(true);
    recent.reload();
    showToast("已重新读取书目。");
    try {
      window.scrollTo({ top: 0 });
    } catch {
      /* 测试环境没有 scrollTo */
    }
  }

  return (
    <div className="h5-home" data-testid="view:home">
      <div className="h5-top">
        <button
          className="icon-btn h5-burger"
          type="button"
          onClick={(event) => nav.toggle(event.currentTarget)}
          aria-label="打开导航"
          aria-expanded={nav.open}
        >
          <MenuIcon size={20} />
        </button>
        <div>
          <p className="eyebrow">norvels · 本地书库</p>
          <h1>今晚读点什么</h1>
          {todayLine ? (
            <p className="meta h5-today" data-testid="home:today">
              今天已读 {formatDuration(todaySeconds)}
              {stats.data && stats.data.streak > 1 ? ` · 连续 ${stats.data.streak} 天` : ""}
            </p>
          ) : null}
        </div>
      </div>

      {progress.loading ? (
        <LoadingPanel title="正在读取阅读进度" description="稍等一下，正在取回你上次读到的地方。" />
      ) : resume ? (
        <Link
          className="resume-card"
          to={`/read/${resume.bookId}/${resume.chapterId}`}
          data-testid="home:resume"
        >
          <Cover title={resume.bookTitle} showLabel={false} />
          <div className="resume-body">
            <span className="meta">继续阅读 · {resume.chapterTitle}</span>
            <strong>{resume.bookTitle}</strong>
            <span className="meta">已读 {Math.round(resume.percent * 100)}%</span>
            <span className="resume-bar">
              <i style={{ width: `${Math.round(resume.percent * 100)}%` }} />
            </span>
            <span className="btn btn-primary resume-cta">接着读</span>
          </div>
        </Link>
      ) : (
        <div>
          <EmptyPanel
            title="还没有阅读记录"
            description="从书架挑一本开始读，下次回来就能从这里接着看。"
            actions={
              <Link className="btn btn-primary" to="/shelf">
                去书架
              </Link>
            }
          />
        </div>
      )}

      <form className="h5-search" onSubmit={submit} role="search">
        <span className="search-icon">
          <SearchIcon />
        </span>
        <input
          className="input"
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={onSearchKeyDown}
          placeholder="搜书名…"
          aria-label="搜索"
        />
        <button className="btn" type="submit">
          搜索
        </button>
      </form>

      {/* 最近搜过的词直接可点：首页是「今晚读点什么」的决策屏，让用户重新打一遍
          上次搜过的字是把最常做的事放在最远的位置。清空入口只放在搜索页一处，
          同一个「清空」在两页各有一套确认逻辑会分叉。 */}
      {history.length > 0 ? (
        <div className="h5-search-history" data-testid="home-search-history">
          <span className="meta">最近搜过</span>
          <div className="chips">
            {history.map((keyword) => (
              <button className="chip" key={keyword} type="button" onClick={() => pickHistory(keyword)}>
                {keyword}
              </button>
            ))}
          </div>
        </div>
      ) : null}

      {/* 推荐位：首屏最贵的位置之一，放在「最近在读」之前——用户还没有阅读记录时，
          这一块是首页唯一能给出「接下来读什么」的答案（空/失败时整块不渲染，不留空框）。 */}
      <SceneRecommendBlock scene="home" />

      <div className="h5-section">
        <div className="h5-section-head">
          <h2>最近在读</h2>
          <Link className="meta h5-section-link" to="/shelf">
            全部书架 →
          </Link>
        </div>
        {others.length > 0 ? (
          <div className="h5-list" data-testid="home:reading">
            {others.map((item) => (
              <Link className="h5-row" key={item.bookId} to={`/read/${item.bookId}/${item.chapterId}`}>
                <Cover title={item.bookTitle} showLabel={false} />
                <span className="h5-row-text">
                  <b>{item.bookTitle}</b>
                  <span className="meta">{item.chapterTitle}</span>
                </span>
                <span className="h5-row-pct num">{Math.round(item.percent * 100)}%</span>
              </Link>
            ))}
          </div>
        ) : (
          <p className="meta">读完几本之后，这里会列出最近打开过的书。</p>
        )}
      </div>

      {recentVisible ? (
        <div className="h5-section" data-testid="home:recent-section">
          <div className="h5-section-head">
            <h2>最近入库</h2>
            <Link className="meta h5-section-link" to="/shelf">
              全部 →
            </Link>
          </div>
          {recent.loading ? (
            // 行骨架而不是转圈面板：形状与真实行一致，取回数据时列表不跳
            <RowSkeleton count={3} testId="home:recent-skeleton" />
          ) : recent.error ? (
            <ErrorPanel error={recent.error} title="书目没能加载" onRetry={retryRecent} testId="home:recent-error" />
          ) : (
            <div className="h5-list" ref={recentRowsRef} data-testid="home:recent-books">
              {fresh.map((book) => (
                <Link className="h5-row" key={book.id} to={`/book/${book.id}`}>
                  <Cover title={book.title} showLabel={false} />
                  <span className="h5-row-text">
                    <b>{book.title}</b>
                    <span className="meta">{book.authorName || "佚名"}</span>
                  </span>
                  <span className="h5-row-pct num">{formatCount(book.chapterCount)} 章</span>
                </Link>
              ))}
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
}
