import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { routeTier, useTier } from "../lib/tier";

/**
 * 分档路由闸门：H5 下管理类路由直接不渲染（DOM 里没有），改为跳到该档首屏。
 * 这是一个判断，不是 CSS display:none —— 组件根本不会挂载，也就不会发请求。
 *
 * 档位由 `tier.ts` 的 `routeTier(segment)` 决定（唯一事实源），路由表只给出自己的一级段名；
 * 这样「常量说两档可用、路由表却拦住」这类不同源不可能再发生。
 */
export function TierRoute({
  children,
  segment,
  minTier,
}: {
  children: ReactNode;
  /** 路由的一级路径段（如 "series"）；给了就按 tier.ts 的表判定。 */
  segment?: string;
  /** 显式指定档位（覆盖表）；一般只用 segment。 */
  minTier?: "pc" | "h5";
}) {
  const tier = useTier();
  const location = useLocation();
  const required = minTier ?? (segment ? routeTier(segment).minTier : null);
  if (required === null) return <>{children}</>;
  if (required === "pc" && tier === "h5") {
    return <Navigate to="/home" replace state={{ from: location.pathname }} />;
  }
  if (required === "h5" && tier === "pc") {
    return <Navigate to="/shelf" replace state={{ from: location.pathname }} />;
  }
  return <>{children}</>;
}
