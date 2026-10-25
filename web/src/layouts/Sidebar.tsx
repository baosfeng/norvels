import { NavLink } from "react-router-dom";
import { useStats } from "../api/hooks";
import { useReadingProgress } from "../api/hooks";
import { Cover } from "../components/Cover";
import { formatCount } from "../lib/format";
import { useIsH5 } from "../lib/tier";

/**
 * PC 侧栏（H5 下作为汉堡抽屉内容渲染）。
 * 管理入口（书籍管理 / 驾驶舱 / 作者 / 导入管理）只在 PC 出现——H5 是条件渲染，不是隐藏。
 */
export function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const isH5 = useIsH5();
  const stats = useStats();
  const progress = useReadingProgress();

  const resume = progress.data?.items?.[0] ?? null;
  const books = stats.data?.books ?? 0;
  const authors = stats.data?.authors ?? 0;

  return (
    <aside className="sidebar" data-testid="sidebar" data-od-id="sidebar" aria-label="站点导航">
      <NavLink className="brand" to={isH5 ? "/home" : "/shelf"} onClick={onNavigate}>
        <strong>norvels</strong>
        <span>本地中文小说库</span>
      </NavLink>

      <nav className="side-nav" aria-label="主导航">
        {isH5 ? (
          <NavLink to="/home" onClick={onNavigate}>
            首页
          </NavLink>
        ) : null}
        <NavLink to="/shelf" onClick={onNavigate}>
          书架
          {books > 0 ? <span className="nav-count num">{formatCount(books)}</span> : null}
        </NavLink>
        <NavLink to="/search" onClick={onNavigate}>
          搜索
        </NavLink>
        {isH5 ? null : (
          <>
            <NavLink to="/admin" onClick={onNavigate} data-testid="nav-admin">
              书籍管理
            </NavLink>
            <NavLink to="/dashboard" onClick={onNavigate}>
              驾驶舱
            </NavLink>
            <NavLink to="/authors" onClick={onNavigate}>
              作者
              {authors > 0 ? <span className="nav-count num">{formatCount(authors)}</span> : null}
            </NavLink>
            <NavLink to="/import" onClick={onNavigate}>
              导入管理
            </NavLink>
          </>
        )}
      </nav>

      {resume ? (
        <div className="side-block">
          <p className="side-label">继续阅读</p>
          <NavLink className="continue-card" to={`/read/${resume.bookId}/${resume.chapterId}`} onClick={onNavigate}>
            <Cover title={resume.bookTitle} className="cover-xs" showLabel={false} />
            <span className="continue-text">
              <strong>{resume.bookTitle}</strong>
              <span className="meta">
                {resume.chapterTitle} · {Math.round(resume.percent * 100)}%
              </span>
            </span>
          </NavLink>
        </div>
      ) : null}

      <div className="side-stats">
        <div>
          <strong className="num">{stats.loading ? "—" : formatCount(books)}</strong>
          <span>本书</span>
        </div>
        <div>
          <strong className="num">
            {stats.loading ? "—" : formatCount(stats.data?.chapters ?? 0)}
          </strong>
          <span>章节</span>
        </div>
        <div>
          <strong className="num">
            {stats.loading ? "—" : formatCount(stats.data?.words ?? 0)}
          </strong>
          <span>字数</span>
        </div>
      </div>

      <p className="side-foot meta">所有数据保存在你的电脑里</p>
    </aside>
  );
}