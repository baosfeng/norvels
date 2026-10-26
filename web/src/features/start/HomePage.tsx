import { Link, useNavigate } from "react-router-dom";
import { useState, type FormEvent } from "react";
import { useBooks, useReadingProgress } from "../../api/hooks";
import { Cover } from "../../components/Cover";
import { MenuIcon, SearchIcon } from "../../components/Icons";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";
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

  const resume = progress.data?.items?.[0] ?? null;
  const others = (progress.data?.items ?? []).slice(1, 4);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const keyword = q.trim();
    if (keyword) navigate(`/search?q=${encodeURIComponent(keyword)}`);
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
        <Link className="resume-card" to={`/read/${resume.bookId}/${resume.chapterId}`} data-testid="home:resume" data-od-id="home:resume">
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
        <EmptyPanel
          title="还没有阅读记录"
          description="从书架挑一本开始读，下次回来就能从这里接着看。"
          actions={
            <Link className="btn btn-primary" to="/shelf">
              去书架
            </Link>
          }
        />
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
          <Link className="meta" to="/shelf">
            全部书架 →
          </Link>
        </div>
        {others.length > 0 ? (
          <div className="h5-list">
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

      <div className="h5-section">
        <div className="h5-section-head">
          <h2>最近入库</h2>
          <Link className="meta" to="/shelf">
            全部 →
          </Link>
        </div>
        {recent.loading ? (
          <LoadingPanel title="正在读取书目" description="稍等一下。" />
        ) : recent.error ? (
          <ErrorPanel error={recent.error} title="书目没能加载" onRetry={recent.reload} />
        ) : (
          <div className="h5-list">
            {(recent.data?.items ?? []).map((book) => (
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
    </div>
  );
}