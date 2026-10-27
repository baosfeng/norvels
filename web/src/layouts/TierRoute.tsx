import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { useTier } from "../lib/tier";

/**
 * 分档路由闸门：H5 下管理类路由直接不渲染（DOM 里没有），改为跳到该档首屏。
 * 这是一个判断，不是 CSS display:none —— 组件根本不会挂载，也就不会发请求。
 */
export function TierRoute({
  children,
  minTier = "pc",
}: {
  children: ReactNode;
  minTier?: "pc" | "h5";
}) {
  const tier = useTier();
  const location = useLocation();
  if (minTier === "pc" && tier === "h5") {
    return <Navigate to="/home" replace state={{ from: location.pathname }} />;
  }
  if (minTier === "h5" && tier === "pc") {
    return <Navigate to="/shelf" replace state={{ from: location.pathname }} />;
  }
  return <>{children}</>;
}
