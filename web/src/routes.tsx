import type { RouteObject } from "react-router-dom";
import { AppShell } from "./layouts/AppShell";
import { HomePage } from "./features/start/HomePage";
import { ShelfPage } from "./features/shelf/ShelfPage";
import { BookPage } from "./features/book/BookPage";
import { ReaderPage } from "./features/reader/ReaderPage";
import { SearchPage } from "./features/search/SearchPage";
import { ImportPage } from "./features/import/ImportPage";
import { AuthorsPage } from "./features/authors/AuthorsPage";
import { DashboardPage } from "./features/dashboard/DashboardPage";
import { AdminBooksPage } from "./features/admin/AdminBooksPage";
import { AdminBookPage } from "./features/admin/AdminBookPage";
import { SeriesPage } from "./features/series/SeriesPage";
import { SeriesDetailPage } from "./features/series/SeriesDetailPage";
import { HistoryPage } from "./features/history/HistoryPage";
import { TierRoute } from "./layouts/TierRoute";
import { LandingRedirect, NotFoundPage, TierHomeRedirect } from "./layouts/Redirects";

/**
 * 路由表。
 * - 阅读器是独立外壳（没有侧栏/顶栏），其余视图共用 AppShell。
 * - 管理类视图（驾驶舱 / 导入 / 作者 / 书籍管理）包在 TierRoute 里：H5 下不是隐藏，而是根本不渲染。
 */
export const routeTable: RouteObject[] = [
  { path: "/", element: <TierHomeRedirect /> },
  {
    element: <AppShell />,
    children: [
      {
        path: "home",
        element: (
          <TierRoute minTier="h5">
            <HomePage />
          </TierRoute>
        ),
      },
      { path: "shelf", element: <ShelfPage /> },
      { path: "book/:bookId", element: <BookPage /> },
      { path: "search", element: <SearchPage /> },
      {
        path: "import",
        element: (
          <TierRoute>
            <ImportPage />
          </TierRoute>
        ),
      },
      {
        path: "authors",
        element: (
          <TierRoute>
            <AuthorsPage />
          </TierRoute>
        ),
      },
      // 书籍管理（作者后台）：PC 专属，H5 下由 TierRoute 直接不渲染
      {
        path: "admin",
        element: (
          <TierRoute>
            <AdminBooksPage />
          </TierRoute>
        ),
      },
      {
        path: "admin/books/:bookId",
        element: (
          <TierRoute>
            <AdminBookPage />
          </TierRoute>
        ),
      },
      {
        path: "dashboard",
        element: (
          <TierRoute>
            <DashboardPage />
          </TierRoute>
        ),
      },
      // 发现域（P3）：系列聚合与阅读历史都是 PC 视野，H5 下由 TierRoute 直接不渲染
      {
        path: "series",
        element: (
          <TierRoute>
            <SeriesPage />
          </TierRoute>
        ),
      },
      {
        path: "series/:name",
        element: (
          <TierRoute>
            <SeriesDetailPage />
          </TierRoute>
        ),
      },
      {
        path: "history",
        element: (
          <TierRoute>
            <HistoryPage />
          </TierRoute>
        ),
      },
      { path: "landing", element: <LandingRedirect /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
  { path: "/read/:bookId/:chapterId", element: <ReaderPage /> },
];