import { BrowserRouter, useRoutes } from "react-router-dom";
import { routeTable } from "./routes";
import { TierProvider, useTierMedia } from "./lib/tier";
import { useTheme } from "./lib/theme";
import { ToastHost } from "./lib/toast";

function Routes() {
  return useRoutes(routeTable);
}

/**
 * 应用根。
 * 路由选 BrowserRouter：服务端对非 /api/* 的未知路径回退 index.html（docs/api.md §6，实测 /book/1 返回 200），
 * 因此可以直接用可读的干净地址，不需要 hash。#/ 形式的 hash 路由反而会把「返回」行为交给浏览器历史之外的逻辑。
 */
export function App() {
  const tier = useTierMedia();
  useTheme();

  return (
    <TierProvider value={tier}>
      <BrowserRouter>
        <Routes />
        {/* toast 的唯一渲染点：挂在路由之外，/read（阅读器是独立外壳，不在 AppShell 内）也能看见；
            放在这张图里之前它挂在 AppShell，阅读器里的操作结果一律永不渲染。 */}
        <ToastHost />
      </BrowserRouter>
    </TierProvider>
  );
}
