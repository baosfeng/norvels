import { BrowserRouter, useRoutes } from "react-router-dom";
import { routeTable } from "./routes";
import { TierProvider, useTierMedia } from "./lib/tier";
import { useTheme } from "./lib/theme";

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
      </BrowserRouter>
    </TierProvider>
  );
}
