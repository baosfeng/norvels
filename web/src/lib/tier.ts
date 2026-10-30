import { createContext, useContext, useEffect, useState } from "react";

/**
 * H5/PC 分档：≤768 是轻量阅读器，≥1024 是全功能工作台（768–1024 归 PC，保留管理能力）。
 * H5 不是「不渲染工具栏」：书架两档同一套筛选（H5 收进底部弹层）、系列与阅读历史两档都开。
 */
export type Tier = "pc" | "h5";

export const H5_QUERY = "(max-width: 768px)";

export function tierOf(matches: boolean): Tier {
  return matches ? "h5" : "pc";
}

/**
 * 每条路由的档位：**唯一事实源**。
 *
 * 之前 MANAGEMENT_ROUTES（只有管理四页）与 routes.tsx 里实际包的 TierRoute（多包了 series / history）
 * 各写一套，导致「常量说系列归两档、路由表说系列只在 PC」这种不同源。
 * 现在由本表生成路由闸门（routes.tsx）与「H5 是否承认这个路由」的判断，两边不可能再漂移。
 *
 * - pc：管理面，H5 下整页不渲染（不是 CSS 隐藏，组件根本不挂载）
 * - h5：窄屏首屏，PC 下落到书架
 * - both：两档都开，缺省值（不列进表）
 */
export const PC_ONLY_ROUTES = ["import", "authors", "dashboard", "admin"] as const;
export const H5_ONLY_ROUTES = ["home"] as const;

/** 管理能力路由（H5 下不渲染，也不承认这个地址）。名字保留，语义 = PC_ONLY_ROUTES。 */
export const MANAGEMENT_ROUTES = PC_ONLY_ROUTES;

export type RouteTier =
  | { minTier: "pc" }
  | { minTier: "h5" }
  | { minTier: null };

/** 某个一级路径段在哪些档位可用；表里没有的一律两档可用。 */
export function routeTier(segment: string): RouteTier {
  if ((PC_ONLY_ROUTES as readonly string[]).includes(segment)) return { minTier: "pc" };
  if ((H5_ONLY_ROUTES as readonly string[]).includes(segment)) return { minTier: "h5" };
  return { minTier: null };
}

export function isManagementRoute(segment: string): boolean {
  return (PC_ONLY_ROUTES as readonly string[]).includes(segment);
}

const TierContext = createContext<Tier>("pc");

export function useTierMedia(): Tier {
  const [tier, setTier] = useState<Tier>(() => {
    if (typeof window === "undefined" || !window.matchMedia) return "pc";
    return tierOf(window.matchMedia(H5_QUERY).matches);
  });

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const mq = window.matchMedia(H5_QUERY);
    const onChange = (event: MediaQueryListEvent) => setTier(tierOf(event.matches));
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  return tier;
}

export const TierProvider = TierContext.Provider;

/** 唯一的分档读取入口：条件渲染一律走它，不用 CSS 隐藏。 */
export function useTier(): Tier {
  return useContext(TierContext);
}

export function useIsH5(): boolean {
  return useTier() === "h5";
}
