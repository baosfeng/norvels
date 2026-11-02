import type { RouteObject } from "react-router-dom";
import { AppShell } from "./layouts/AppShell";
import { HomePage } from "./features/start/HomePage";
import { ShelfPage } from "./features/shelf/ShelfPage";
import { BookPage } from "./features/book/BookPage";
import { ReaderPage } from "./features/reader/ReaderPage";
import { SearchPage } from "./features/search/SearchPage";
import { ImportPage } from "./features/import/ImportPage";
import { AuthorsPage } from "./features/authors/AuthorsPage";
import { AuthorWorksPage } from "./features/authors/AuthorWorksPage";
import { DashboardPage } from "./features/dashboard/DashboardPage";
import { AdminBooksPage } from "./features/admin/AdminBooksPage";
import { AdminBookPage } from "./features/admin/AdminBookPage";
import { SeriesPage } from "./features/series/SeriesPage";
import { SeriesDetailPage } from "./features/series/SeriesDetailPage";
import { HistoryPage } from "./features/history/HistoryPage";
import { CollectionsPage } from "./features/collections/CollectionsPage";
import { DiscoverPage } from "./features/discover/DiscoverPage";
import { ReleaseNotesPage } from "./features/release/ReleaseNotesPage";
import { LoginPage } from "./features/auth/LoginPage";
import { TierRoute } from "./layouts/TierRoute";
import { LandingRedirect, NotFoundPage, TierHomeRedirect } from "./layouts/Redirects";

/**
 * 路由表。
 *
 * - 阅读器是独立外壳（没有侧栏/顶栏），其余视图共用 AppShell。
 * - 哪些路由分档、分到哪一档，**只在 `lib/tier.ts` 里定义**（`routeTier`），这里只把一级段名
 *   交给 `TierRoute` 去查表：管理四页（导入 / 作者 / 驾驶舱 / 书籍管理）PC 专属，
 *   首页 H5 专属，其余（书架 / 详情 / 搜索 / 系列 / 系列详情 / 阅读历史 / 书单 / 阅读器）两档都在。
 */
export const routeTable: RouteObject[] = [
  { path: "/", element: <TierHomeRedirect /> },
  {
    element: <AppShell />,
    children: [
      {
        path: "home",
        element: (
          <TierRoute segment="home">
            <HomePage />
          </TierRoute>
        ),
      },
      // 发现页（为你推荐）：两档都开——H5 也需要一个「今天读点什么」的入口
      { path: "discover", element: <DiscoverPage /> },
      { path: "shelf", element: <ShelfPage /> },
      // 登录 / 注册：两档都开（H5 同样需要一个入口）。匿名可用是硬要求，
      // 它不是拦截页，只是"需要账号的动作"的落脚点。
      { path: "login", element: <LoginPage /> },
      { path: "book/:bookId", element: <BookPage /> },
      { path: "search", element: <SearchPage /> },
      {
        path: "import",
        element: (
          <TierRoute segment="import">
            <ImportPage />
          </TierRoute>
        ),
      },
      {
        path: "authors",
        element: (
          <TierRoute segment="authors">
            <AuthorsPage />
          </TierRoute>
        ),
      },
      // 某位作者的作品页：跟着作者页的档位走
      {
        path: "authors/:authorId",
        element: (
          <TierRoute segment="authors">
            <AuthorWorksPage />
          </TierRoute>
        ),
      },
      {
        path: "admin",
        element: (
          <TierRoute segment="admin">
            <AdminBooksPage />
          </TierRoute>
        ),
      },
      {
        path: "admin/books/:bookId",
        element: (
          <TierRoute segment="admin">
            <AdminBookPage />
          </TierRoute>
        ),
      },
      {
        path: "dashboard",
        element: (
          <TierRoute segment="dashboard">
            <DashboardPage />
          </TierRoute>
        ),
      },
      // 发现域（P3）：系列聚合与阅读历史两档都开（H5 用卡片列表，见各自页面）
      {
        path: "series",
        element: (
          <TierRoute segment="series">
            <SeriesPage />
          </TierRoute>
        ),
      },
      {
        path: "series/:name",
        element: (
          <TierRoute segment="series">
            <SeriesDetailPage />
          </TierRoute>
        ),
      },
      {
        path: "history",
        element: (
          <TierRoute segment="history">
            <HistoryPage />
          </TierRoute>
        ),
      },
      // 书单是收藏夹本身的管理面，两档都开（表里没有 = 两档可用，见 lib/tier.ts）
      { path: "collections", element: <CollectionsPage /> },
      // 更新日志（两档都开）：说明随二进制走，用户不必去 GitHub——NAS 上只有网页
      { path: "changelog", element: <ReleaseNotesPage /> },
      { path: "landing", element: <LandingRedirect /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
  { path: "/read/:bookId/:chapterId", element: <ReaderPage /> },
];
