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
import { TierRoute } from "./layouts/TierRoute";
import { LandingRedirect, NotFoundPage, TierHomeRedirect } from "./layouts/Redirects";

/**
 * 路由表。
 * - 阅读器是独立外壳（没有侧栏/顶栏），其余视图共用 AppShell。
 * - 管理类视图（驾驶舱 / 导入 / 作者）包在 TierRoute 里：H5 下不是隐藏，而是根本不渲染。
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
      {
        path: "dashboard",
        element: (
          <TierRoute>
            <DashboardPage />
          </TierRoute>
        ),
      },
      { path: "landing", element: <LandingRedirect /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
  { path: "/read/:bookId/:chapterId", element: <ReaderPage /> },
];