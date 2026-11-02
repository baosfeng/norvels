import type { RefObject } from "react";
import { NavLink } from "react-router-dom";
import { useStats } from "../api/hooks";
import { useReadingProgress } from "../api/hooks";
import { Cover } from "../components/Cover";
import { formatCount } from "../lib/format";
import { useIsH5 } from "../lib/tier";
import { SideAccount } from "./SideAccount";
import { SideVersion } from "./SideVersion";

/**
 * 侧栏（PC 常驻 / H5 作为汉堡抽屉内容）。
 * 两档都开：首页（仅 H5）/ **发现** / 书架 / 搜索 / **系列** / **阅读历史** / **书单**。
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

      {/* 导航项与路由档位同源：两档都开的能力（首页 / 发现 / 书架 / 搜索 / 系列 / 阅读历史 / 书单）两档都出现，
          PC 专属的管理四项才按档位不渲染。不引入底部 tab bar——外壳重排风险大，抽屉已够用。 */}
      <nav className="side-nav" aria-label="主导航">
        {isH5 ? (
          <NavLink to="/home" onClick={onNavigate} data-od-id="C-04">
            首页
          </NavLink>
        ) : null}
        {/* 发现：推荐流入口，两档都开——放在书架之前，因为它是「读什么」的第一个落点 */}
        <NavLink to="/discover" onClick={onNavigate} data-testid="nav-discover">
          发现
        </NavLink>
        <NavLink to="/shelf" onClick={onNavigate} data-od-id="C-02">
          书架
          {badges.books !== null ? <span className="nav-count num">{badges.books}</span> : null}
        </NavLink>
        <NavLink to="/search" onClick={onNavigate} data-od-id="C-03">
          搜索
        </NavLink>
        <NavLink to="/series" onClick={onNavigate} data-testid="nav-series" data-od-id="C-05">
          系列
        </NavLink>
        <NavLink to="/history" onClick={onNavigate} data-testid="nav-history" data-od-id="C-06">
          阅读历史
        </NavLink>
        {/* 书单管理不再藏在书架工具栏里：它有独立页，入口与筛选能力平级 */}
        <NavLink to="/collections" onClick={onNavigate} data-testid="nav-collections">
          书单
        </NavLink>
        {/* 更新日志：产品里能直接看到每个版本改了什么（说明内嵌在二进制里，不依赖外网） */}
        <NavLink to="/changelog" onClick={onNavigate} data-testid="nav-changelog">
          更新日志
        </NavLink>
        {isH5 ? null : (
          <>
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

      {/* 侧栏底部整块（账号区 + 页脚）吸底：侧栏在 800px 高的屏上内容已经溢出
          （10 个导航项 + 统计），不吸底的话「登录」与版本号都要滚到底才看得到。
          整块吸底而不是只吸页脚：只吸页脚会把账号区压在下面。 */}
      <div className="side-bottom">
        {/* 账号区放在侧栏底部：PC 常驻可见，H5 就在汉堡抽屉底部。不强制拦截浏览 */}
        <SideAccount onNavigate={onNavigate} />

        {/* 页脚两行：版本位（点进更新日志）+ 数据归属。版本号是「这版改了什么」的入口，
            PC 与 H5 抽屉是同一份 DOM，一处改两档都生效 */}
        <div className="side-foot meta">
          <SideVersion onNavigate={onNavigate} />
          <span>所有数据保存在你的电脑里</span>
        </div>
      </div>
    </aside>
  );
}
