import { Link, useNavigate } from "react-router-dom";
import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import type { ApiError } from "../../api/client";
import { useBooks, useReadingProgress } from "../../api/hooks";
import { Cover } from "../../components/Cover";
import { MenuIcon, SearchIcon } from "../../components/Icons";
import { EmptyPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";
import { showToast } from "../../lib/toastStore";
import { useNavDrawer } from "../../layouts/navDrawer";

/**
 * H5 首屏（≤768）：继续阅读 → 最近在读 → 搜索入口 → 书架列表。
 * PC 不会渲染这个视图（TierRoute minTier="h5"），所以这里不写 PC 分支。
 */
export function HomePage() {
  const navigate = useNavigate();
  const nav = useNavDrawer();
  const progress = useReadingProgress();
  const recent = useBooks({ sort: "updated", page: 1, size: 5 });
  const [q, setQ] = useState("");
  const recentRowsRef = useRef<HTMLDivElement | null>(null);
  const [awaitRetry, setAwaitRetry] = useState(false);

  // 重试成功后焦点交给最近入库的第一行（C-71 离开错误态）
  useEffect(() => {
    if (!awaitRetry || recent.loading || recent.error) return;
    queueMicrotask(() => setAwaitRetry(false));
    recentRowsRef.current?.querySelector<HTMLElement>("a")?.focus();
  }, [awaitRetry, recent.loading, recent.error]);

  const resume = progress.data?.items?.[0] ?? null;
  const others = (progress.data?.items ?? []).slice(1, 4);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const keyword = q.trim();
    if (keyword) navigate(`/search?q=${encodeURIComponent(keyword)}`);
  }

  function onSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    // Esc 清空并保持焦点；输入法组合期间的 Enter 不提交（C-43）
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
    <div className="h5-home" data-testid="view:home" data-od-id="home">
      <div className="h5-top">
        <button
          className="icon-btn h5-burger"
          type="button"
          data-od-id="C-15"
          onClick={(event) => nav.toggle(event.currentTarget)}
          aria-label="打开导航"
          aria-expanded={nav.open}
        >
          <MenuIcon size={20} />
        </button>
        <div>
          <p className="eyebrow">norvels · 本地书库</p>
          <h1>今晚读点什么</h1>
        </div>
      </div>

      {progress.loading ? (
        <LoadingPanel title="正在读取阅读进度" description="稍等一下，正在取回你上次读到的地方。" />
      ) : resume ? (
        <Link
          className="resume-card"
          to={`/read/${resume.bookId}/${resume.chapterId}`}
          data-testid="home:resume"
          data-od-id="C-42"
          data-legacy-od-id="home:resume"
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
        <div data-od-id="C-45">
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

      <form className="h5-search" onSubmit={submit} role="search" data-od-id="C-43">
        <span className="search-icon">
          <SearchIcon />
        </span>
        <input
          className="input"
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={onSearchKeyDown}
          placeholder="搜书名、作者、章节标题…"
          aria-label="搜索"
        />
        <button className="btn" type="submit">
          搜索
        </button>
      </form>

      <div className="h5-section">
        <div className="h5-section-head">
          <h2>最近在读</h2>
          <Link className="meta h5-section-link" to="/shelf" data-od-id="C-44">
            全部书架 →
          </Link>
        </div>
        {others.length > 0 ? (
          <div className="h5-list">
            {others.map((item) => (
              <Link className="h5-row" key={item.bookId} to={`/read/${item.bookId}/${item.chapterId}`} data-od-id="C-46">
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

      <div className="h5-section">
        <div className="h5-section-head">
          <h2>最近入库</h2>
          <Link className="meta h5-section-link" to="/shelf" data-od-id="C-47">
            全部 →
          </Link>
        </div>
        {recent.loading ? (
          <LoadingPanel title="正在读取书目" description="稍等一下，正在取回最近入库的书。" />
        ) : recent.error ? (
          <RecentError error={recent.error} onRetry={retryRecent} />
        ) : (
          <div className="h5-list" ref={recentRowsRef}>
            {(recent.data?.items ?? []).map((book) => (
              <Link className="h5-row" key={book.id} to={`/book/${book.id}`} data-od-id="C-48">
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
    </div>
  );
}
/** 最近入库的错误面板：重试按钮带契约锚点 C-71，文案口径与通用错误面板一致。 */
function RecentError({ error, onRetry }: { error: ApiError | null; onRetry: () => void }) {
  const description =
    error?.kind === "timeout"
      ? "读取超时了。稍等片刻再试一次，已经导入的书目不会丢失。"
      : error?.status === 503
        ? "数据库暂时不可用，请稍后再试。"
        : "读取时出了点问题。可以重试，或者稍后再回来看。";
  return (
    <div className="state-panel is-error">
      <span className="state-mark" aria-hidden="true">
        !
      </span>
      <h2>书目没能加载</h2>
      <p>{description}</p>
      <div className="state-actions">
        <button className="btn btn-primary" type="button" data-od-id="C-71" onClick={onRetry}>
          重试
        </button>
      </div>
    </div>
  );
}
