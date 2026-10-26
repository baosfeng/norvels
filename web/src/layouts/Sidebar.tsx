import type { RefObject } from "react";
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
export function Sidebar({
  onNavigate,
  navRef,
}: {
  onNavigate?: () => void;
  navRef?: RefObject<HTMLElement | null>;
}) {
  const isH5 = useIsH5();
  const stats = useStats();
  const progress = useReadingProgress();

  const resume = progress.data?.items?.[0] ?? null;
  const books = stats.data?.books ?? 0;
  const authors = stats.data?.authors ?? 0;
  // 库存读取出错时徽标显示「—」（不是 0）；正常为 0 时不出现徽标
  const badges = {
    books: stats.error ? "—" : books > 0 ? formatCount(books) : null,
    authors: stats.error ? "—" : authors > 0 ? formatCount(authors) : null,
  };

  return (
    <aside ref={navRef} className="sidebar" data-testid="sidebar" data-od-id="sidebar" aria-label="站点导航">
      <NavLink className="brand" to={isH5 ? "/home" : "/shelf"} onClick={onNavigate} data-od-id="C-01">
        <strong>norvels</strong>
        <span>本地中文小说库</span>
      </NavLink>

      <nav className="side-nav" aria-label="主导航">
        {isH5 ? (
          <NavLink to="/home" onClick={onNavigate} data-od-id="C-04">
            首页
          </NavLink>
        ) : null}
        <NavLink to="/shelf" onClick={onNavigate} data-od-id="C-02">
          书架
          {badges.books !== null ? <span className="nav-count num">{badges.books}</span> : null}
        </NavLink>
        <NavLink to="/search" onClick={onNavigate} data-od-id="C-03">
          搜索
        </NavLink>
        {isH5 ? null : (
          <>
            <NavLink to="/series" onClick={onNavigate} data-testid="nav-series" data-od-id="C-05">
              系列
            </NavLink>
            <NavLink to="/history" onClick={onNavigate} data-testid="nav-history" data-od-id="C-06">
              阅读历史
            </NavLink>
            <NavLink to="/admin" onClick={onNavigate} data-testid="nav-admin" data-od-id="C-07">
              书籍管理
            </NavLink>
            <NavLink to="/dashboard" onClick={onNavigate} data-od-id="C-08">
              驾驶舱
            </NavLink>
            <NavLink to="/authors" onClick={onNavigate} data-od-id="C-09">
              作者
              {badges.authors !== null ? <span className="nav-count num">{badges.authors}</span> : null}
            </NavLink>
            <NavLink to="/import" onClick={onNavigate} data-od-id="C-10">
              导入管理
            </NavLink>
          </>
        )}
      </nav>

      {resume ? (
        <div className="side-block">
          <p className="side-label">继续阅读</p>
          <NavLink
            className="continue-card"
            to={`/read/${resume.bookId}/${resume.chapterId}`}
            onClick={onNavigate}
            data-od-id="C-11"
          >
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
